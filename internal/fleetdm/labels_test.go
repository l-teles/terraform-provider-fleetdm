package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels" {
			t.Errorf("expected path /api/v1/fleet/labels, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListLabelsResponse{
			Labels: []Label{
				{ID: 1, Name: "All Hosts", Query: "SELECT 1"},
				{ID: 2, Name: "macOS", Query: "SELECT 1 WHERE platform = 'darwin'"},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	labels, err := client.ListLabels(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(labels) != 2 {
		t.Errorf("expected 2 labels, got: %d", len(labels))
	}

	if labels[0].Name != "All Hosts" {
		t.Errorf("expected first label name 'All Hosts', got: %s", labels[0].Name)
	}
}

func TestClient_GetLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels/1" {
			t.Errorf("expected path /api/v1/fleet/labels/1, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetLabelResponse{
			Label: Label{
				ID:          1,
				Name:        "All Hosts",
				Description: "All hosts in Fleet",
				Query:       "SELECT 1",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	label, err := client.GetLabel(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if label.ID != 1 {
		t.Errorf("expected label ID 1, got: %d", label.ID)
	}

	if label.Name != "All Hosts" {
		t.Errorf("expected label name 'All Hosts', got: %s", label.Name)
	}
}

// labelDetailWireResponse is the verbatim body Fleet 4.90.0 returns from
// GET /labels/{id} for a host vitals label built on a custom host vital, with
// `count` bumped off zero (the probe rig had no enrolled hosts).
//
// Kept as a raw string on purpose: encoding a fixture through Label makes every
// json tag round-trip symmetrically, so a wrong tag still passes. Only a
// literal body pins the field names Fleet actually sends.
const labelDetailWireResponse = `{
  "label": {
    "created_at": "2026-08-12T10:36:54Z",
    "updated_at": "2026-08-12T10:36:54Z",
    "id": 327,
    "author_id": 1,
    "name": "chv-label",
    "description": "probe criteria chv",
    "query": "",
    "criteria": {
      "value": "acme",
      "vital": "custom_host_vital",
      "operator": "=",
      "custom_host_vital_id": 42
    },
    "platform": "",
    "label_type": "regular",
    "label_membership_type": "host_vitals",
    "team_id": null,
    "team_name": null,
    "display_text": "chv-label",
    "count": 7,
    "fleet_id": null,
    "fleet_name": null
  }
}`

// TestClient_GetLabel_wireFormat pins the detail route's real field names:
// host count arrives as `count`, and a host vitals label echoes its full
// criteria plus label_membership_type = "host_vitals".
func TestClient_GetLabel_wireFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(labelDetailWireResponse))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	label, err := client.GetLabel(context.Background(), 327)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if label.HostCount != 7 {
		t.Errorf("expected HostCount 7 decoded from `count`, got: %d", label.HostCount)
	}
	if label.LabelMembershipType != "host_vitals" {
		t.Errorf("expected label_membership_type 'host_vitals', got: %q", label.LabelMembershipType)
	}
	if label.DisplayText != "chv-label" {
		t.Errorf("expected display_text 'chv-label', got: %q", label.DisplayText)
	}
	if label.CreatedAt == "" || label.UpdatedAt == "" {
		t.Errorf("expected created_at/updated_at to decode, got: %q / %q", label.CreatedAt, label.UpdatedAt)
	}

	if label.Criteria == nil {
		t.Fatal("expected criteria to decode, got nil")
	}
	if label.Criteria.Vital != HostVitalCustomHostVital {
		t.Errorf("expected vital %q, got: %q", HostVitalCustomHostVital, label.Criteria.Vital)
	}
	if label.Criteria.Value != "acme" {
		t.Errorf("expected value 'acme', got: %q", label.Criteria.Value)
	}
	if label.Criteria.Operator != HostVitalOperatorEqual {
		t.Errorf("expected operator '=', got: %q", label.Criteria.Operator)
	}
	if label.Criteria.CustomHostVitalID == nil || *label.Criteria.CustomHostVitalID != 42 {
		t.Errorf("expected custom_host_vital_id 42, got: %v", label.Criteria.CustomHostVitalID)
	}
}

// labelsListWireResponse is the verbatim body Fleet 4.90.0 returns from
// GET /labels, trimmed to one label of each membership kind. The list route is
// a full echo — it carries criteria and label_membership_type just like the
// detail route — which is what lets the plural data source skip per-label GETs.
const labelsListWireResponse = `{
  "labels": [
    {
      "created_at": "2026-08-12T10:36:35Z",
      "updated_at": "2026-08-12T10:36:35Z",
      "id": 324,
      "author_id": 1,
      "name": "manual-label",
      "description": "probe manual",
      "query": "",
      "platform": "",
      "label_type": "regular",
      "label_membership_type": "manual",
      "team_id": null,
      "display_text": "manual-label",
      "count": 2,
      "fleet_id": null
    },
    {
      "created_at": "2026-08-12T10:36:35Z",
      "updated_at": "2026-08-12T10:36:35Z",
      "id": 325,
      "author_id": 1,
      "name": "dynamic-label",
      "description": "probe dynamic",
      "query": "SELECT 1 FROM osquery_info WHERE start_time > 0;",
      "platform": "darwin",
      "label_type": "regular",
      "label_membership_type": "dynamic",
      "team_id": null,
      "display_text": "dynamic-label",
      "count": 3,
      "fleet_id": null
    },
    {
      "created_at": "2026-08-12T10:36:35Z",
      "updated_at": "2026-08-12T10:36:35Z",
      "id": 326,
      "author_id": 1,
      "name": "idp-label",
      "description": "probe criteria idp",
      "query": "",
      "criteria": {
        "value": "engineering",
        "vital": "end_user_idp_group"
      },
      "platform": "",
      "label_type": "regular",
      "label_membership_type": "host_vitals",
      "team_id": null,
      "display_text": "idp-label",
      "count": 4,
      "fleet_id": null
    }
  ]
}`

// TestClient_ListLabels_wireFormat pins the list route across all three
// membership kinds: criteria present only for the host vitals label, `operator`
// and `custom_host_vital_id` absent when they weren't set, and host count from
// `count`.
func TestClient_ListLabels_wireFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(labelsListWireResponse))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	labels, err := client.ListLabels(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got: %d", len(labels))
	}

	wantMembership := []string{"manual", "dynamic", "host_vitals"}
	wantCounts := []int{2, 3, 4}
	for i, label := range labels {
		if label.LabelMembershipType != wantMembership[i] {
			t.Errorf("label %d: expected membership %q, got: %q", i, wantMembership[i], label.LabelMembershipType)
		}
		if label.HostCount != wantCounts[i] {
			t.Errorf("label %d: expected HostCount %d decoded from `count`, got: %d", i, wantCounts[i], label.HostCount)
		}
	}

	// Manual and dynamic labels carry no criteria at all.
	for _, i := range []int{0, 1} {
		if labels[i].Criteria != nil {
			t.Errorf("label %d: expected nil criteria, got: %+v", i, labels[i].Criteria)
		}
	}

	criteria := labels[2].Criteria
	if criteria == nil {
		t.Fatal("expected criteria on the host vitals label, got nil")
	}
	if criteria.Vital != HostVitalEndUserIDPGroup {
		t.Errorf("expected vital %q, got: %q", HostVitalEndUserIDPGroup, criteria.Vital)
	}
	if criteria.Value != "engineering" {
		t.Errorf("expected value 'engineering', got: %q", criteria.Value)
	}
	// Fleet omits both of these when the label didn't set them.
	if criteria.Operator != "" {
		t.Errorf("expected empty operator, got: %q", criteria.Operator)
	}
	if criteria.CustomHostVitalID != nil {
		t.Errorf("expected nil custom_host_vital_id, got: %v", *criteria.CustomHostVitalID)
	}
}

func TestClient_CreateLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels" {
			t.Errorf("expected path /api/v1/fleet/labels, got: %s", r.URL.Path)
		}

		var req CreateLabelRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Name != "Windows Servers" {
			t.Errorf("expected name 'Windows Servers', got: %s", req.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateLabelResponse{
			Label: Label{
				ID:          3,
				Name:        req.Name,
				Description: req.Description,
				Query:       req.Query,
				Platform:    req.Platform,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	label, err := client.CreateLabel(context.Background(), CreateLabelRequest{
		Name:        "Windows Servers",
		Description: "All Windows Server machines",
		Query:       "SELECT 1 WHERE platform = 'windows'",
		Platform:    "windows",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if label.ID != 3 {
		t.Errorf("expected label ID 3, got: %d", label.ID)
	}

	if label.Name != "Windows Servers" {
		t.Errorf("expected label name 'Windows Servers', got: %s", label.Name)
	}
}

func TestClient_UpdateLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels/3" {
			t.Errorf("expected path /api/v1/fleet/labels/3, got: %s", r.URL.Path)
		}

		var req UpdateLabelRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdateLabelResponse{
			Label: Label{
				ID:          3,
				Name:        req.Name,
				Description: req.Description,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	label, err := client.UpdateLabel(context.Background(), 3, UpdateLabelRequest{
		Name:        "Windows Servers Updated",
		Description: "Updated description",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if label.Name != "Windows Servers Updated" {
		t.Errorf("expected label name 'Windows Servers Updated', got: %s", label.Name)
	}
}

func TestClient_DeleteLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels/id/3" {
			t.Errorf("expected path /api/v1/fleet/labels/id/3, got: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.DeleteLabel(context.Background(), 3)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
