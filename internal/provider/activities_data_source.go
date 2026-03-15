package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ActivitiesDataSource{}

// NewActivitiesDataSource creates a new activities data source.
func NewActivitiesDataSource() datasource.DataSource {
	return &ActivitiesDataSource{}
}

// ActivitiesDataSource defines the data source implementation.
type ActivitiesDataSource struct {
	client *fleetdm.Client
}

// ActivityModel describes the data model for a single activity.
type ActivityModel struct {
	ID             types.Int64  `tfsdk:"id"`
	CreatedAt      types.String `tfsdk:"created_at"`
	ActorFullName  types.String `tfsdk:"actor_full_name"`
	ActorID        types.Int64  `tfsdk:"actor_id"`
	ActorGravatar  types.String `tfsdk:"actor_gravatar"`
	ActorEmail     types.String `tfsdk:"actor_email"`
	Type           types.String `tfsdk:"type"`
	FleetInitiated types.Bool   `tfsdk:"fleet_initiated"`
}

// ActivitiesDataSourceModel describes the data source data model.
type ActivitiesDataSourceModel struct {
	Query          types.String    `tfsdk:"query"`
	ActivityType   types.String    `tfsdk:"activity_type"`
	StartCreatedAt types.String    `tfsdk:"start_created_at"`
	EndCreatedAt   types.String    `tfsdk:"end_created_at"`
	PerPage        types.Int64     `tfsdk:"per_page"`
	Activities     []ActivityModel `tfsdk:"activities"`
}

// Metadata returns the data source type name.
func (d *ActivitiesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_activities"
}

// Schema defines the schema for the data source.
func (d *ActivitiesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves the list of activities (audit log) from FleetDM.",
		MarkdownDescription: `Retrieves the list of activities (audit log) from FleetDM.

Activities are an audit log of actions performed in Fleet. This data source allows you to query recent activities for monitoring and compliance purposes.

## Example Usage

### List Recent Activities

` + "```hcl" + `
data "fleetdm_activities" "recent" {}
` + "```" + `

### Filter by Activity Type

` + "```hcl" + `
data "fleetdm_activities" "user_logins" {
  activity_type = "user_logged_in"
  per_page      = 50
}
` + "```" + `

### Filter by Date Range

` + "```hcl" + `
data "fleetdm_activities" "last_week" {
  start_created_at = "2024-01-01T00:00:00Z"
  end_created_at   = "2024-01-07T23:59:59Z"
}
` + "```",

		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				Description:         "Search query keywords. Searchable fields include actor_full_name and actor_email.",
				MarkdownDescription: "Search query keywords. Searchable fields include `actor_full_name` and `actor_email`.",
				Optional:            true,
			},
			"activity_type": schema.StringAttribute{
				Description:         "Filter by activity type (e.g., user_logged_in, created_team, etc.).",
				MarkdownDescription: "Filter by activity type (e.g., `user_logged_in`, `created_team`, etc.).",
				Optional:            true,
			},
			"start_created_at": schema.StringAttribute{
				Description:         "Filter to include only activities that happened after this date (ISO 8601 format).",
				MarkdownDescription: "Filter to include only activities that happened after this date (ISO 8601 format).",
				Optional:            true,
			},
			"end_created_at": schema.StringAttribute{
				Description:         "Filter to include only activities that happened before this date (ISO 8601 format).",
				MarkdownDescription: "Filter to include only activities that happened before this date (ISO 8601 format).",
				Optional:            true,
			},
			"per_page": schema.Int64Attribute{
				Description:         "Maximum number of activities to return. Default is 20.",
				MarkdownDescription: "Maximum number of activities to return. Default is 20.",
				Optional:            true,
			},
			"activities": schema.ListNestedAttribute{
				Description:         "The list of activities.",
				MarkdownDescription: "The list of activities.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description:         "The unique identifier of the activity.",
							MarkdownDescription: "The unique identifier of the activity.",
							Computed:            true,
						},
						"created_at": schema.StringAttribute{
							Description:         "When the activity occurred.",
							MarkdownDescription: "When the activity occurred.",
							Computed:            true,
						},
						"actor_full_name": schema.StringAttribute{
							Description:         "The full name of the user who performed the action.",
							MarkdownDescription: "The full name of the user who performed the action.",
							Computed:            true,
						},
						"actor_id": schema.Int64Attribute{
							Description:         "The ID of the user who performed the action.",
							MarkdownDescription: "The ID of the user who performed the action.",
							Computed:            true,
						},
						"actor_gravatar": schema.StringAttribute{
							Description:         "The Gravatar URL for the actor.",
							MarkdownDescription: "The Gravatar URL for the actor.",
							Computed:            true,
						},
						"actor_email": schema.StringAttribute{
							Description:         "The email address of the actor.",
							MarkdownDescription: "The email address of the actor.",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							Description:         "The type of activity.",
							MarkdownDescription: "The type of activity.",
							Computed:            true,
						},
						"fleet_initiated": schema.BoolAttribute{
							Description:         "Whether the activity was initiated by Fleet automatically.",
							MarkdownDescription: "Whether the activity was initiated by Fleet automatically.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ActivitiesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *ActivitiesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ActivitiesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading activities")

	// Build options from config
	opts := &fleetdm.ListActivitiesOptions{}

	if !config.Query.IsNull() {
		opts.Query = config.Query.ValueString()
	}
	if !config.ActivityType.IsNull() {
		opts.ActivityType = config.ActivityType.ValueString()
	}
	if !config.StartCreatedAt.IsNull() {
		opts.StartCreatedAt = config.StartCreatedAt.ValueString()
	}
	if !config.EndCreatedAt.IsNull() {
		opts.EndCreatedAt = config.EndCreatedAt.ValueString()
	}
	if !config.PerPage.IsNull() {
		opts.PerPage = int(config.PerPage.ValueInt64())
	}

	// Default sort by created_at descending (most recent first)
	opts.OrderKey = "created_at"
	opts.OrderDirection = "desc"

	activities, err := d.client.ListActivities(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Activities",
			"Could not read activities: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Activities read", map[string]interface{}{
		"count": len(activities),
	})

	// Map response to model
	config.Activities = make([]ActivityModel, len(activities))
	for i, activity := range activities {
		config.Activities[i] = ActivityModel{
			ID:             types.Int64Value(int64(activity.ID)),
			CreatedAt:      types.StringValue(activity.CreatedAt),
			ActorFullName:  types.StringValue(activity.ActorFullName),
			ActorGravatar:  types.StringValue(activity.ActorGravatar),
			ActorEmail:     types.StringValue(activity.ActorEmail),
			Type:           types.StringValue(activity.Type),
			FleetInitiated: types.BoolValue(activity.FleetInitiated),
		}

		config.Activities[i].ActorID = intPtrToInt64(activity.ActorID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
