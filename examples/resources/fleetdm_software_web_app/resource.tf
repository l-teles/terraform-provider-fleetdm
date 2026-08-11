# Create an Android web app (web clip). Requires Fleet Premium with Android MDM
# enabled and configured.
#
# Creating the web app only registers it in your Android enterprise. To make it
# installable on a team, pass the resulting app_store_id to
# fleetdm_software_app_store_app with platform = "android".

resource "fleetdm_software_web_app" "support_portal" {
  title = "Support Portal"
  url   = "https://support.example.com"
}

resource "fleetdm_software_app_store_app" "support_portal" {
  app_store_id = fleetdm_software_web_app.support_portal.app_store_id
  team_id      = fleetdm_team.field_devices.id
  platform     = "android"
  self_service = true
}

# With a custom icon. Fleet requires a square PNG of at least 512x512px.
resource "fleetdm_software_web_app" "timesheets" {
  title     = "Timesheets"
  url       = "https://timesheets.example.com"
  icon_path = "${path.module}/icons/timesheets-512.png"
}
