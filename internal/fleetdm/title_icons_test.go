package fleetdm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newIconTestClient builds a client pointed at a mock server.
func newIconTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{
		ServerAddress: serverURL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

// testPNG returns a syntactically valid PNG. Small enough to inline, real
// enough that anything sniffing the magic bytes is satisfied. The client layer
// never inspects pixels, so a fixed header + IDAT stub is sufficient here; the
// provider-level tests generate a properly-sized image.
func testPNG() []byte {
	return append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("IHDRstub-idat-bytes")...)
}

// TestUploadTitleIcon pins the wire shape the Fleet 4.90 icon endpoint
// requires: PUT, the "icon" file part, and fleet_id in the query string (Fleet
// rejects it as a form field with "team_id is required").
func TestUploadTitleIcon(t *testing.T) {
	icon := testPNG()

	var (
		gotMethod      string
		gotPath        string
		gotFleetQuery  string
		gotPartName    string
		gotPartFile    string
		gotPartContent []byte
		gotFormFleetID string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotFleetQuery = r.URL.Query().Get("fleet_id")

		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Read the multipart body directly: r.FormValue merges the URL query
		// into the form, so it would report the query's fleet_id and defeat
		// the point of the assertion below.
		gotFormFleetID = strings.Join(r.MultipartForm.Value["fleet_id"], ",")

		for name, headers := range r.MultipartForm.File {
			gotPartName = name
			if len(headers) > 0 {
				gotPartFile = headers[0].Filename
				f, err := headers[0].Open()
				if err != nil {
					t.Errorf("failed to open part: %v", err)
					continue
				}
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(f); err != nil {
					t.Errorf("failed to read part: %v", err)
				}
				_ = f.Close()
				gotPartContent = buf.Bytes()
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"icon_url": "/api/latest/fleet/software/titles/42/icon?fleet_id=3",
		})
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	iconURL, err := client.UploadTitleIcon(context.Background(), &UploadTitleIconRequest{
		TitleID:  42,
		FleetID:  3,
		Icon:     icon,
		Filename: "brand.png",
	})
	if err != nil {
		t.Fatalf("UploadTitleIcon returned error: %v", err)
	}

	if iconURL != "/api/latest/fleet/software/titles/42/icon?fleet_id=3" {
		t.Errorf("unexpected icon_url: %q", iconURL)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if want := "/api/v1/fleet/software/titles/42/icon"; gotPath != want {
		t.Errorf("expected path %s, got %s", want, gotPath)
	}
	if gotFleetQuery != "3" {
		t.Errorf("expected fleet_id=3 in the query string, got %q", gotFleetQuery)
	}
	// Fleet ignores fleet_id when it arrives as a multipart field, so the
	// query string is the only thing carrying the scope. Asserting the body
	// stays clean documents that and keeps a future refactor from moving the
	// parameter into the form, where it would silently stop working.
	if gotFormFleetID != "" {
		t.Errorf("expected no fleet_id multipart field, got %q", gotFormFleetID)
	}
	if gotPartName != "icon" {
		t.Errorf("expected file part named \"icon\", got %q", gotPartName)
	}
	if gotPartFile != "brand.png" {
		t.Errorf("expected part filename brand.png, got %q", gotPartFile)
	}
	if !bytes.Equal(gotPartContent, icon) {
		t.Errorf("part content mismatch: got %d bytes, want %d", len(gotPartContent), len(icon))
	}
}

// TestUploadTitleIconFleetIDZero guards the "No team" scope: fleet_id 0 is a
// real fleet and must still be sent, not elided as a zero value.
func TestUploadTitleIconFleetIDZero(t *testing.T) {
	var gotRawQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"icon_url": "/icon"})
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	if _, err := client.UploadTitleIcon(context.Background(), &UploadTitleIconRequest{
		TitleID: 7,
		FleetID: 0,
		Icon:    testPNG(),
	}); err != nil {
		t.Fatalf("UploadTitleIcon returned error: %v", err)
	}

	if gotRawQuery != "fleet_id=0" {
		t.Errorf("expected query fleet_id=0, got %q", gotRawQuery)
	}
}

// TestUploadTitleIconDefaultFilename checks the fallback part filename, since
// Fleet requires a filename on the part even though it ignores the extension.
func TestUploadTitleIconDefaultFilename(t *testing.T) {
	var gotFilename string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			if headers := r.MultipartForm.File["icon"]; len(headers) > 0 {
				gotFilename = headers[0].Filename
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"icon_url": "/icon"})
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	if _, err := client.UploadTitleIcon(context.Background(), &UploadTitleIconRequest{
		TitleID: 7,
		FleetID: 0,
		Icon:    testPNG(),
	}); err != nil {
		t.Fatalf("UploadTitleIcon returned error: %v", err)
	}

	if gotFilename != "icon.png" {
		t.Errorf("expected default filename icon.png, got %q", gotFilename)
	}
}

// TestUploadTitleIconValidation covers the local guards before any HTTP call.
func TestUploadTitleIconValidation(t *testing.T) {
	client := newIconTestClient(t, "http://127.0.0.1:1")

	if _, err := client.UploadTitleIcon(context.Background(), nil); err == nil {
		t.Error("expected an error for a nil request")
	}
	if _, err := client.UploadTitleIcon(context.Background(), &UploadTitleIconRequest{TitleID: 1}); err == nil {
		t.Error("expected an error for empty icon content")
	}
}

// TestUploadTitleIconServerError checks Fleet's validation messages survive as
// an *APIError rather than being flattened into an opaque string.
func TestUploadTitleIconServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Bad request",
			"errors":  []map[string]string{{"name": "base", "reason": "icon must be at least 120x120 pixels"}},
		})
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	_, err := client.UploadTitleIcon(context.Background(), &UploadTitleIconRequest{
		TitleID: 1, FleetID: 0, Icon: testPNG(),
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "at least 120x120") {
		t.Errorf("expected Fleet's validation reason in the error, got: %v", err)
	}
}

// TestGetTitleIcon checks the raw-bytes read path used for drift detection.
func TestGetTitleIcon(t *testing.T) {
	icon := testPNG()

	var gotPath, gotFleetQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotFleetQuery = r.URL.Query().Get("fleet_id")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(icon)
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	got, err := client.GetTitleIcon(context.Background(), 42, 3)
	if err != nil {
		t.Fatalf("GetTitleIcon returned error: %v", err)
	}
	if !bytes.Equal(got, icon) {
		t.Errorf("expected the icon bytes verbatim, got %d bytes", len(got))
	}
	if want := "/api/v1/fleet/software/titles/42/icon"; gotPath != want {
		t.Errorf("expected path %s, got %s", want, gotPath)
	}
	if gotFleetQuery != "3" {
		t.Errorf("expected fleet_id=3, got %q", gotFleetQuery)
	}
}

// TestGetTitleIconNotFound checks that an absent icon surfaces as a 404
// *APIError, which is what the resource's Read keys off to drop state.
func TestGetTitleIconNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Resource Not Found",
			"errors":  []map[string]string{{"name": "base", "reason": "VPPApp was not found in the datastore"}},
		})
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	_, err := client.GetTitleIcon(context.Background(), 42, 0)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsTitleIconAbsent(err) {
		t.Errorf("expected IsTitleIconAbsent to recognise the 404, got: %v", err)
	}
}

// TestGetTitleIconOversizedResponse checks the read bound. Fleet caps icons at
// 100KB, so a multi-megabyte body means something other than a real icon is
// answering; the client must refuse it instead of buffering it all.
func TestGetTitleIconOversizedResponse(t *testing.T) {
	oversized := bytes.Repeat([]byte{0xAB}, MaxTitleIconSize+4096)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(oversized)
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	_, err := client.GetTitleIcon(context.Background(), 42, 0)
	if err == nil {
		t.Fatal("expected an error for an over-size icon response")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected a size-limit error, got: %v", err)
	}
	// The failure must not be reported as "no icon" — that would make Read
	// silently drop the resource from state and re-upload on every apply.
	if IsTitleIconAbsent(err) {
		t.Error("an over-size response must not be treated as an absent icon")
	}
}

// TestGetTitleIconAtSizeLimit checks the boundary is inclusive: a response of
// exactly MaxTitleIconSize is still accepted.
func TestGetTitleIconAtSizeLimit(t *testing.T) {
	atLimit := bytes.Repeat([]byte{0xCD}, MaxTitleIconSize)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(atLimit)
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	got, err := client.GetTitleIcon(context.Background(), 42, 0)
	if err != nil {
		t.Fatalf("a response at exactly the limit must be accepted, got: %v", err)
	}
	if len(got) != MaxTitleIconSize {
		t.Errorf("expected %d bytes, got %d", MaxTitleIconSize, len(got))
	}
}

// TestDeleteTitleIcon pins the delete route and its query parameter.
func TestDeleteTitleIcon(t *testing.T) {
	var gotMethod, gotPath, gotFleetQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotFleetQuery = r.URL.Query().Get("fleet_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	if err := client.DeleteTitleIcon(context.Background(), 42, 3); err != nil {
		t.Fatalf("DeleteTitleIcon returned error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if want := "/api/v1/fleet/software/titles/42/icon"; gotPath != want {
		t.Errorf("expected path %s, got %s", want, gotPath)
	}
	if gotFleetQuery != "3" {
		t.Errorf("expected fleet_id=3, got %q", gotFleetQuery)
	}
}

// TestIsTitleIconAbsent covers the Fleet quirk this helper exists for: DELETE
// against a title with no icon answers HTTP 500 "sql: no rows in result set"
// instead of a 404. Unrelated 500s must NOT be swallowed.
func TestIsTitleIconAbsent(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "404 from GET",
			err:  &APIError{StatusCode: http.StatusNotFound, Message: "Resource Not Found"},
			want: true,
		},
		{
			name: "500 no-rows in message",
			err:  &APIError{StatusCode: http.StatusInternalServerError, Message: "sql: no rows in result set"},
			want: true,
		},
		{
			name: "500 no-rows in errors detail",
			err: &APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Internal Server Error",
				Errors:     []ErrorDetail{{Name: "base", Reason: "sql: no rows in result set"}},
			},
			want: true,
		},
		{
			name: "unrelated 500 is a real failure",
			err:  &APIError{StatusCode: http.StatusInternalServerError, Message: "database connection refused"},
			want: false,
		},
		{
			name: "403 is a real failure",
			err:  &APIError{StatusCode: http.StatusForbidden, Message: "Forbidden"},
			want: false,
		},
		{
			name: "non-API error",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTitleIconAbsent(tt.err); got != tt.want {
				t.Errorf("IsTitleIconAbsent(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestDeleteTitleIconAbsentWrapped checks the wrapped error DeleteTitleIcon
// returns still satisfies IsTitleIconAbsent (errors.As has to see through the
// fmt.Errorf %w wrap).
func TestDeleteTitleIconAbsentWrapped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "sql: no rows in result set",
			"errors":  []map[string]string{{"name": "base", "reason": "sql: no rows in result set"}},
		})
	}))
	defer server.Close()

	client := newIconTestClient(t, server.URL)

	err := client.DeleteTitleIcon(context.Background(), 42, 0)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsTitleIconAbsent(err) {
		t.Errorf("expected the wrapped error to be recognised as absent, got: %v", err)
	}
}
