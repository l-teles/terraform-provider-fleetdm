package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient is a small local helper to keep the web app tests terse.
func newWebAppTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{ServerAddress: serverURL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

// TestClient_CreateWebApp_WithIcon pins the multipart shape Fleet's decoder
// expects: text fields "title" and "url", plus a file part named "icon".
func TestClient_CreateWebApp_WithIcon(t *testing.T) {
	const wantID = "com.google.enterprise.webapp.0123456789abcdef"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/software/web_apps" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %q", ct)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if got := r.FormValue("title"); got != "Support Portal" {
			t.Errorf("expected title 'Support Portal', got %q", got)
		}
		if got := r.FormValue("url"); got != "https://support.example.com" {
			t.Errorf("expected url 'https://support.example.com', got %q", got)
		}

		file, header, err := r.FormFile("icon")
		if err != nil {
			t.Fatalf("failed to get icon form file: %v", err)
		}
		defer file.Close()
		if header.Filename != "timesheets-512.png" {
			t.Errorf("expected filename 'timesheets-512.png', got %q", header.Filename)
		}
		content, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("failed to read icon content: %v", err)
		}
		if string(content) != "fake-png-bytes" {
			t.Errorf("unexpected icon content %q", string(content))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"app_store_id": wantID})
	}))
	defer server.Close()

	client := newWebAppTestClient(t, server.URL)

	appStoreID, err := client.CreateWebApp(context.Background(), &CreateWebAppRequest{
		Title:    "Support Portal",
		URL:      "https://support.example.com",
		IconName: "timesheets-512.png",
		Icon:     []byte("fake-png-bytes"),
	})
	if err != nil {
		t.Fatalf("CreateWebApp failed: %v", err)
	}
	if appStoreID != wantID {
		t.Errorf("expected app_store_id %q, got %q", wantID, appStoreID)
	}
}

// TestClient_CreateWebApp_WithoutIcon verifies that omitting the icon sends a
// multipart body carrying the text fields and no file part at all — Fleet reads
// a missing "icon" part as "no icon".
func TestClient_CreateWebApp_WithoutIcon(t *testing.T) {
	const wantID = "com.google.enterprise.webapp.abc123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if got := r.FormValue("title"); got != "Timesheets" {
			t.Errorf("expected title 'Timesheets', got %q", got)
		}
		if got := r.FormValue("url"); got != "https://timesheets.example.com" {
			t.Errorf("expected url 'https://timesheets.example.com', got %q", got)
		}
		if r.MultipartForm != nil && len(r.MultipartForm.File["icon"]) != 0 {
			t.Errorf("expected no icon file part, got %d", len(r.MultipartForm.File["icon"]))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"app_store_id": wantID})
	}))
	defer server.Close()

	client := newWebAppTestClient(t, server.URL)

	appStoreID, err := client.CreateWebApp(context.Background(), &CreateWebAppRequest{
		Title: "Timesheets",
		URL:   "https://timesheets.example.com",
	})
	if err != nil {
		t.Fatalf("CreateWebApp failed: %v", err)
	}
	if appStoreID != wantID {
		t.Errorf("expected app_store_id %q, got %q", wantID, appStoreID)
	}
}

// TestClient_CreateWebApp_DefaultIconName verifies the fallback filename when
// the caller supplies icon bytes but no name.
func TestClient_CreateWebApp_DefaultIconName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		_, header, err := r.FormFile("icon")
		if err != nil {
			t.Fatalf("failed to get icon form file: %v", err)
		}
		if header.Filename != "icon.png" {
			t.Errorf("expected default filename 'icon.png', got %q", header.Filename)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"app_store_id": "com.google.enterprise.webapp.def456"})
	}))
	defer server.Close()

	client := newWebAppTestClient(t, server.URL)

	if _, err := client.CreateWebApp(context.Background(), &CreateWebAppRequest{
		Title: "No Name Icon",
		URL:   "https://example.com",
		Icon:  []byte("fake-png-bytes"),
	}); err != nil {
		t.Fatalf("CreateWebApp failed: %v", err)
	}
}

// TestClient_CreateWebApp_AndroidMDMDisabled covers the gating error Fleet
// returns when Android MDM is not enabled — the VerifyAndroidMDM middleware
// answers 400 before the endpoint runs.
func TestClient_CreateWebApp_AndroidMDMDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Android MDM isn't turned on."})
	}))
	defer server.Close()

	client := newWebAppTestClient(t, server.URL)

	_, err := client.CreateWebApp(context.Background(), &CreateWebAppRequest{
		Title: "Support Portal",
		URL:   "https://support.example.com",
	})
	if err == nil {
		t.Fatal("expected an error when Android MDM is disabled, got nil")
	}
	if !strings.Contains(err.Error(), "Android MDM isn't turned on.") {
		t.Errorf("expected the Android MDM gating message, got %q", err.Error())
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected the error to wrap *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

// TestClient_CreateWebApp_ValidationError covers Fleet's icon validation, which
// runs server-side (square PNG, >= 512x512).
func TestClient_CreateWebApp_ValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Couldn't create. The icon must be a PNG file and square, with dimensions of at least 512 x 512px.",
		})
	}))
	defer server.Close()

	client := newWebAppTestClient(t, server.URL)

	_, err := client.CreateWebApp(context.Background(), &CreateWebAppRequest{
		Title:    "Bad Icon",
		URL:      "https://example.com",
		IconName: "non-square.png",
		Icon:     []byte("not-really-a-png"),
	})
	if err == nil {
		t.Fatal("expected an error for an invalid icon, got nil")
	}
	if !strings.Contains(err.Error(), "must be a PNG file and square") {
		t.Errorf("expected the icon validation message, got %q", err.Error())
	}
}

// TestClient_CreateWebApp_EmptyAppStoreID guards against Fleet answering 200
// with no ID, which would otherwise write an empty app_store_id into state.
func TestClient_CreateWebApp_EmptyAppStoreID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"app_store_id": ""})
	}))
	defer server.Close()

	client := newWebAppTestClient(t, server.URL)

	_, err := client.CreateWebApp(context.Background(), &CreateWebAppRequest{
		Title: "Empty",
		URL:   "https://example.com",
	})
	if err == nil {
		t.Fatal("expected an error when app_store_id is empty, got nil")
	}
	if !strings.Contains(err.Error(), "app_store_id is empty") {
		t.Errorf("unexpected error %q", err.Error())
	}
}

// TestClient_CreateWebApp_MalformedResponse covers a non-JSON 200 body.
func TestClient_CreateWebApp_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := newWebAppTestClient(t, server.URL)

	_, err := client.CreateWebApp(context.Background(), &CreateWebAppRequest{
		Title: "Malformed",
		URL:   "https://example.com",
	})
	if err == nil {
		t.Fatal("expected an error for a malformed response, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse create web app response") {
		t.Errorf("unexpected error %q", err.Error())
	}
}
