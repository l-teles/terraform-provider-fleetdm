package fleetdm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestS3Source_Fields(t *testing.T) {
	src := S3Source{
		Bucket: "my-bucket",
		Key:    "path/to/package.pkg",
		Region: "us-east-1",
	}

	if src.Bucket != "my-bucket" {
		t.Errorf("expected bucket 'my-bucket', got %q", src.Bucket)
	}
	if src.Key != "path/to/package.pkg" {
		t.Errorf("expected key 'path/to/package.pkg', got %q", src.Key)
	}
	if src.Region != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got %q", src.Region)
	}
}

func TestS3Source_OptionalRegion(t *testing.T) {
	src := S3Source{
		Bucket: "my-bucket",
		Key:    "installer.msi",
	}

	if src.Region != "" {
		t.Errorf("expected empty region, got %q", src.Region)
	}
}

func TestS3Source_EndpointURL(t *testing.T) {
	src := S3Source{
		Bucket:      "my-bucket",
		Key:         "installer.pkg",
		EndpointURL: "http://localhost:4566",
	}

	if src.EndpointURL != "http://localhost:4566" {
		t.Errorf("expected endpoint URL 'http://localhost:4566', got %q", src.EndpointURL)
	}
}

func TestDownloadS3Object(t *testing.T) {
	content := []byte("fake-installer-content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// S3 GetObject with path-style: GET /bucket/key
		if r.URL.Path == "/test-bucket/test-installer.pkg" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(content)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Set dummy AWS credentials so the SDK does not fail looking for real ones.
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a proper S3-style 404 XML error
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
