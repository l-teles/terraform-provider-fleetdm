# Import a custom variable using its name (not its numeric id).
terraform import fleetdm_custom_variable.cert_password CERT_PASSWORD

# The value is NOT imported — Fleet has no endpoint that returns a custom
# variable's value. The imported state carries a null value, so the first plan
# after importing pushes the configured `value` in place. When using `value_wo`,
# set or increment `value_wo_version` after importing to push a known value.
