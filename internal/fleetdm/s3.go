package fleetdm

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
