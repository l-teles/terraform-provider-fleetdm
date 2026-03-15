package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAppConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/config" {
			t.Errorf("Expected path /api/v1/fleet/config, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		response := AppConfig{
			OrgInfo: OrgInfo{
				OrgName:    "Test Organization",
				OrgLogoURL: "https://example.com/logo.png",
				ContactURL: "https://example.com/contact",
			},
			ServerSettings: ServerSettings{
				ServerURL:            "https://fleet.example.com",
				LiveQueryDisabled:    false,
				EnableAnalytics:      true,
				QueryReportsDisabled: false,
				ScriptsDisabled:      false,
			},
			HostExpirySettings: HostExpirySettings{
				HostExpiryEnabled: true,
				HostExpiryWindow:  30,
			},
			ActivityExpirySettings: ActivityExpirySettings{
				ActivityExpiryEnabled: true,
				ActivityExpiryWindow:  90,
			},
			Features: Features{
				EnableHostUsers:         true,
				EnableSoftwareInventory: true,
			},
			FleetDesktop: FleetDesktopSettings{
				TransparencyURL: "https://example.com/transparency",
			},
			License: &LicenseInfo{
				Tier:         "premium",
				Organization: "Test Org",
				DeviceCount:  1000,
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	config, err := client.GetAppConfig(context.Background())
	if err != nil {
		t.Fatalf("GetAppConfig failed: %v", err)
	}

	if config.OrgInfo.OrgName != "Test Organization" {
		t.Errorf("Expected org name 'Test Organization', got '%s'", config.OrgInfo.OrgName)
	}

	if config.ServerSettings.ServerURL != "https://fleet.example.com" {
		t.Errorf("Expected server URL 'https://fleet.example.com', got '%s'", config.ServerSettings.ServerURL)
	}

	if !config.HostExpirySettings.HostExpiryEnabled {
		t.Error("Expected host expiry to be enabled")
	}

	if config.HostExpirySettings.HostExpiryWindow != 30 {
		t.Errorf("Expected host expiry window 30, got %d", config.HostExpirySettings.HostExpiryWindow)
	}

	if config.License == nil || config.License.Tier != "premium" {
		t.Error("Expected premium license tier")
	}
}

func TestGetEnrollSecretSpec(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/spec/enroll_secret" {
			t.Errorf("Expected path /api/v1/fleet/spec/enroll_secret, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		response := map[string]interface{}{
			"spec": map[string]interface{}{
				"secrets": []map[string]interface{}{
					{
						"secret":     "secret-1-abc123",
						"created_at": "2024-01-15T10:30:00Z",
					},
					{
						"secret":     "secret-2-def456",
						"created_at": "2024-01-10T08:00:00Z",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	spec, err := client.GetEnrollSecretSpec(context.Background())
	if err != nil {
		t.Fatalf("GetEnrollSecretSpec failed: %v", err)
	}

	if len(spec.Secrets) != 2 {
		t.Errorf("Expected 2 secrets, got %d", len(spec.Secrets))
	}

	if spec.Secrets[0].Secret != "secret-1-abc123" {
		t.Errorf("Expected first secret 'secret-1-abc123', got '%s'", spec.Secrets[0].Secret)
	}

	if spec.Secrets[1].Secret != "secret-2-def456" {
		t.Errorf("Expected second secret 'secret-2-def456', got '%s'", spec.Secrets[1].Secret)
	}
}

func TestApplyEnrollSecretSpec(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/spec/enroll_secret" {
			t.Errorf("Expected path /api/v1/fleet/spec/enroll_secret, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		var request struct {
			Spec EnrollSecretSpec `json:"spec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if len(request.Spec.Secrets) != 1 {
			t.Errorf("Expected 1 secret in request, got %d", len(request.Spec.Secrets))
		}

		if request.Spec.Secrets[0].Secret != "new-secret-xyz" {
			t.Errorf("Expected secret 'new-secret-xyz', got '%s'", request.Spec.Secrets[0].Secret)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	spec := &EnrollSecretSpec{
		Secrets: []EnrollSecret{
			{Secret: "new-secret-xyz"},
		},
	}

	err = client.ApplyEnrollSecretSpec(context.Background(), spec)
	if err != nil {
		t.Fatalf("ApplyEnrollSecretSpec failed: %v", err)
	}
}

func TestGetAppConfigWithWebhooks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"org_info": map[string]interface{}{
				"org_name": "Webhook Test Org",
			},
			"server_settings": map[string]interface{}{
				"server_url": "https://fleet.example.com",
			},
			"host_expiry_settings": map[string]interface{}{
				"host_expiry_enabled": false,
				"host_expiry_window":  0,
			},
			"activity_expiry_settings": map[string]interface{}{
				"activity_expiry_enabled": false,
				"activity_expiry_window":  0,
			},
			"features": map[string]interface{}{
				"enable_host_users":         true,
				"enable_software_inventory": false,
			},
			"fleet_desktop": map[string]interface{}{
				"transparency_url": "",
			},
			"webhook_settings": map[string]interface{}{
				"host_status_webhook": map[string]interface{}{
					"enable_host_status_webhook": true,
					"destination_url":            "https://webhook.example.com/host-status",
					"host_percentage":            10.0,
					"days_count":                 7,
				},
				"failing_policies_webhook": map[string]interface{}{
					"enable_failing_policies_webhook": true,
					"destination_url":                 "https://webhook.example.com/policies",
					"policy_ids":                      []int{1, 2, 3},
					"host_batch_size":                 100,
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	config, err := client.GetAppConfig(context.Background())
	if err != nil {
		t.Fatalf("GetAppConfig failed: %v", err)
	}

	if config.WebhookSettings.HostStatusWebhook == nil {
		t.Fatal("Expected host status webhook to be present")
	}

	if !config.WebhookSettings.HostStatusWebhook.Enable {
		t.Error("Expected host status webhook to be enabled")
	}

	if config.WebhookSettings.HostStatusWebhook.DestinationURL != "https://webhook.example.com/host-status" {
		t.Errorf("Expected host status webhook URL 'https://webhook.example.com/host-status', got '%s'",
			config.WebhookSettings.HostStatusWebhook.DestinationURL)
	}

	if config.WebhookSettings.FailingPoliciesWebhook == nil {
		t.Fatal("Expected failing policies webhook to be present")
	}

	if !config.WebhookSettings.FailingPoliciesWebhook.Enable {
		t.Error("Expected failing policies webhook to be enabled")
	}
}

func TestUpdateAppConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/config" {
			t.Errorf("Expected path /api/v1/fleet/config, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPatch {
			t.Errorf("Expected PATCH method, got %s", r.Method)
		}

		var req UpdateAppConfigRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.OrgInfo == nil || req.OrgInfo.OrgName != "Updated Org" {
			t.Errorf("Expected org name 'Updated Org'")
		}

		response := AppConfig{
			OrgInfo: OrgInfo{
				OrgName: "Updated Org",
			},
			ServerSettings: ServerSettings{
				ServerURL: "https://fleet.example.com",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-token", VerifyTLS: false})

	config, err := client.UpdateAppConfig(context.Background(), &UpdateAppConfigRequest{
		OrgInfo: &OrgInfo{OrgName: "Updated Org"},
	})
	if err != nil {
		t.Fatalf("UpdateAppConfig failed: %v", err)
	}
	if config.OrgInfo.OrgName != "Updated Org" {
		t.Errorf("Expected org name 'Updated Org', got '%s'", config.OrgInfo.OrgName)
	}
}
