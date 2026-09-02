package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &FleetMaintainedAppDataSource{}

// NewFleetMaintainedAppDataSource creates a new Fleet Maintained App data source.
func NewFleetMaintainedAppDataSource() datasource.DataSource {
	return &FleetMaintainedAppDataSource{}
}

// FleetMaintainedAppDataSource defines the data source implementation.
type FleetMaintainedAppDataSource struct {
	client *fleetdm.Client
}

// FleetMaintainedAppDataSourceModel describes the data source data model.
type FleetMaintainedAppDataSourceModel struct {
	ID              types.Int64  `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	TeamID          types.Int64  `tfsdk:"team_id"`
	Slug            types.String `tfsdk:"slug"`
	Platform        types.String `tfsdk:"platform"`
	Version         types.String `tfsdk:"version"`
	SoftwareTitleID types.Int64  `tfsdk:"software_title_id"`
	Filename        types.String `tfsdk:"filename"`
	InstallScript   types.String `tfsdk:"install_script"`
	UninstallScript types.String `tfsdk:"uninstall_script"`
}

func (d *FleetMaintainedAppDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_fleet_maintained_app"
}

func (d *FleetMaintainedAppDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about a specific Fleet Maintained App by name or ID.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The Fleet Maintained App ID. If specified, the app is looked up by ID.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The app name (e.g., \"1Password\", \"Google Chrome\"). Used for lookup when `id` is not specified. When set alongside `id`, it must match the resolved app's actual name — a mismatch is an error, not a hint the resolved app is wrong.",
			},
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "If specified, includes software_title_id showing whether the app is already added to that team. Works with lookup by either `id` or `name`.",
			},
			"slug": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The app slug (e.g., \"1password/darwin\").",
			},
			"platform": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The platform (`darwin`, `windows`, `linux`). Optional as an input: set it alongside `name` to disambiguate apps Fleet publishes under the same name on more than one platform (e.g. \"Mozilla Firefox\" exists for both `darwin` and `windows`) — a `name` lookup that still matches more than one app is an error asking for `platform`, not a pick-the-first guess. Not needed when looking up by `id`, which is already unique — but if set anyway, it must match the resolved app's actual platform, or the read errors instead of silently resolving in the app's favor. Always reflects the resolved app's actual platform in state.",
				Validators: []validator.String{
					stringvalidator.OneOf("darwin", "windows", "linux"),
				},
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The latest version.",
			},
			"software_title_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Set if the app is already added to the team.",
			},
			"filename": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The package filename.",
			},
			"install_script": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The default install script.",
			},
			"uninstall_script": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The default uninstall script.",
			},
		},
	}
}

func (d *FleetMaintainedAppDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *FleetMaintainedAppDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data FleetMaintainedAppDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var app *fleetdm.FleetMaintainedApp

	if !data.ID.IsNull() && !data.ID.IsUnknown() {
		// Lookup by ID
		var err error
		app, err = d.client.GetFleetMaintainedApp(ctx, int(data.ID.ValueInt64()), optionalIntPtr(data.TeamID))
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get Fleet Maintained App: %s", err))
			return
		}

		// id already pins a single, unambiguous app, so name/platform aren't
		// needed to find it — but if the practitioner set one anyway and it
		// contradicts the resolved app, that's a stale or mistaken value in
		// their config, not something to resolve silently in its favor.
		if name := optionalStringPtr(data.Name); name != nil && !strings.EqualFold(*name, app.Name) {
			resp.Diagnostics.AddError(
				"Inconsistent Attributes",
				fmt.Sprintf("id %d resolves to name %q, but the configuration specifies name %q.", app.ID, app.Name, *name),
			)
			return
		}
		if platform := optionalStringPtr(data.Platform); platform != nil && *platform != app.Platform {
			resp.Diagnostics.AddError(
				"Inconsistent Attributes",
				fmt.Sprintf("id %d resolves to platform %q, but the configuration specifies platform %q.", app.ID, app.Platform, *platform),
			)
			return
		}
	} else if !data.Name.IsNull() && !data.Name.IsUnknown() {
		// Lookup by name
		teamID := optionalIntPtr(data.TeamID)
		apps, err := d.client.ListFleetMaintainedApps(ctx, teamID)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list Fleet Maintained Apps: %s", err))
			return
		}

		// Fleet publishes some apps under the same name on several platforms, so
		// an optional platform narrows the name match to a single entry. If
		// more than one entry still matches, that's ambiguous rather than a
		// pick-the-first guess: the caller needs to add platform.
		searchName := strings.ToLower(data.Name.ValueString())
		platform := optionalStringPtr(data.Platform)
		var matches []fleetdm.FleetMaintainedApp
		for i := range apps {
			if strings.ToLower(apps[i].Name) != searchName {
				continue
			}
			if platform != nil && apps[i].Platform != *platform {
				continue
			}
			matches = append(matches, apps[i])
		}

		switch len(matches) {
		case 0:
			notFound := fmt.Sprintf("No Fleet Maintained App found with name %q", data.Name.ValueString())
			if platform != nil {
				notFound += fmt.Sprintf(" and platform %q", *platform)
			}
			resp.Diagnostics.AddError("Not Found", notFound)
			return
		case 1:
			app = &matches[0]
		default:
			platforms := make([]string, len(matches))
			for i, m := range matches {
				platforms[i] = m.Platform
			}
			resp.Diagnostics.AddError(
				"Ambiguous Name",
				fmt.Sprintf("%d Fleet Maintained Apps found with name %q, on platforms %s. Set platform to disambiguate.", len(matches), data.Name.ValueString(), strings.Join(platforms, ", ")),
			)
			return
		}
	} else {
		resp.Diagnostics.AddError("Missing Attribute", "Either 'id' or 'name' must be specified.")
		return
	}

	// Map response to model
	data.ID = types.Int64Value(int64(app.ID))
	data.Name = types.StringValue(app.Name)
	data.Slug = types.StringValue(app.Slug)
	data.Platform = types.StringValue(app.Platform)
	data.Version = types.StringValue(app.Version)
	data.Filename = types.StringValue(app.Filename)
	data.InstallScript = types.StringValue(app.InstallScript)
	data.UninstallScript = types.StringValue(app.UninstallScript)

	if app.SoftwareTitleID != nil {
		data.SoftwareTitleID = types.Int64Value(int64(*app.SoftwareTitleID))
	} else {
		data.SoftwareTitleID = types.Int64Null()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
