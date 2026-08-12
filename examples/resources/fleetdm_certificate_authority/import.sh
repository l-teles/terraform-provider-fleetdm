# Certificate authorities are imported by their numeric Fleet id.
# The import is partial: Fleet never returns CA secrets, so both the in-state
# and the write-only secret attributes import as null and must be supplied in
# configuration. With an in-state secret the first plan after import shows a
# diff that pushes it to Fleet; with a write-only secret, set secrets_wo_version
# to push it, since Terraform cannot see a write-only value.
terraform import fleetdm_certificate_authority.scep_wifi 1
