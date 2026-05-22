package fleetdm

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ErrUnsupportedChecksum indicates the S3 object exposes a SHA256 checksum that
// cannot be compared to sha256(content) — currently only multipart composite
// checksums fall in this bucket. Callers should surface a fatal error so the
// user can fix the upload.
var ErrUnsupportedChecksum = errors.New("s3 object has an unsupported SHA256 checksum")

// ErrNoSHA256Available indicates the S3 object has no usable SHA256 — neither
// a server-managed full-object checksum nor a well-formed x-amz-meta-sha256.
// Callers should fall back to downloading the body and hashing locally.
var ErrNoSHA256Available = errors.New("s3 object has no SHA256 available")

// hexSHA256Pattern matches a lowercase hex-encoded SHA256.
var hexSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Sources for the SHA returned by FetchS3ObjectSHA256.
const (
	SHASourceS3Checksum     = "s3-checksum"
	SHASourceObjectMetadata = "object-metadata"
)

// MaxPackageSize is the maximum allowed size for a software package download (2 GiB).
const MaxPackageSize int64 = 2 << 30

// S3Source represents an S3 object location.
type S3Source struct {
	Bucket      string
	Key         string
	Region      string // optional
	EndpointURL string // optional, for S3-compatible services (LocalStack, MinIO)
}

// s3ClientKey is the cache key for S3 clients.
type s3ClientKey struct {
	Region      string
	EndpointURL string
}

var (
	s3ClientCache = make(map[s3ClientKey]*s3.Client)
	s3ClientMu    sync.Mutex
)

// getOrCreateS3Client returns a cached S3 client for the given region/endpoint,
// creating one if it doesn't already exist. This avoids repeated credential chain
// resolution when downloading multiple packages in a single apply.
func getOrCreateS3Client(ctx context.Context, src S3Source) (*s3.Client, error) {
	key := s3ClientKey{Region: src.Region, EndpointURL: src.EndpointURL}

	s3ClientMu.Lock()
	if client, ok := s3ClientCache[key]; ok {
		s3ClientMu.Unlock()
		return client, nil
	}
	// Hold the lock while creating the client to prevent duplicate work.
	// LoadDefaultConfig may do network I/O (IMDS) which blocks concurrent
	// downloads for different keys. This is acceptable since it only happens
	// once per unique (region, endpoint) pair. If this becomes a bottleneck,
	// consider singleflight or per-key locking.
	defer s3ClientMu.Unlock()

	var opts []func(*config.LoadOptions) error
	if src.Region != "" {
		opts = append(opts, config.WithRegion(src.Region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if src.EndpointURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(src.EndpointURL)
			o.UsePathStyle = true // needed for LocalStack, MinIO, and test servers
		})
	}
	client := s3.NewFromConfig(cfg, s3Opts...)
	s3ClientCache[key] = client

	return client, nil
}

// DownloadS3Object downloads an object from S3 and returns its content.
// Objects larger than MaxPackageSize (2 GB) are rejected.
func DownloadS3Object(ctx context.Context, src S3Source) ([]byte, error) {
	client, err := getOrCreateS3Client(ctx, src)
	if err != nil {
		return nil, err
	}

	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(src.Bucket),
		Key:    aws.String(src.Key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download s3://%s/%s: %w", src.Bucket, src.Key, err)
	}
	defer func() { _ = output.Body.Close() }()

	// Enforce a size limit to prevent OOM on misconfigured keys.
	if output.ContentLength != nil && *output.ContentLength > MaxPackageSize {
		return nil, fmt.Errorf("s3://%s/%s is too large (%d bytes, max %d)", src.Bucket, src.Key, *output.ContentLength, MaxPackageSize)
	}

	// Use LimitReader as a safety net even when ContentLength is not reported.
	limitedReader := io.LimitReader(output.Body, MaxPackageSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		// Check if the context was cancelled during the read.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("S3 download cancelled: %w", ctx.Err())
		}
		return nil, fmt.Errorf("failed to read S3 object body: %w", err)
	}
	if int64(len(data)) > MaxPackageSize {
		return nil, fmt.Errorf("s3://%s/%s exceeds maximum size (%d bytes, max %d)", src.Bucket, src.Key, len(data), MaxPackageSize)
	}

	return data, nil
}

// ResetS3ClientCache clears the cached S3 clients. Useful for testing.
func ResetS3ClientCache() {
	s3ClientMu.Lock()
	s3ClientCache = make(map[s3ClientKey]*s3.Client)
	s3ClientMu.Unlock()
}

// FetchS3ObjectSHA256 resolves the SHA256 of an S3 object using HeadObject —
// it does NOT download the body. The returned hash is lowercase hex (matching
// what Fleet stores), and `source` describes where it came from for logging.
//
// Resolution order:
//  1. Server-managed full-object SHA256 (object uploaded with
//     `--checksum-algorithm SHA256` in a single part). Returned as
//     SHASourceS3Checksum.
//  2. User-controlled `x-amz-meta-sha256` metadata header, when it's a valid
//     lowercase hex SHA256. Returned as SHASourceObjectMetadata.
//
// Returns ErrUnsupportedChecksum when the only SHA256 present is a multipart
// composite checksum (which is sha256(concat(part-sha256s)), not
// sha256(content)). The error message names the three remediation paths.
//
// Returns ErrNoSHA256Available when no usable SHA256 is present. Callers are
// expected to fall back to DownloadS3Object and hash the body locally.
//
// Other errors (network, auth, 404) are returned as-is from the SDK.
func FetchS3ObjectSHA256(ctx context.Context, src S3Source) (string, string, error) {
	client, err := getOrCreateS3Client(ctx, src)
	if err != nil {
		return "", "", err
	}

	output, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket:       aws.String(src.Bucket),
		Key:          aws.String(src.Key),
		ChecksumMode: s3types.ChecksumModeEnabled,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to HEAD s3://%s/%s: %w", src.Bucket, src.Key, err)
	}

	if output.ChecksumSHA256 != nil && *output.ChecksumSHA256 != "" {
		if output.ChecksumType == s3types.ChecksumTypeComposite {
			return "", "", fmt.Errorf(
				"%w: s3://%s/%s reports a composite (multipart) SHA256 checksum, "+
					"which is not the SHA256 of the object content. Either: "+
					"(a) re-upload the object in a single part with `--checksum-algorithm SHA256`, "+
					"(b) set the metadata header `x-amz-meta-sha256` to the lowercase hex SHA256 of the file, or "+
					"(c) set `package_s3.expected_sha256` in your Terraform config",
				ErrUnsupportedChecksum, src.Bucket, src.Key,
			)
		}
		// FULL_OBJECT (or empty ChecksumType, which AWS uses for single-part uploads
		// with SHA256 — those are full-object by definition).
		raw, err := base64.StdEncoding.DecodeString(*output.ChecksumSHA256)
		if err != nil {
			return "", "", fmt.Errorf("s3://%s/%s returned a malformed SHA256 checksum %q: %w", src.Bucket, src.Key, *output.ChecksumSHA256, err)
		}
		if len(raw) != 32 {
			return "", "", fmt.Errorf("s3://%s/%s returned a SHA256 checksum of unexpected length %d (want 32)", src.Bucket, src.Key, len(raw))
		}
		return hex.EncodeToString(raw), SHASourceS3Checksum, nil
	}

	// Fall back to user metadata. The SDK lowercases metadata keys and strips
	// the `x-amz-meta-` prefix, so `x-amz-meta-sha256` lands in Metadata["sha256"].
	if metaSHA, ok := output.Metadata["sha256"]; ok && hexSHA256Pattern.MatchString(metaSHA) {
		return metaSHA, SHASourceObjectMetadata, nil
	}

	return "", "", fmt.Errorf(
		"%w: s3://%s/%s has no SHA256 via HeadObject. Either: "+
			"(a) re-upload with `--checksum-algorithm SHA256`, "+
			"(b) set the metadata header `x-amz-meta-sha256` to the lowercase hex SHA256 of the file, or "+
			"(c) set `package_s3.expected_sha256` in your Terraform config",
		ErrNoSHA256Available, src.Bucket, src.Key,
	)
}
