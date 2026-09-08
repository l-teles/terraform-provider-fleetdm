package fleetdm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

// The bodies this client produced *before* the request structs were
// pointerized, captured against a live Fleet v4.90.0 and recorded here so the
// regression they caused stays legible:
//
//	name+desc     {"name":"engineering","description":"Engineering fleet"}
//	hostexpiry    {...,"host_expiry_settings":{"host_expiry_enabled":true,"host_expiry_window":30}}
//	mdm-de-true   {...,"mdm":{"enable_disk_encryption":true,"windows_require_bitlocker_pin":false}}
//	mdm-macos     {...,"mdm":{"enable_disk_encryption":false,"windows_require_bitlocker_pin":false,
//	                          "macos_updates":{"minimum_version":"26.6.1","deadline":"2026-12-01","update_new_hosts":false}}}
//	mdm-win       {...,"mdm":{"enable_disk_encryption":false,"windows_require_bitlocker_pin":false,
//	                          "windows_updates":{"deadline_days":7}}}
//
// Three defects are visible. `windows_require_bitlocker_pin:false` rode along
// on every MDM write, so an apply that only touched disk encryption reset a
// BitLocker PIN requirement enabled in the UI (verified against a live 4.90:
// the field flipped true -> false). `update_new_hosts:false` did the same
// inside macos_updates. And `grace_period_days` was dropped when it was 0,
// making the setting impossible to clear.
//
// Note that the pinned bodies below are deliberately *not* byte-identical to
// the ones above: removing those unintended keys is the fix, so byte-identity
// would mean the bug survived. What is preserved byte-for-byte is every key
// the caller actually asked for.

// captureUpdateTeamBody runs UpdateTeam against a mock server and returns the
// exact JSON bytes that were put on the wire.
//
// These are byte-level pins, not field-level assertions, on purpose: the whole
// hazard this file guards against is a *key that should not be there*. A
// field-level assertion can only check keys the test author thought about,
// whereas a byte comparison fails the moment an unintended key appears or a
// field that should be omitted starts being serialised as its zero value.
func captureUpdateTeamBody(t *testing.T, req UpdateTeamRequest) string {
	t.Helper()

	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		body = b

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(GetTeamResponse{Team: Team{ID: 1, Name: req.Name}}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if _, err := client.UpdateTeam(context.Background(), 1, req); err != nil {
		t.Fatalf("UpdateTeam returned an error: %v", err)
	}

	return string(body)
}

// TestUpdateTeamRequest_WireFormat_NoNewAttributes pins the request bodies
// produced by the configurations that existed before the Fleet 4.87-4.90
// attributes were added: name/description only, plus host expiry, plus the
// legacy top-level enable_disk_encryption.
//
// Fleet's PATCH /fleets/{id} handler assigns whole sub-structs
// (`team.Config.WebhookSettings = *payload.WebhookSettings`) and reads scalars
// through optjson, so *any* key present in the body is authoritative. A key
// serialised as its zero value because the Go field is a value type is
// therefore not inert — it silently overwrites whatever an operator set in the
// UI. These pins are what makes it safe to pointerize the request structs.
func TestUpdateTeamRequest_WireFormat_NoNewAttributes(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateTeamRequest
		want string
	}{
		{
			name: "name and description only",
			req: UpdateTeamRequest{
				Name:        "engineering",
				Description: "Engineering fleet",
			},
			want: `{"name":"engineering","description":"Engineering fleet"}`,
		},
		{
			name: "with host expiry settings",
			req: UpdateTeamRequest{
				Name:        "engineering",
				Description: "Engineering fleet",
				HostExpirySettings: &HostExpirySettings{
					HostExpiryEnabled: true,
					HostExpiryWindow:  30,
				},
			},
			want: `{"name":"engineering","description":"Engineering fleet","host_expiry_settings":{"host_expiry_enabled":true,"host_expiry_window":30}}`,
		},
		{
			// The legacy top-level `enable_disk_encryption` attribute. Before
			// pointerization this body also carried
			// `"windows_require_bitlocker_pin":false`, which cleared a
			// UI-enabled BitLocker PIN requirement on every apply. That key
			// must now be absent.
			name: "with legacy enable_disk_encryption only",
			req: UpdateTeamRequest{
				Name:        "engineering",
				Description: "Engineering fleet",
				MDM:         &TeamMDMSettings{EnableDiskEncryption: boolPtr(true)},
			},
			want: `{"name":"engineering","description":"Engineering fleet","mdm":{"enable_disk_encryption":true}}`,
		},
		{
			name: "with legacy enable_disk_encryption disabled",
			req: UpdateTeamRequest{
				Name:        "engineering",
				Description: "Engineering fleet",
				MDM:         &TeamMDMSettings{EnableDiskEncryption: boolPtr(false)},
			},
			want: `{"name":"engineering","description":"Engineering fleet","mdm":{"enable_disk_encryption":false}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := captureUpdateTeamBody(t, tt.req); got != tt.want {
				t.Errorf("request body mismatch\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

// TestUpdateTeamRequest_WireFormat_OmitsUnsetFields is the nil-vs-omitted
// matrix. For every pointerized field, setting exactly that field must produce
// a body containing exactly that key: no sibling may be dragged along as a
// zero value, because Fleet would apply it.
func TestUpdateTeamRequest_WireFormat_OmitsUnsetFields(t *testing.T) {
	base := func(mdm *TeamMDMSettings) UpdateTeamRequest {
		return UpdateTeamRequest{Name: "t", Description: "d", MDM: mdm}
	}

	tests := []struct {
		name string
		req  UpdateTeamRequest
		want string
	}{
		{
			name: "empty mdm block sends no keys",
			req:  base(&TeamMDMSettings{}),
			want: `{"name":"t","description":"d","mdm":{}}`,
		},
		{
			name: "enable_recovery_lock_password alone",
			req:  base(&TeamMDMSettings{EnableRecoveryLockPassword: boolPtr(true)}),
			want: `{"name":"t","description":"d","mdm":{"enable_recovery_lock_password":true}}`,
		},
		{
			name: "windows_require_bitlocker_pin alone",
			req:  base(&TeamMDMSettings{WindowsRequireBitlockerPIN: boolPtr(true)}),
			want: `{"name":"t","description":"d","mdm":{"windows_require_bitlocker_pin":true}}`,
		},
		{
			name: "windows_require_bitlocker_pin false is sent, not omitted",
			req:  base(&TeamMDMSettings{WindowsRequireBitlockerPIN: boolPtr(false)}),
			want: `{"name":"t","description":"d","mdm":{"windows_require_bitlocker_pin":false}}`,
		},
		{
			name: "name_template alone",
			req:  base(&TeamMDMSettings{NameTemplate: strPtr("$FLEET_VAR_HOST_HARDWARE_SERIAL")}),
			want: `{"name":"t","description":"d","mdm":{"name_template":"$FLEET_VAR_HOST_HARDWARE_SERIAL"}}`,
		},
		{
			// Clearing name_template requires an explicit empty string; a nil
			// pointer would be omitted and Fleet would keep the old value.
			name: "name_template empty string is sent, not omitted",
			req:  base(&TeamMDMSettings{NameTemplate: strPtr("")}),
			want: `{"name":"t","description":"d","mdm":{"name_template":""}}`,
		},
		{
			name: "macos_updates minimum_version and deadline without update_new_hosts",
			req: base(&TeamMDMSettings{MacOSUpdates: &AppleOSUpdates{
				MinimumVersion: strPtr("26.6.1"),
				Deadline:       strPtr("2026-12-01"),
			}}),
			want: `{"name":"t","description":"d","mdm":{"macos_updates":{"minimum_version":"26.6.1","deadline":"2026-12-01"}}}`,
		},
		{
			name: "macos_updates update_new_hosts alone",
			req: base(&TeamMDMSettings{MacOSUpdates: &AppleOSUpdates{
				UpdateNewHosts: boolPtr(true),
			}}),
			want: `{"name":"t","description":"d","mdm":{"macos_updates":{"update_new_hosts":true}}}`,
		},
		{
			// The clear path: Fleet only treats an Apple OS update setting as
			// cleared when both strings are explicitly empty.
			name: "macos_updates cleared with empty strings",
			req: base(&TeamMDMSettings{MacOSUpdates: &AppleOSUpdates{
				MinimumVersion: strPtr(""),
				Deadline:       strPtr(""),
			}}),
			want: `{"name":"t","description":"d","mdm":{"macos_updates":{"minimum_version":"","deadline":""}}}`,
		},
		{
			name: "ios_updates alone leaves macos and ipados absent",
			req: base(&TeamMDMSettings{IOSUpdates: &AppleOSUpdates{
				MinimumVersion: strPtr("26.6"),
				Deadline:       strPtr("2026-12-02"),
			}}),
			want: `{"name":"t","description":"d","mdm":{"ios_updates":{"minimum_version":"26.6","deadline":"2026-12-02"}}}`,
		},
		{
			name: "ipados_updates alone",
			req: base(&TeamMDMSettings{IPadOSUpdates: &AppleOSUpdates{
				MinimumVersion: strPtr("26.6"),
				Deadline:       strPtr("2026-12-03"),
			}}),
			want: `{"name":"t","description":"d","mdm":{"ipados_updates":{"minimum_version":"26.6","deadline":"2026-12-03"}}}`,
		},
		{
			name: "windows_updates deadline_days alone",
			req: base(&TeamMDMSettings{WindowsUpdates: &WindowsUpdates{
				DeadlineDays: intPtr(7),
			}}),
			want: `{"name":"t","description":"d","mdm":{"windows_updates":{"deadline_days":7}}}`,
		},
		{
			// Zero is a meaningful value here (it is how the setting is
			// cleared), so it must survive serialisation.
			name: "windows_updates zero values are sent, not omitted",
			req: base(&TeamMDMSettings{WindowsUpdates: &WindowsUpdates{
				DeadlineDays:    intPtr(0),
				GracePeriodDays: intPtr(0),
			}}),
			want: `{"name":"t","description":"d","mdm":{"windows_updates":{"deadline_days":0,"grace_period_days":0}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := captureUpdateTeamBody(t, tt.req); got != tt.want {
				t.Errorf("request body mismatch\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

// TestUpdateTeamRequest_WireFormat_WebhookSettings pins the webhook_settings
// body. Fleet replaces the whole block, so the sub-block pointers are the only
// thing standing between a partial config and a cleared host status webhook.
func TestUpdateTeamRequest_WireFormat_WebhookSettings(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateTeamRequest
		want string
	}{
		{
			name: "failing policies webhook only omits host status webhook",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				WebhookSettings: &TeamWebhookSettings{
					FailingPoliciesWebhook: &FailingPoliciesWebhookSettings{
						Enable:         true,
						DestinationURL: "https://example.com/fp",
						HostBatchSize:  50,
					},
				},
			},
			want: `{"name":"t","description":"d","webhook_settings":{"failing_policies_webhook":{"enable_failing_policies_webhook":true,"destination_url":"https://example.com/fp","policy_ids":null,"host_batch_size":50}}}`,
		},
		{
			name: "host status webhook only omits failing policies webhook",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				WebhookSettings: &TeamWebhookSettings{
					HostStatusWebhook: &HostStatusWebhookSettings{
						Enable:         true,
						DestinationURL: "https://example.com/hs",
						HostPercentage: 25,
						DaysCount:      7,
					},
				},
			},
			want: `{"name":"t","description":"d","webhook_settings":{"host_status_webhook":{"enable_host_status_webhook":true,"destination_url":"https://example.com/hs","host_percentage":25,"days_count":7}}}`,
		},
		{
			name: "both webhooks with policy ids",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				WebhookSettings: &TeamWebhookSettings{
					FailingPoliciesWebhook: &FailingPoliciesWebhookSettings{
						Enable:         true,
						DestinationURL: "https://example.com/fp",
						PolicyIDs:      []int64{3, 9},
					},
					HostStatusWebhook: &HostStatusWebhookSettings{
						Enable:         false,
						DestinationURL: "",
					},
				},
			},
			want: `{"name":"t","description":"d","webhook_settings":{"host_status_webhook":{"enable_host_status_webhook":false,"destination_url":"","host_percentage":0,"days_count":0},"failing_policies_webhook":{"enable_failing_policies_webhook":true,"destination_url":"https://example.com/fp","policy_ids":[3,9],"host_batch_size":0}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := captureUpdateTeamBody(t, tt.req); got != tt.want {
				t.Errorf("request body mismatch\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

// TestUpdateTeamRequest_WireFormat_IntegrationsAndFeatures pins the
// integrations and features bodies. Fleet merges both per sub-key, so omitting
// jira/zendesk leaves any globally-linked integrations untouched -- which is
// why the provider does not model them.
func TestUpdateTeamRequest_WireFormat_IntegrationsAndFeatures(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateTeamRequest
		want string
	}{
		{
			name: "google calendar only",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				Integrations: &TeamIntegrations{
					GoogleCalendar: &TeamGoogleCalendarIntegration{
						EnableCalendarEvents: true,
						WebhookURL:           "https://example.com/cal",
					},
				},
			},
			want: `{"name":"t","description":"d","integrations":{"google_calendar":{"enable_calendar_events":true,"webhook_url":"https://example.com/cal"}}}`,
		},
		{
			name: "conditional access only",
			req: UpdateTeamRequest{
				Name:         "t",
				Description:  "d",
				Integrations: &TeamIntegrations{ConditionalAccessEnabled: boolPtr(false)},
			},
			want: `{"name":"t","description":"d","integrations":{"conditional_access_enabled":false}}`,
		},
		{
			name: "historical data vulnerabilities only",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				Features: &TeamFeatures{
					HistoricalData: &HistoricalDataSettings{Vulnerabilities: boolPtr(false)},
				},
			},
			want: `{"name":"t","description":"d","features":{"historical_data":{"vulnerabilities":false}}}`,
		},
		{
			name: "historical data both sub keys",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				Features: &TeamFeatures{
					HistoricalData: &HistoricalDataSettings{
						Uptime:          boolPtr(true),
						Vulnerabilities: boolPtr(true),
					},
				},
			},
			want: `{"name":"t","description":"d","features":{"historical_data":{"uptime":true,"vulnerabilities":true}}}`,
		},
		{
			// enable_host_users and enable_software_inventory are read-only on
			// this endpoint (Fleet's TeamPayloadFeatures only carries
			// historical_data), so they must never be serialised into a PATCH.
			name: "read only feature flags are never sent",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				Features:    &TeamFeatures{},
			},
			want: `{"name":"t","description":"d","features":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := captureUpdateTeamBody(t, tt.req); got != tt.want {
				t.Errorf("request body mismatch\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

// TestUpdateTeamRequest_WireFormat_Fleet491 pins the bodies for the fields
// Fleet 4.91 added to PATCH /fleets/{id}. Two of them carry constraints that
// only the wire format can express:
//
//   - deadline_days must be absent, not 0, when it is unset: Fleet rejects
//     `deadline_days: 0` alongside an exact minimum_version, and omitting the
//     key is how a fleet migrates off minimum_version "latest".
//   - windows_settings must never carry custom_settings, so a PATCH cannot
//     clobber configuration profiles managed elsewhere.
func TestUpdateTeamRequest_WireFormat_Fleet491(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	int64Ptr := func(i int64) *int64 { return &i }
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name string
		req  UpdateTeamRequest
		want string
	}{
		{
			name: "host activities webhook",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				WebhookSettings: &TeamWebhookSettings{
					HostActivitiesWebhook: &HostActivitiesWebhookSettings{
						Enable:         true,
						DestinationURL: "https://example.com/activities",
					},
				},
			},
			want: `{"name":"t","description":"d","webhook_settings":{"host_activities_webhook":{"enable_host_activities_webhook":true,"destination_url":"https://example.com/activities"}}}`,
		},
		{
			name: "apple updates latest carries deadline_days and no deadline",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				MDM: &TeamMDMSettings{
					MacOSUpdates: &AppleOSUpdates{
						MinimumVersion: strPtr("latest"),
						DeadlineDays:   int64Ptr(7),
					},
				},
			},
			want: `{"name":"t","description":"d","mdm":{"macos_updates":{"minimum_version":"latest","deadline_days":7}}}`,
		},
		{
			name: "exact version omits deadline_days entirely rather than sending 0",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				MDM: &TeamMDMSettings{
					MacOSUpdates: &AppleOSUpdates{
						MinimumVersion: strPtr("26.6.1"),
						Deadline:       strPtr("2027-01-01"),
					},
				},
			},
			want: `{"name":"t","description":"d","mdm":{"macos_updates":{"minimum_version":"26.6.1","deadline":"2027-01-01"}}}`,
		},
		{
			name: "windows settings sends only the managed local account toggle",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				MDM: &TeamMDMSettings{
					WindowsSettings: &WindowsMDMSettings{
						EnableManagedLocalAccount: boolPtr(true),
					},
				},
			},
			want: `{"name":"t","description":"d","mdm":{"windows_settings":{"enable_managed_local_account":true}}}`,
		},
		{
			name: "features enable_software_inventory false is sent, not omitted",
			req: UpdateTeamRequest{
				Name:        "t",
				Description: "d",
				Features: &TeamFeatures{
					EnableSoftwareInventory: boolPtr(false),
				},
			},
			want: `{"name":"t","description":"d","features":{"enable_software_inventory":false}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := captureUpdateTeamBody(t, tt.req); got != tt.want {
				t.Errorf("request body mismatch\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}
