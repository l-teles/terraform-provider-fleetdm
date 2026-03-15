package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &VersionDataSource{}

// NewVersionDataSource creates a new version data source.
func NewVersionDataSource() datasource.DataSource {
	return &VersionDataSource{}
}

// VersionDataSource defines the data source implementation.
type VersionDataSource struct {
	client *fleetdm.Client
}

// VersionDataSourceModel describes the data source data model.
type VersionDataSourceModel struct {
	Version   types.String `tfsdk:"version"`
	Branch    types.String `tfsdk:"branch"`
	Revision  types.String `tfsdk:"revision"`
	GoVersion types.String `tfsdk:"go_version"`
	BuildDate types.String `tfsdk:"build_date"`
	BuildUser types.String `tfsdk:"build_user"`
}

func (d *VersionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_version"
}

func (d *VersionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve version information about the FleetDM server.",

		Attributes: map[string]schema.Attribute{
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The version of the FleetDM server.",
			},
			"branch": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The git branch from which the server was built.",
			},
			"revision": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The git revision (commit hash) from which the server was built.",
			},
			"go_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Go version used to build the server.",
			},
			"build_date": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The date and time when the server was built.",
			},
			"build_user": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The user who built the server.",
			},
		},
	}
}

func (d *VersionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *VersionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data VersionDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	version, err := d.client.GetVersion(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get version: %s", err))
		return
	}

	// Map response to model
	data.Version = types.StringValue(version.Version)
	data.Branch = types.StringValue(version.Branch)
	data.Revision = types.StringValue(version.Revision)
	data.GoVersion = types.StringValue(version.GoVersion)
	data.BuildDate = types.StringValue(version.BuildDate)
	data.BuildUser = types.StringValue(version.BuildUser)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
