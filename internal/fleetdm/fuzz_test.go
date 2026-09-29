package fleetdm

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

// FuzzEncodeAppConfiguration exercises the encode/decode pair on arbitrary
// configuration strings. Whatever the input, encoding must either fail or
// produce valid JSON, non-JSON input must survive the round trip, and a
// value must always compare equal to itself.
func FuzzEncodeAppConfiguration(f *testing.F) {
	for _, seed := range []string{
		"",
		`{"key":"value"}`,
		`[1,2,3]`,
		`"<dict/>"`,
		"<dict><key>Foo</key><string>bar</string></dict>",
		"\uFEFF{\"key\":1}",
		"  {not json",
		"[",
		"plain text",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		// Terraform strings are always valid UTF-8; encoding/json replaces
		// invalid bytes, which would break the round trip for input that
		// cannot occur.
		if !utf8.ValidString(raw) {
			t.Skip()
		}
		encoded, err := EncodeAppConfiguration(raw)
		if err != nil {
			return
		}
		if raw == "" {
			if encoded != nil {
				t.Fatalf("empty input encoded to %q, want nil", encoded)
			}
			return
		}
		if !json.Valid(encoded) {
			t.Fatalf("encoded value is not valid JSON: %q", encoded)
		}
		if !json.Valid([]byte(raw)) {
			// Non-JSON input is wrapped in a JSON string and must decode back
			// byte for byte.
			if got := DecodeAppConfiguration(encoded); got != raw {
				t.Fatalf("round trip mismatch: %q -> %q -> %q", raw, encoded, got)
			}
		}
		if !SameAppConfiguration(raw, raw) {
			t.Fatalf("value does not compare equal to itself: %q", raw)
		}
	})
}

// FuzzProfileContent feeds arbitrary bytes to the profile sniffers. The
// extension must be one of the three Fleet understands, an identifier is
// reported only together with ok, and a Windows profile never carries one.
func FuzzProfileContent(f *testing.F) {
	for _, seed := range []string{
		"",
		"\xef\xbb\xbf",
		`{"Identifier":"com.example.decl","Type":"com.apple.configuration.passcode.settings"}`,
		`{"Identifier":""}`,
		`{`,
		`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadIdentifier</key>
			<string>com.example.nested</string>
		</dict>
	</array>
	<key>PayloadIdentifier</key>
	<string>com.example.profile</string>
	<key>PayloadType</key>
	<string>Configuration</string>
</dict>
</plist>`,
		`<plist><dict><key>PayloadIdentifier</key>`,
		`<plist><dict><key>PayloadIdentifier</key><dict/></dict></plist>`,
		`<Replace><CmdID>1</CmdID><Item><Target><LocURI>./Device/Vendor/MSFT/Policy</LocURI></Target></Item></Replace>`,
		"not a profile at all",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, content []byte) {
		ext := ProfileExtensionFromContent(content)
		switch ext {
		case ".mobileconfig", ".xml", ".json":
		default:
			t.Fatalf("unexpected extension %q", ext)
		}
		id, ok := ProfileIdentifierFromContent(content)
		if ok && id == "" {
			t.Fatal("ok reported with an empty identifier")
		}
		if !ok && id != "" {
			t.Fatalf("identifier %q reported without ok", id)
		}
		if ok && ext == ".xml" {
			t.Fatalf("Windows profile reported identifier %q", id)
		}
	})
}
