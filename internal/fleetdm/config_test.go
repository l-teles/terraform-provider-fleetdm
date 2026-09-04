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
		OrgInfo: &OrgInfoUpdate{OrgName: "Updated Org"},
	})
	if err != nil {
		t.Fatalf("UpdateAppConfig failed: %v", err)
	}
	if config.OrgInfo.OrgName != "Updated Org" {
		t.Errorf("Expected org name 'Updated Org', got '%s'", config.OrgInfo.OrgName)
	}
}

// TestUpdateAppConfigHostNameTemplate pins the wire shape of the global ("No
// team") host name template in PATCH /config. Fleet merges the request body onto
// the stored config, so an omitted mdm object must mean "leave every MDM setting
// alone", while an explicitly empty template must still be serialized — that
// empty string is how the template is cleared.
func TestUpdateAppConfigHostNameTemplate(t *testing.T) {
	tests := []struct {
		name string
		mdm  *MDMSettingsUpdate
		// want is the expected "mdm" fragment of the request body, or "" when the
		// key must be absent entirely.
		want string
	}{
		{
			name: "unmanaged template omits the mdm object",
			mdm:  nil,
			want: "",
		},
		{
			name: "template is sent verbatim",
			mdm:  &MDMSettingsUpdate{NameTemplate: strPtr("host-$FLEET_VAR_HOST_HARDWARE_SERIAL")},
			want: `{"name_template":"host-$FLEET_VAR_HOST_HARDWARE_SERIAL"}`,
		},
		{
			// Clearing the template requires an explicit empty string; a nil pointer
			// would omit the key and leave Fleet's current template in place.
			name: "empty string is sent, not omitted",
			mdm:  &MDMSettingsUpdate{NameTemplate: strPtr("")},
			want: `{"name_template":""}`,
		},
		{
			name: "windows automatic enrollment alone omits name_template",
			mdm: &MDMSettingsUpdate{
				WindowsAutomaticEnrollment: &WindowsAutomaticEnrollment{DefaultFleet: "onboarding"},
			},
			want: `{"windows_automatic_enrollment":{"default_fleet":"onboarding"}}`,
		},
		{
			// Fleet accepts one mdm object per request, so both managed keys have
			// to survive being merged into it.
			name: "both managed keys share the one mdm object",
			mdm: &MDMSettingsUpdate{
				NameTemplate:               strPtr("host-$FLEET_VAR_HOST_UUID"),
				WindowsAutomaticEnrollment: &WindowsAutomaticEnrollment{DefaultFleet: "onboarding"},
			},
			want: `{"name_template":"host-$FLEET_VAR_HOST_UUID","windows_automatic_enrollment":{"default_fleet":"onboarding"}}`,
		},
		{
			// "" is how Fleet expresses "no default fleet", so it must go on the
			// wire rather than being omitted as a zero value.
			name: "empty default fleet is sent, not omitted",
			mdm: &MDMSettingsUpdate{
				WindowsAutomaticEnrollment: &WindowsAutomaticEnrollment{DefaultFleet: ""},
			},
			want: `{"windows_automatic_enrollment":{"default_fleet":""}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/fleet/config" {
					t.Errorf("expected PATCH /api/v1/fleet/config, got %s %s", r.Method, r.URL.Path)
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decoding request body: %v", err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(AppConfig{}); err != nil {
					t.Errorf("encoding response: %v", err)
				}
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-token", VerifyTLS: false})
			if err != nil {
				t.Fatalf("NewClient failed: %v", err)
			}

			if _, err := client.UpdateAppConfig(context.Background(), &UpdateAppConfigRequest{
				OrgInfo: &OrgInfoUpdate{OrgName: "Org"},
				MDM:     tt.mdm,
			}); err != nil {
				t.Fatalf("UpdateAppConfig failed: %v", err)
			}

			raw, present := body["mdm"]
			if tt.want == "" {
				if present {
					t.Errorf("expected no mdm key in the request body, got %s", raw)
				}
				return
			}
			if !present {
				t.Fatal("expected an mdm key in the request body, none sent")
			}
			if string(raw) != tt.want {
				t.Errorf("mdm fragment = %s, want %s", raw, tt.want)
			}
		})
	}
}

// TestGetAppConfigHostNameTemplate verifies the global host name template is
// read from GET /config at mdm.name_template.
func TestGetAppConfigHostNameTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"mdm":{"name_template":"host-$FLEET_VAR_HOST_UUID","enabled_and_configured":true}}`)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-token", VerifyTLS: false})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	config, err := client.GetAppConfig(context.Background())
	if err != nil {
		t.Fatalf("GetAppConfig failed: %v", err)
	}
	if config.MDM == nil {
		t.Fatal("expected an MDM section in the parsed config")
	}
	if got, want := config.MDM.NameTemplate, "host-$FLEET_VAR_HOST_UUID"; got != want {
		t.Errorf("NameTemplate = %q, want %q", got, want)
	}
}
