package fleetdm

import (
	"context"
	"net/http"
	"net/http/httptest"
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
