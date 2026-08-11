# Import a self-service category using "fleet_id:id".
# Both parts are required because Fleet only exposes a fleet-scoped list endpoint.
terraform import fleetdm_software_self_service_category.engineering 3:42

# Categories on hosts that are not assigned to a fleet use fleet_id 0
terraform import fleetdm_software_self_service_category.unassigned_design 0:7
