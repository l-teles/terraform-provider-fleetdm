# Example: Using the activities data source (audit log)

# Get all recent activities
data "fleetdm_activities" "all" {}

# Get activities filtered by type
data "fleetdm_activities" "user_logins" {
  activity_type = "user_logged_in"
}

# Get activities with query search
data "fleetdm_activities" "policy_changes" {
  query = "policy"
}

# Get activities for a specific date range
data "fleetdm_activities" "last_week" {
  created_at_start = "2024-01-01T00:00:00Z"
  created_at_end   = "2024-01-07T23:59:59Z"
}

# Get Fleet-initiated activities (automated actions)
data "fleetdm_activities" "fleet_initiated" {
  fleet_initiated = true
}

# Combined filters: user activities in a date range
data "fleetdm_activities" "recent_user_activity" {
  activity_type    = "user_created"
  created_at_start = "2024-01-01T00:00:00Z"
}

# Output activity information
output "recent_activities" {
  description = "Recent activity summary"
  value = [for a in data.fleetdm_activities.all.activities : {
    type       = a.type
    actor      = a.actor_email
    created_at = a.created_at
  }]
}

output "login_count" {
  description = "Number of login events"
  value       = length(data.fleetdm_activities.user_logins.activities)
}

# Get the most recent activity
output "latest_activity" {
  description = "Most recent activity"
  value       = length(data.fleetdm_activities.all.activities) > 0 ? data.fleetdm_activities.all.activities[0] : null
}
