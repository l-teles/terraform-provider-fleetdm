package fleetdm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_UploadEULA(t *testing.T) {
	content := []byte("# Terms\n\nBy enrolling you agree.\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/fleet/setup_experience/eula" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("platform must travel in the form only, got query %q", r.URL.RawQuery)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("platform"); got != "windows" {
			t.Errorf("expected platform=windows, got %q", got)
		}
		f, hdr, err := r.FormFile("eula")
		if err != nil {
			t.Fatalf("eula file part: %v", err)
		}
		defer func() { _ = f.Close() }()
		if hdr.Filename != "terms.md" {
			t.Errorf("expected filename terms.md, got %q", hdr.Filename)
		}
		got, _ := io.ReadAll(f)
		if string(got) != string(content) {
			t.Errorf("unexpected content %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	if err := client.UploadEULA(context.Background(), EULAPlatformWindows, "terms.md", content); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_UploadEULA_Conflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"Resource Already Exists"}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	err := client.UploadEULA(context.Background(), EULAPlatformDarwin, "eula.pdf", []byte("%PDF-1.4\n"))
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("expected a 409 APIError, got %v", err)
	}
}

// TestClient_GetEULAMetadata checks that the base64 sha256 Fleet sends (a
// []byte field) decodes to the raw digest and renders as hex.
func TestClient_GetEULAMetadata(t *testing.T) {
	sum := sha256.Sum256([]byte("%PDF-1.4\n"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/setup_experience/eula/metadata" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("platform"); got != "darwin" {
			t.Errorf("expected platform=darwin, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":       "eula.pdf",
			"token":      "tok-1",
			"created_at": "2026-01-02T03:04:05Z",
			"platform":   "darwin",
			"sha256":     sum[:], // encoding/json writes []byte as base64
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	eula, err := client.GetEULAMetadata(context.Background(), EULAPlatformDarwin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eula.Name != "eula.pdf" || eula.Token != "tok-1" || eula.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("unexpected metadata %+v", eula)
	}
	wantHex := hex.EncodeToString(sum[:])
	if eula.Sha256Hex() != wantHex {
		t.Errorf("Sha256Hex = %q, want %q", eula.Sha256Hex(), wantHex)
	}
}

func TestClient_GetEULAMetadata_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource Not Found"}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	_, err := client.GetEULAMetadata(context.Background(), EULAPlatformWindows)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a 404 APIError, got %v", err)
	}
}

// TestClient_GetEULAMetadata_PlatformMismatch covers Fleet before 4.94, which
// ignores ?platform and answers with the macOS EULA and no platform field.
func TestClient_GetEULAMetadata_PlatformMismatch(t *testing.T) {
	cases := []struct {
		name      string
		requested string
		reported  string
		wantErr   bool
	}{
		{"pre-4.94 darwin", EULAPlatformDarwin, "", false},
		{"pre-4.94 windows", EULAPlatformWindows, "", true},
		{"4.94 windows", EULAPlatformWindows, "windows", false},
		{"4.94 wrong platform", EULAPlatformWindows, "darwin", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				body := map[string]any{"name": "eula.pdf", "token": "tok"}
				if tc.reported != "" {
					body["platform"] = tc.reported
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()

			client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
			_, err := client.GetEULAMetadata(context.Background(), tc.requested)
			if tc.wantErr != errors.Is(err, ErrEULAPlatformUnsupported) {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestClient_DeleteEULA(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/fleet/setup_experience/eula/tok-1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	if err := client.DeleteEULA(context.Background(), "tok-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("server was not called")
	}
}
