# Define a slot for an asset tag that Fleet cannot collect itself.
# Per-host values are pushed in out-of-band with:
#   PUT /api/v1/fleet/hosts/{host_id}/custom_host_vitals/{id}
resource "fleetdm_custom_host_vital" "asset_tag" {
  name = "Asset tag"
}

resource "fleetdm_custom_host_vital" "cost_centre" {
  name = "Cost centre"
}

# Build a label whose membership follows the vital's value. Fleet cannot change
# a label's criteria after creation, so edits here replace the label.
resource "fleetdm_label" "leased_hardware" {
  name        = "Leased hardware"
  description = "Hosts whose asset tag marks them as leased"

  criteria = {
    vital                = "custom_host_vital"
    operator             = "LIKE"
    value                = "LEASE-%"
    custom_host_vital_id = fleetdm_custom_host_vital.asset_tag.id
  }
}

# A vital's per-host value can also be interpolated into a configuration
# profile, script or software installer as $FLEET_HOST_VITAL_<id>, which Fleet
# expands per host at delivery time. Referencing the resource's id keeps the
# token correct and makes Terraform create the vital before the profile.
resource "fleetdm_configuration_profile" "asset_tag" {
  team_id = 1

  profile_content = <<-XML
    <?xml version="1.0" encoding="UTF-8"?>
    <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
    <plist version="1.0">
      <dict>
        <key>PayloadIdentifier</key>
        <string>com.example.assettag</string>
        <key>PayloadType</key>
        <string>Configuration</string>
        <key>PayloadUUID</key>
        <string>0b9b1c5a-2f4d-4d8e-9c1a-8f2b7d3e5a61</string>
        <key>PayloadVersion</key>
        <integer>1</integer>
        <key>PayloadDisplayName</key>
        <string>Asset tag</string>
        <key>PayloadContent</key>
        <array>
          <dict>
            <key>PayloadIdentifier</key>
            <string>com.example.assettag.payload</string>
            <key>PayloadType</key>
            <string>com.example.assettag</string>
            <key>PayloadUUID</key>
            <string>7d1e4c33-6a58-42b7-9f0d-1c4e8a2b6f90</string>
            <key>PayloadVersion</key>
            <integer>1</integer>
            <key>AssetTag</key>
            <string>$FLEET_HOST_VITAL_${fleetdm_custom_host_vital.asset_tag.id}</string>
          </dict>
        </array>
      </dict>
    </plist>
  XML
}
