package provider

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// appleRootCA is https://www.apple.com/appleca/AppleIncRootCertificate.cer,
// the root Fleet pins for Apple's Software Lookup Service. The service's chain
// is not in the usual public CA bundles.
//
//go:embed testdata/AppleIncRootCertificate.cer
var appleRootCA []byte

const appleSoftwareLookupURL = "https://gdmf.apple.com/v2/pmv"

// liveSupportedMacOSVersion returns the newest macOS version Apple's Software
// Lookup Service lists publicly. Fleet rejects an exact minimum_version that
// the service doesn't list, and Apple drops old versions, so live tests can't
// hard-code one. It skips like resource.Test when acceptance tests are off.
func liveSupportedMacOSVersion(t *testing.T) string {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	testAccPreCheck(t)

	cert, err := x509.ParseCertificate(appleRootCA)
	if err != nil {
		t.Fatalf("parsing the Apple root CA: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	// Clone the default transport to keep its proxy and dial settings.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, appleSoftwareLookupURL, nil)
	if err != nil {
		t.Fatalf("building the Apple Software Lookup Service request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("querying the Apple Software Lookup Service: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Apple Software Lookup Service returned status %d", resp.StatusCode)
	}

	var body struct {
		PublicAssetSets struct {
			MacOS []struct {
				ProductVersion string `json:"ProductVersion"`
			} `json:"macOS"`
		} `json:"PublicAssetSets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding the Apple Software Lookup Service response: %v", err)
	}
	versions := make([]string, 0, len(body.PublicAssetSets.MacOS))
	for _, asset := range body.PublicAssetSets.MacOS {
		if asset.ProductVersion != "" {
			versions = append(versions, asset.ProductVersion)
		}
	}
	if len(versions) == 0 {
		t.Fatal("the Apple Software Lookup Service listed no public macOS versions")
	}
	return slices.MaxFunc(versions, compareDottedVersions)
}

// compareDottedVersions orders versions such as "26.7.3" and "27.0"
// numerically, treating a missing component as 0.
func compareDottedVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x - y
		}
	}
	return 0
}

func TestCompareDottedVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int // sign only
	}{
		{"26.7.3", "27.2.1", -1},
		{"27.0", "26.7.3", 1},
		{"15.8", "15.8.0", 0},
		{"15.10", "15.9.9", 1},
		{"27.2.1", "27.2.1", 0},
	}
	for _, c := range cases {
		got := compareDottedVersions(c.a, c.b)
		if (got > 0) != (c.want > 0) || (got < 0) != (c.want < 0) {
			t.Errorf("compareDottedVersions(%q, %q) = %d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

// TestAppleRootCAPinned guards the embedded root against accidental replacement.
func TestAppleRootCAPinned(t *testing.T) {
	const want = "b0b1730ecbc7ff4505142c49f1295e6eda6bcaed7e2c68c5be91b5a11001f024"
	if got := fmt.Sprintf("%x", sha256.Sum256(appleRootCA)); got != want {
		t.Fatalf("embedded Apple root CA SHA-256 = %s, want %s", got, want)
	}
	if _, err := x509.ParseCertificate(appleRootCA); err != nil {
		t.Fatalf("parsing the embedded Apple root CA: %v", err)
	}
}
