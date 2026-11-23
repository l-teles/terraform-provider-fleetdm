package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

var (
	_ datasource.DataSource              = &SoftwarePatchDeadlineSettingsDataSource{}
	_ datasource.DataSourceWithConfigure = &SoftwarePatchDeadlineSettingsDataSource{}
)

// NewSoftwarePatchDeadlineSettingsDataSource creates a new patch deadline settings data source.
func NewSoftwarePatchDeadlineSettingsDataSource() datasource.DataSource {
	return &SoftwarePatchDeadlineSettingsDataSource{}
}

// SoftwarePatchDeadlineSettingsDataSource reads a fleet's patch deadline settings.
type SoftwarePatchDeadlineSettingsDataSource struct {
	client *fleetdm.Client
}

// SoftwarePatchDeadlineSettingsDataSourceModel describes the data source data model.
type SoftwarePatchDeadlineSettingsDataSourceModel struct {
	FleetID types.Int64  `tfsdk:"fleet_id"`
	Days    types.List   `tfsdk:"days"`
	Time    types.String `tfsdk:"time"`
}

func (d *SoftwarePatchDeadlineSettingsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_patch_deadline_settings"
}

func (d *SoftwarePatchDeadlineSettingsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves a fleet's software patch deadline settings. A fleet whose settings were never changed reports Fleet's default (`tuesday` at `17:00`). Requires Fleet Premium and Fleet >= 4.95.",

		Attributes: map[string]schema.Attribute{
			"fleet_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The ID of the fleet (team) to read. Use `0` for hosts that are not assigned to a fleet (Unassigned).",
			},
			"days": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "The weekdays when patch deadlines fall, as lowercase names in week order (Monday first).",
			},
			"time": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The host-local time when patch deadlines fall, in 24-hour `HH:MM` format.",
			},
		},
	}
}

func (d *SoftwarePatchDeadlineSettingsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *SoftwarePatchDeadlineSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SoftwarePatchDeadlineSettingsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	settings, err := d.client.GetPatchDeadlineSettings(ctx, data.FleetID.ValueInt64())
	if err != nil {
		if patchDeadlineSettingsUnsupported(ctx, d.client, err) {
			resp.Diagnostics.AddError("Patch Deadline Settings Need Fleet 4.95", patchDeadlineSettingsUnsupportedDetail)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Patch Deadline Settings",
			fmt.Sprintf("Unable to read patch deadline settings: %s", err),
		)
		return
	}

	days, diags := types.ListValueFrom(ctx, types.StringType, settings.Days)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Days = days
	data.Time = types.StringValue(settings.Time)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
