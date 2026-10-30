# Example: FleetDM MDM end user license agreements (EULAs)
#
# Fleet keeps one EULA per platform. The file is hashed at plan time and is
# not stored in state; changing its content or name replaces the EULA.

# macOS: a PDF shown during automatic (ADE) enrollment.
resource "fleetdm_mdm_eula" "macos" {
  platform = "darwin"
  source   = "${path.module}/eula/terms.pdf"
}

# Windows (Fleet 4.94+): a Markdown file of up to 512 KB.
resource "fleetdm_mdm_eula" "windows" {
  platform = "windows"
  source   = "${path.module}/eula/terms.md"

  # Optional: the file name Fleet stores. Defaults to the base name of source.
  name = "acme-terms.md"
}
