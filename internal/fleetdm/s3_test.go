package fleetdm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadS3Object(t *testing.T) {
	ResetS3ClientCache()
	content := []byte("fake-installer-content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header to be present")
		}
		if r.URL.Path == "/test-bucket/test-installer.pkg" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(content)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	src := S3Source{
		Bucket:      "test-bucket",
		Key:         "test-installer.pkg",
		Region:      "us-east-1",
		EndpointURL: server.URL,
	}

	data, err := DownloadS3Object(context.Background(), src)
	if err != nil {
		t.Fatalf("DownloadS3Object returned error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected content %q, got %q", string(content), string(data))
	}
}

func TestDownloadS3Object_NotFound(t *testing.T) {
	ResetS3ClientCache()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>NoSuchKey</Code>
  <Message>The specified key does not exist.</Message>
  <Key>nonexistent.pkg</Key>
</Error>`))
	}))
	defer server.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	src := S3Source{
		Bucket:      "test-bucket",
		Key:         "nonexistent.pkg",
		Region:      "us-east-1",
		EndpointURL: server.URL,
	}

	_, err := DownloadS3Object(context.Background(), src)
	if err == nil {
		t.Fatal("expected error for non-existent key, got nil")
	}
}

func TestDownloadS3Object_ClientCaching(t *testing.T) {
	ResetS3ClientCache()
	content := []byte("cached-content")

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(content)
	}))
	defer server.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	src := S3Source{
		Bucket:      "test-bucket",
		Key:         "file1.pkg",
		Region:      "us-east-1",
		EndpointURL: server.URL,
	}

	// Two downloads with same region+endpoint should reuse the client.
	_, err := DownloadS3Object(context.Background(), src)
	if err != nil {
		t.Fatalf("first download failed: %v", err)
	}

	src.Key = "file2.pkg"
	_, err = DownloadS3Object(context.Background(), src)
	if err != nil {
		t.Fatalf("second download failed: %v", err)
	}

	if requestCount != 2 {
		t.Errorf("expected 2 HTTP requests, got %d", requestCount)
	}

	// Verify the cache has exactly one entry (same region+endpoint = same client).
	s3ClientMu.Lock()
	cacheSize := len(s3ClientCache)
	s3ClientMu.Unlock()
	if cacheSize != 1 {
		t.Errorf("expected 1 cached S3 client, got %d", cacheSize)
	}
}

// headHandlerOpts controls how the mock HEAD handler responds. Tests construct
// one and pass it to newHeadObjectServer.
type headHandlerOpts struct {
	// statusCode overrides the response status (default 200).
	statusCode int
	// checksumSHA256 sets the x-amz-checksum-sha256 header (already base64).
	checksumSHA256 string
	// checksumType sets the x-amz-checksum-type header (FULL_OBJECT or COMPOSITE).
	checksumType string
	// metaSHA256 sets the x-amz-meta-sha256 header (hex or arbitrary).
	metaSHA256 string
	// onHEAD, if non-nil, is invoked for each HEAD request (for assertions).
	onHEAD func(r *http.Request)
}

// newHeadObjectServer returns a test S3 server that only services HEAD requests.
// It is intentionally strict: any GET on the object path is treated as a test
// failure, so we prove FetchS3ObjectSHA256 never downloads the body.
func newHeadObjectServer(t *testing.T, bucketKey string, opts headHandlerOpts) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+bucketKey {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			t.Errorf("unexpected GET request to %s — FetchS3ObjectSHA256 must not download the body", r.URL.Path)
			http.Error(w, "unexpected GET", http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if opts.onHEAD != nil {
			opts.onHEAD(r)
		}
		if opts.checksumSHA256 != "" {
			w.Header().Set("x-amz-checksum-sha256", opts.checksumSHA256)
		}
		if opts.checksumType != "" {
			w.Header().Set("x-amz-checksum-type", opts.checksumType)
		}
		if opts.metaSHA256 != "" {
			w.Header().Set("x-amz-meta-sha256", opts.metaSHA256)
		}
		w.Header().Set("Content-Length", "0")
		if opts.statusCode != 0 {
			w.WriteHeader(opts.statusCode)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func setS3TestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
}

// sha256B64 returns the base64-encoded SHA256 of the input — the format S3
// uses on the wire for `x-amz-checksum-sha256`.
func sha256B64(b []byte) string {
	sum := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// sha256Hex returns the lowercase hex SHA256 of the input — Fleet's format.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestFetchS3ObjectSHA256_FullObjectChecksum(t *testing.T) {
	ResetS3ClientCache()
	setS3TestEnv(t)

	content := []byte("hello-installer-content")
	server := newHeadObjectServer(t, "b/k.pkg", headHandlerOpts{
		checksumSHA256: sha256B64(content),
		checksumType:   "FULL_OBJECT",
	})
	defer server.Close()

	sha, source, err := FetchS3ObjectSHA256(context.Background(), S3Source{
		Bucket: "b", Key: "k.pkg", Region: "us-east-1", EndpointURL: server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha != sha256Hex(content) {
		t.Errorf("sha mismatch: got %s want %s", sha, sha256Hex(content))
	}
	if source != SHASourceS3Checksum {
		t.Errorf("source = %q, want %q", source, SHASourceS3Checksum)
	}
}

func TestFetchS3ObjectSHA256_CompositeChecksum(t *testing.T) {
	ResetS3ClientCache()
	setS3TestEnv(t)

	server := newHeadObjectServer(t, "b/k.pkg", headHandlerOpts{
		checksumSHA256: sha256B64([]byte("hash-of-hashes-not-content")),
		checksumType:   "COMPOSITE",
	})
	defer server.Close()

	_, _, err := FetchS3ObjectSHA256(context.Background(), S3Source{
		Bucket: "b", Key: "k.pkg", Region: "us-east-1", EndpointURL: server.URL,
	})
	if err == nil {
		t.Fatal("expected error for composite checksum, got nil")
	}
	if !errors.Is(err, ErrUnsupportedChecksum) {
		t.Errorf("expected ErrUnsupportedChecksum, got %v", err)
	}
	// The user-facing message must point at all three fixes.
	msg := err.Error()
	for _, fragment := range []string{"--checksum-algorithm SHA256", "x-amz-meta-sha256", "expected_sha256"} {
		if !strings.Contains(msg, fragment) {
			t.Errorf("error message missing remediation %q: %s", fragment, msg)
		}
	}
}

func TestFetchS3ObjectSHA256_MetadataSHA256(t *testing.T) {
	ResetS3ClientCache()
	setS3TestEnv(t)

	content := []byte("metadata-only-installer")
	server := newHeadObjectServer(t, "b/k.pkg", headHandlerOpts{
		metaSHA256: sha256Hex(content),
	})
	defer server.Close()

	sha, source, err := FetchS3ObjectSHA256(context.Background(), S3Source{
		Bucket: "b", Key: "k.pkg", Region: "us-east-1", EndpointURL: server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha != sha256Hex(content) {
		t.Errorf("sha mismatch: got %s want %s", sha, sha256Hex(content))
	}
	if source != SHASourceObjectMetadata {
		t.Errorf("source = %q, want %q", source, SHASourceObjectMetadata)
	}
}

func TestFetchS3ObjectSHA256_MetadataMalformed(t *testing.T) {
	ResetS3ClientCache()
	setS3TestEnv(t)

	server := newHeadObjectServer(t, "b/k.pkg", headHandlerOpts{
		metaSHA256: "not-a-real-sha",
	})
	defer server.Close()

	_, _, err := FetchS3ObjectSHA256(context.Background(), S3Source{
		Bucket: "b", Key: "k.pkg", Region: "us-east-1", EndpointURL: server.URL,
	})
	if err == nil {
		t.Fatal("expected error when metadata SHA is malformed, got nil")
	}
	if !errors.Is(err, ErrNoSHA256Available) {
		t.Errorf("expected ErrNoSHA256Available, got %v", err)
	}
}

func TestFetchS3ObjectSHA256_BothSources_ChecksumWins(t *testing.T) {
	ResetS3ClientCache()
	setS3TestEnv(t)

	// The server-managed checksum points at one value; the metadata points at
	// a different (also valid hex) value. The server-managed one must win.
	contentForChecksum := []byte("the-real-content")
	contentForMeta := []byte("something-else-entirely")

	server := newHeadObjectServer(t, "b/k.pkg", headHandlerOpts{
		checksumSHA256: sha256B64(contentForChecksum),
		checksumType:   "FULL_OBJECT",
		metaSHA256:     sha256Hex(contentForMeta),
	})
	defer server.Close()

	sha, source, err := FetchS3ObjectSHA256(context.Background(), S3Source{
		Bucket: "b", Key: "k.pkg", Region: "us-east-1", EndpointURL: server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha != sha256Hex(contentForChecksum) {
		t.Errorf("expected server-managed checksum to win; got %s want %s", sha, sha256Hex(contentForChecksum))
	}
	if source != SHASourceS3Checksum {
		t.Errorf("source = %q, want %q", source, SHASourceS3Checksum)
	}
}

func TestFetchS3ObjectSHA256_NoChecksum(t *testing.T) {
	ResetS3ClientCache()
	setS3TestEnv(t)

	server := newHeadObjectServer(t, "b/k.pkg", headHandlerOpts{})
	defer server.Close()

	_, _, err := FetchS3ObjectSHA256(context.Background(), S3Source{
		Bucket: "b", Key: "k.pkg", Region: "us-east-1", EndpointURL: server.URL,
	})
	if err == nil {
		t.Fatal("expected error when no SHA is available, got nil")
	}
	if !errors.Is(err, ErrNoSHA256Available) {
		t.Errorf("expected ErrNoSHA256Available, got %v", err)
	}
}

func TestFetchS3ObjectSHA256_NotFound(t *testing.T) {
	ResetS3ClientCache()
	setS3TestEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, _, err := FetchS3ObjectSHA256(context.Background(), S3Source{
		Bucket: "b", Key: "missing.pkg", Region: "us-east-1", EndpointURL: server.URL,
	})
	if err == nil {
		t.Fatal("expected error for not-found object, got nil")
	}
	if errors.Is(err, ErrUnsupportedChecksum) || errors.Is(err, ErrNoSHA256Available) {
		t.Errorf("not-found should surface as a regular error, not a sentinel; got %v", err)
	}
}

func TestFetchS3ObjectSHA256_DoesNotReadBody(t *testing.T) {
	ResetS3ClientCache()
	setS3TestEnv(t)

	var sawHEAD bool
	server := newHeadObjectServer(t, "b/k.pkg", headHandlerOpts{
		checksumSHA256: sha256B64([]byte("ok")),
		checksumType:   "FULL_OBJECT",
		onHEAD: func(r *http.Request) {
			sawHEAD = true
		},
	})
	defer server.Close()

	if _, _, err := FetchS3ObjectSHA256(context.Background(), S3Source{
		Bucket: "b", Key: "k.pkg", Region: "us-east-1", EndpointURL: server.URL,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawHEAD {
		t.Fatal("HEAD handler was never called")
	}
	// The handler treats any GET as a test failure, so the absence of
	// t.Errorf above is also part of this assertion.
}
