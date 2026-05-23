package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
)

// TestClient_GetSetupExperienceSoftware verifies the response decode.
func TestClient_GetSetupExperienceSoftware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/setup_experience/software" {
			t.Errorf("expected /setup_experience/software path, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("team_id"); got != "5" {
			t.Errorf("expected team_id=5, got %q", got)
		}
		if got := r.URL.Query().Get("platform"); got != "darwin" {
			t.Errorf("expected platform=darwin, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"software_titles": []map[string]any{
				{"id": 42}, {"id": 99},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test", VerifyTLS: false})
	teamID := 5
	ids, err := client.GetSetupExperienceSoftware(context.Background(), &teamID, "darwin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != 42 || ids[1] != 99 {
		t.Errorf("unexpected ids: %v", ids)
	}
}

// TestClient_SetSetupExperienceSoftwareInclude_RMW exercises the
// read-modify-write helper. Two goroutines on the same (teamID, platform)
// each call Include with a different title — both must end up in the
// final PUT body. Without the per-(team, platform) mutex, the second
// goroutine's GET would race against the first's PUT and lose the
// first's title.
func TestClient_SetSetupExperienceSoftwareInclude_RMW(t *testing.T) {
	var (
		mu       sync.Mutex
		titles   = []int{} // server-side authoritative set
		putCount int
		getCount int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			mu.Lock()
			payload := map[string]any{"software_titles": []map[string]any{}}
			arr := []map[string]any{}
			for _, id := range titles {
				arr = append(arr, map[string]any{"id": id})
			}
			payload["software_titles"] = arr
			getCount++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(payload)
		case http.MethodPut:
			var body struct {
				SoftwareTitleIDs []int `json:"software_title_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			titles = append([]int{}, body.SoftwareTitleIDs...)
			putCount++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test", VerifyTLS: false})
	teamID := 1

	// Two concurrent Include calls on the same (team, platform) — the
	// per-(team, platform) mutex on the Client must serialize them.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := client.SetSetupExperienceSoftwareInclude(context.Background(), &teamID, "darwin", 100); err != nil {
			t.Errorf("Include(100) failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := client.SetSetupExperienceSoftwareInclude(context.Background(), &teamID, "darwin", 200); err != nil {
			t.Errorf("Include(200) failed: %v", err)
		}
	}()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	sort.Ints(titles)
	if len(titles) != 2 || titles[0] != 100 || titles[1] != 200 {
		t.Fatalf("expected final set to contain both [100, 200], got %v (after %d GETs and %d PUTs)", titles, getCount, putCount)
	}
}

// TestClient_SetSetupExperienceSoftwareExclude exercises removal.
// Idempotent — calling Exclude on a title that's not in the set must not
// emit a PUT.
func TestClient_SetSetupExperienceSoftwareExclude(t *testing.T) {
	var (
		mu       sync.Mutex
		titles   = []int{42, 99}
		putCount int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			mu.Lock()
			arr := []map[string]any{}
			for _, id := range titles {
				arr = append(arr, map[string]any{"id": id})
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_titles": arr})
		case http.MethodPut:
			var body struct {
				SoftwareTitleIDs []int `json:"software_title_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			titles = append([]int{}, body.SoftwareTitleIDs...)
			putCount++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test", VerifyTLS: false})
	teamID := 1

	if err := client.SetSetupExperienceSoftwareExclude(context.Background(), &teamID, "darwin", 42); err != nil {
		t.Fatalf("Exclude(42) failed: %v", err)
	}
	if err := client.SetSetupExperienceSoftwareExclude(context.Background(), &teamID, "darwin", 12345); err != nil {
		t.Fatalf("Exclude(12345, not present) should be no-op: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(titles) != 1 || titles[0] != 99 {
		t.Errorf("expected final set to be [99], got %v", titles)
	}
	if putCount != 1 {
		t.Errorf("expected 1 PUT (for the actual removal; the second Exclude is a no-op), got %d", putCount)
	}
}
