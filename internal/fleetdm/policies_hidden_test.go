package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClient_PolicyHiddenWireShape guards the Fleet Free contract for the
// premium `hidden` field: Fleet rejects it whenever it is present, so a
// request that does not set it must leave the key out. It also checks that
// GET decodes it on both scopes.
func TestClient_PolicyHiddenWireShape(t *testing.T) {
	var bodies []map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
			}
			bodies = append(bodies, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"policy":{"id":42,"name":"p","query":"SELECT 1;","hidden":true}}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	teamID := 7

	tests := []struct {
		name string
		call func() error
		want string // raw JSON, or "" for absent
	}{
		{"global create unset", func() error {
			_, err := client.CreatePolicy(ctx, nil, CreatePolicyRequest{Name: "p", Query: "SELECT 1;"})
			return err
		}, ""},
		{"team create unset", func() error {
			_, err := client.CreatePolicy(ctx, &teamID, CreatePolicyRequest{Name: "p", Query: "SELECT 1;"})
			return err
		}, ""},
		{"global create hidden", func() error {
			_, err := client.CreatePolicy(ctx, nil, CreatePolicyRequest{Name: "p", Query: "SELECT 1;", Hidden: true})
			return err
		}, "true"},
		{"team create hidden", func() error {
			_, err := client.CreatePolicy(ctx, &teamID, CreatePolicyRequest{Name: "p", Query: "SELECT 1;", Hidden: true})
			return err
		}, "true"},
		{"global update unset", func() error {
			_, err := client.UpdatePolicy(ctx, 42, nil, UpdatePolicyRequest{Name: "p"})
			return err
		}, ""},
		{"team update unset", func() error {
			_, err := client.UpdatePolicy(ctx, 42, &teamID, UpdatePolicyRequest{Name: "p"})
			return err
		}, ""},
		{"global update clear", func() error {
			_, err := client.UpdatePolicy(ctx, 42, nil, UpdatePolicyRequest{Name: "p", Hidden: new(false)})
			return err
		}, "false"},
		{"team update set", func() error {
			_, err := client.UpdatePolicy(ctx, 42, &teamID, UpdatePolicyRequest{Name: "p", Hidden: new(true)})
			return err
		}, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodies = nil
			if err := tt.call(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(bodies) != 1 {
				t.Fatalf("expected 1 request body, got %d", len(bodies))
			}
			raw, ok := bodies[0]["hidden"]
			switch {
			case tt.want == "" && ok:
				t.Errorf("expected hidden to be absent, got %s", raw)
			case tt.want != "" && string(raw) != tt.want:
				t.Errorf("expected hidden=%s, got %q", tt.want, raw)
			}
		})
	}

	for _, scope := range []*int{nil, &teamID} {
		p, err := client.GetPolicy(ctx, 42, scope)
		if err != nil {
			t.Fatalf("get policy: %v", err)
		}
		if !p.Hidden {
			t.Errorf("expected hidden to decode as true (team=%v)", scope)
		}
	}
}
