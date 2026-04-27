package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListHosts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/hosts" {
			t.Errorf("expected path /api/v1/fleet/hosts, got %s", r.URL.Path)
		}

		// Check query parameters
		query := r.URL.Query()
		if query.Get("status") != "online" {
			t.Errorf("expected status=online, got %s", query.Get("status"))
		}
		if query.Get("team_id") != "1" {
			t.Errorf("expected team_id=1, got %s", query.Get("team_id"))
		}

		response := map[string]interface{}{
			"hosts": []map[string]interface{}{
				{
					"id":              1,
					"uuid":            "abc-123",
					"hostname":        "host1.example.com",
					"display_name":    "Host 1",
					"platform":        "darwin",
					"os_version":      "macOS 14.0",
					"hardware_vendor": "Apple Inc.",
					"hardware_model":  "MacBookPro18,1",
					"hardware_serial": "C02ABC123456",
					"primary_ip":      "192.168.1.100",
					"status":          "online",
					"team_id":         1,
					"team_name":       "Workstations",
					"seen_time":       time.Now().Format(time.RFC3339),
					"created_at":      time.Now().Format(time.RFC3339),
					"updated_at":      time.Now().Format(time.RFC3339),
				},
				{
					"id":              2,
					"uuid":            "def-456",
					"hostname":        "host2.example.com",
					"display_name":    "Host 2",
					"platform":        "darwin",
					"os_version":      "macOS 13.5",
					"hardware_vendor": "Apple Inc.",
					"hardware_model":  "MacBookPro17,1",
					"hardware_serial": "C02DEF789012",
					"primary_ip":      "192.168.1.101",
					"status":          "online",
					"team_id":         1,
					"team_name":       "Workstations",
					"seen_time":       time.Now().Format(time.RFC3339),
					"created_at":      time.Now().Format(time.RFC3339),
					"updated_at":      time.Now().Format(time.RFC3339),
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     true,
		Timeout:       30,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	hosts, err := client.ListHosts(context.Background(), ListHostsOptions{
		Status: "online",
		TeamID: 1,
	})
	if err != nil {
		t.Fatalf("ListHosts failed: %v", err)
	}

	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}

	if hosts[0].Hostname != "host1.example.com" {
		t.Errorf("expected hostname host1.example.com, got %s", hosts[0].Hostname)
	}
	if hosts[0].UUID != "abc-123" {
		t.Errorf("expected UUID abc-123, got %s", hosts[0].UUID)
	}
	if hosts[0].Platform != "darwin" {
		t.Errorf("expected platform darwin, got %s", hosts[0].Platform)
	}
}

func TestGetHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/hosts/1" {
			t.Errorf("expected path /api/v1/fleet/hosts/1, got %s", r.URL.Path)
		}

		response := map[string]interface{}{
			"host": map[string]interface{}{
				"id":                           1,
				"uuid":                         "abc-123",
				"hostname":                     "host1.example.com",
				"display_name":                 "Host 1",
				"computer_name":                "Host-1",
				"platform":                     "darwin",
				"os_version":                   "macOS 14.0",
				"build":                        "23A344",
				"platform_like":                "darwin",
				"cpu_type":                     "arm64e",
				"cpu_brand":                    "Apple M1 Pro",
				"cpu_physical_cores":           10,
				"cpu_logical_cores":            10,
				"hardware_vendor":              "Apple Inc.",
				"hardware_model":               "MacBookPro18,1",
				"hardware_serial":              "C02ABC123456",
				"primary_ip":                   "192.168.1.100",
				"primary_mac":                  "a1:b2:c3:d4:e5:f6",
				"public_ip":                    "203.0.113.1",
				"memory":                       34359738368,
				"status":                       "online",
				"team_id":                      1,
				"team_name":                    "Workstations",
				"gigs_disk_space_available":    120.5,
				"percent_disk_space_available": 48.2,
				"seen_time":                    time.Now().Format(time.RFC3339),
				"created_at":                   time.Now().Format(time.RFC3339),
				"updated_at":                   time.Now().Format(time.RFC3339),
				"labels": []map[string]interface{}{
					{
						"id":   1,
						"name": "macOS",
					},
				},
				"policies": []map[string]interface{}{
					{
						"id":       1,
						"name":     "Disk Encryption",
						"response": "pass",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     true,
		Timeout:       30,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	host, err := client.GetHost(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetHost failed: %v", err)
	}

	if host.ID != 1 {
		t.Errorf("expected ID 1, got %d", host.ID)
	}
	if host.Hostname != "host1.example.com" {
		t.Errorf("expected hostname host1.example.com, got %s", host.Hostname)
	}
	if host.HardwareSerial != "C02ABC123456" {
		t.Errorf("expected serial C02ABC123456, got %s", host.HardwareSerial)
	}
	if host.CPUBrand != "Apple M1 Pro" {
		t.Errorf("expected CPU Apple M1 Pro, got %s", host.CPUBrand)
	}
	if host.Memory != 34359738368 {
		t.Errorf("expected memory 34359738368, got %d", host.Memory)
	}
}

func TestGetHostByIdentifier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/hosts/identifier/C02ABC123456" {
			t.Errorf("expected path /api/v1/fleet/hosts/identifier/C02ABC123456, got %s", r.URL.Path)
		}

		response := map[string]interface{}{
			"host": map[string]interface{}{
				"id":              1,
				"uuid":            "abc-123",
				"hostname":        "host1.example.com",
				"hardware_serial": "C02ABC123456",
				"platform":        "darwin",
				"status":          "online",
				"seen_time":       time.Now().Format(time.RFC3339),
				"created_at":      time.Now().Format(time.RFC3339),
				"updated_at":      time.Now().Format(time.RFC3339),
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     true,
		Timeout:       30,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	host, err := client.GetHostByIdentifier(context.Background(), "C02ABC123456")
	if err != nil {
		t.Fatalf("GetHostByIdentifier failed: %v", err)
	}

	if host.ID != 1 {
		t.Errorf("expected ID 1, got %d", host.ID)
	}
	if host.HardwareSerial != "C02ABC123456" {
		t.Errorf("expected serial C02ABC123456, got %s", host.HardwareSerial)
	}
}
