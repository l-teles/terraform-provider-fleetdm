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
var _ datasource.DataSource = &EnrollSecretsDataSource{}

func NewEnrollSecretsDataSource() datasource.DataSource {
	return &EnrollSecretsDataSource{}
}

// EnrollSecretsDataSource defines the data source implementation.
type EnrollSecretsDataSource struct {
	client *fleetdm.Client
}

// EnrollSecretModel describes a single enroll secret
type EnrollSecretModel struct {
	Secret    types.String `tfsdk:"secret"`
	CreatedAt types.String `tfsdk:"created_at"`
	TeamID    types.Int64  `tfsdk:"team_id"`
}

// EnrollSecretsDataSourceModel describes the data source data model.
type EnrollSecretsDataSourceModel struct {
	TeamID  types.Int64         `tfsdk:"team_id"`
	Secrets []EnrollSecretModel `tfsdk:"secrets"`
}

func (d *EnrollSecretsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_enroll_secrets"
}

func (d *EnrollSecretsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches enrollment secrets. If team_id is not specified, returns global enrollment secrets.",

		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the team to get enrollment secrets for. If not specified, returns global secrets.",
				Optional:            true,
			},
			"secrets": schema.ListNestedAttribute{
				MarkdownDescription: "The list of enrollment secrets.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"secret": schema.StringAttribute{
							MarkdownDescription: "The enrollment secret string.",
							Computed:            true,
							Sensitive:           true,
						},
						"created_at": schema.StringAttribute{
							MarkdownDescription: "The timestamp when the secret was created.",
							Computed:            true,
						},
						"team_id": schema.Int64Attribute{
							MarkdownDescription: "The team ID this secret belongs to. Null for global secrets.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *EnrollSecretsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *EnrollSecretsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data EnrollSecretsDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var secrets []fleetdm.EnrollSecret

	if data.TeamID.IsNull() || data.TeamID.IsUnknown() {
		// Get global enrollment secrets
		spec, err := d.client.GetEnrollSecretSpec(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read global enrollment secrets, got error: %s", err))
			return
		}
		secrets = spec.Secrets
	} else {
		// Get team enrollment secrets
		teamID := data.TeamID.ValueInt64()
		teamSecrets, err := d.client.GetTeamEnrollSecrets(ctx, teamID)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read team enrollment secrets, got error: %s", err))
			return
		}
		secrets = teamSecrets
	}

	// Map response to model
	data.Secrets = make([]EnrollSecretModel, len(secrets))
	for i, secret := range secrets {
		data.Secrets[i] = EnrollSecretModel{
			Secret:    types.StringValue(secret.Secret),
			CreatedAt: types.StringValue(secret.CreatedAt),
		}
		if secret.TeamID != nil {
			data.Secrets[i].TeamID = types.Int64Value(*secret.TeamID)
		} else {
			data.Secrets[i].TeamID = types.Int64Null()
		}
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
