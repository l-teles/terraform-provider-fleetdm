package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &EnrollSecretResource{}
	_ resource.ResourceWithConfigure   = &EnrollSecretResource{}
	_ resource.ResourceWithImportState = &EnrollSecretResource{}
)

// NewEnrollSecretResource creates a new resource for managing enrollment secrets.
func NewEnrollSecretResource() resource.Resource {
	return &EnrollSecretResource{}
}

// EnrollSecretResource defines the resource implementation.
type EnrollSecretResource struct {
	client *fleetdm.Client
}

// EnrollSecretResourceModel describes the resource data model.
type EnrollSecretResourceModel struct {
	ID      types.String             `tfsdk:"id"`
	TeamID  types.Int64              `tfsdk:"team_id"`
	Secrets []EnrollSecretEntryModel `tfsdk:"secrets"`
}

// EnrollSecretEntryModel describes an individual secret entry.
type EnrollSecretEntryModel struct {
	Secret    types.String `tfsdk:"secret"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// Metadata returns the resource type name.
func (r *EnrollSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_enroll_secret"
}

// Schema defines the schema for the resource.
func (r *EnrollSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages FleetDM enrollment secrets.

Enrollment secrets are used by hosts to authenticate when enrolling with Fleet. This resource manages 
either global enrollment secrets (when team_id is not specified) or team-specific enrollment secrets 
(when team_id is specified). Note: Team enrollment secrets require FleetDM Premium.

~> **Note:** This resource manages the complete set of enrollment secrets. When you apply this resource, 
it will replace all existing enrollment secrets for the specified scope (global or team) with the 
secrets defined in this resource.

## Example Usage

### Global Enrollment Secrets

` + "```hcl" + `
resource "fleetdm_enroll_secret" "global" {
  secrets = [
    { secret = "my-global-secret-1" },
    { secret = "my-global-secret-2" },
  ]
}
` + "```" + `

### Team Enrollment Secrets (Premium)

` + "```hcl" + `
resource "fleetdm_enroll_secret" "team" {
  team_id = 1
  secrets = [
    { secret = "my-team-secret-1" },
  ]
}
` + "```" + `
`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The identifier for this resource. For global secrets, this is 'global'. For team secrets, this is 'team-{team_id}'.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the team for team-specific enrollment secrets. If not specified, manages global enrollment secrets.",
				Optional:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"secrets": schema.ListNestedAttribute{
				MarkdownDescription: "The list of enrollment secrets. At least one secret is required.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"secret": schema.StringAttribute{
							MarkdownDescription: "The enrollment secret value.",
							Required:            true,
							Sensitive:           true,
						},
						"created_at": schema.StringAttribute{
							MarkdownDescription: "The timestamp when the secret was created.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *EnrollSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *EnrollSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EnrollSecretResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Build the secrets spec
	secrets := make([]fleetdm.EnrollSecret, len(data.Secrets))
	for i, s := range data.Secrets {
		secrets[i] = fleetdm.EnrollSecret{
			Secret: s.Secret.ValueString(),
		}
	}

	if data.TeamID.IsNull() {
		// Global secrets
		tflog.Debug(ctx, "Creating global enrollment secrets", map[string]interface{}{
			"secret_count": len(secrets),
		})

		spec := &fleetdm.EnrollSecretSpec{
			Secrets: secrets,
		}

		err := r.client.ApplyEnrollSecretSpec(ctx, spec)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Creating Global Enrollment Secrets",
				"Could not create global enrollment secrets: "+err.Error(),
			)
			return
		}

		data.ID = types.StringValue("global")
	} else {
		// Team secrets
		teamID := data.TeamID.ValueInt64()
		tflog.Debug(ctx, "Creating team enrollment secrets", map[string]interface{}{
			"team_id":      teamID,
			"secret_count": len(secrets),
		})

		_, err := r.client.ModifyTeamEnrollSecrets(ctx, teamID, secrets)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Creating Team Enrollment Secrets",
				fmt.Sprintf("Could not create enrollment secrets for team %d: %s", teamID, err.Error()),
			)
			return
		}

		data.ID = types.StringValue(fmt.Sprintf("team-%d", teamID))
	}

	// Read back the created secrets to get created_at timestamps
	r.readSecrets(ctx, &data, newEnrollDiagAdapter(resp.Diagnostics.AddError))

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Created enrollment secrets", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *EnrollSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EnrollSecretResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r.readSecrets(ctx, &data, newEnrollDiagAdapter(resp.Diagnostics.AddError))

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// diagWriter is a minimal interface covering the Diagnostics field on
// Create/Read/Update response types, allowing readSecrets to be generic.
type diagWriter interface {
	addError(summary, detail string)
}

type enrollDiagAdapter struct {
	add func(string, string)
}

func (a enrollDiagAdapter) addError(s, d string) { a.add(s, d) }

func newEnrollDiagAdapter(add func(string, string)) diagWriter {
	return enrollDiagAdapter{add: add}
}

// readSecrets is a helper function to read secrets from the API.
func (r *EnrollSecretResource) readSecrets(ctx context.Context, data *EnrollSecretResourceModel, diag diagWriter) {
	if data.TeamID.IsNull() {
		// Global secrets
		tflog.Debug(ctx, "Reading global enrollment secrets")

		spec, err := r.client.GetEnrollSecretSpec(ctx)
		if err != nil {
			diag.addError(
				"Error Reading Global Enrollment Secrets",
				"Could not read global enrollment secrets: "+err.Error(),
			)
			return
		}

		// Preserve the order from the plan/state, matching by secret value
		// This ensures terraform doesn't show spurious diffs
		data.Secrets = r.matchSecrets(data.Secrets, spec.Secrets)
		data.ID = types.StringValue("global")
	} else {
		// Team secrets
		teamID := data.TeamID.ValueInt64()
		tflog.Debug(ctx, "Reading team enrollment secrets", map[string]interface{}{
			"team_id": teamID,
		})

		secrets, err := r.client.GetTeamEnrollSecrets(ctx, teamID)
		if err != nil {
			diag.addError(
				"Error Reading Team Enrollment Secrets",
				fmt.Sprintf("Could not read enrollment secrets for team %d: %s", teamID, err.Error()),
			)
			return
		}

		// Preserve the order from the plan/state, matching by secret value
		data.Secrets = r.matchSecrets(data.Secrets, secrets)
		data.ID = types.StringValue(fmt.Sprintf("team-%d", teamID))
	}
}

// matchSecrets preserves the order from the plan/state while updating created_at from API
func (r *EnrollSecretResource) matchSecrets(planned []EnrollSecretEntryModel, apiSecrets []fleetdm.EnrollSecret) []EnrollSecretEntryModel {
	// Create a map of API secrets for lookup
	apiSecretMap := make(map[string]fleetdm.EnrollSecret)
	for _, s := range apiSecrets {
		apiSecretMap[s.Secret] = s
	}

	// Match planned secrets with API results
	result := make([]EnrollSecretEntryModel, len(planned))
	for i, p := range planned {
		secretValue := p.Secret.ValueString()
		if apiSecret, found := apiSecretMap[secretValue]; found {
			result[i] = EnrollSecretEntryModel{
				Secret:    types.StringValue(apiSecret.Secret),
				CreatedAt: types.StringValue(apiSecret.CreatedAt),
			}
		} else {
			// Keep the planned value if not found in API (shouldn't happen normally)
			result[i] = p
		}
	}

	return result
}

// Update updates the resource and sets the updated Terraform state.
func (r *EnrollSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EnrollSecretResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Build the secrets spec
	secrets := make([]fleetdm.EnrollSecret, len(data.Secrets))
	for i, s := range data.Secrets {
		secrets[i] = fleetdm.EnrollSecret{
			Secret: s.Secret.ValueString(),
		}
	}

	if data.TeamID.IsNull() {
		// Global secrets
		tflog.Debug(ctx, "Updating global enrollment secrets", map[string]interface{}{
			"secret_count": len(secrets),
		})

		spec := &fleetdm.EnrollSecretSpec{
			Secrets: secrets,
		}

		err := r.client.ApplyEnrollSecretSpec(ctx, spec)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating Global Enrollment Secrets",
				"Could not update global enrollment secrets: "+err.Error(),
			)
			return
		}
	} else {
		// Team secrets
		teamID := data.TeamID.ValueInt64()
		tflog.Debug(ctx, "Updating team enrollment secrets", map[string]interface{}{
			"team_id":      teamID,
			"secret_count": len(secrets),
		})

		_, err := r.client.ModifyTeamEnrollSecrets(ctx, teamID, secrets)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating Team Enrollment Secrets",
				fmt.Sprintf("Could not update enrollment secrets for team %d: %s", teamID, err.Error()),
			)
			return
		}
	}

	// Read back the updated secrets
	r.readSecrets(ctx, &data, newEnrollDiagAdapter(resp.Diagnostics.AddError))

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updated enrollment secrets", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource and clears the Terraform state.
// For team secrets this sets the list to empty. For global secrets it also
// attempts to clear them; Fleet may reject an empty list if it requires at
// least one secret, in which case a warning is logged and state is still removed.
func (r *EnrollSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EnrollSecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	emptySecrets := []fleetdm.EnrollSecret{}

	if data.TeamID.IsNull() {
		tflog.Debug(ctx, "Clearing global enrollment secrets")
		spec := &fleetdm.EnrollSecretSpec{Secrets: emptySecrets}
		if err := r.client.ApplyEnrollSecretSpec(ctx, spec); err != nil {
			// Fleet may reject an empty secrets list. Log a warning but still
			// remove the resource from Terraform state so it is no longer managed.
			tflog.Warn(ctx, "Could not clear global enrollment secrets (Fleet may require at least one); removing from Terraform state only",
				map[string]interface{}{"error": err.Error()})
		}
	} else {
		teamID := data.TeamID.ValueInt64()
		tflog.Debug(ctx, "Clearing team enrollment secrets", map[string]interface{}{"team_id": teamID})
		if _, err := r.client.ModifyTeamEnrollSecrets(ctx, teamID, emptySecrets); err != nil {
			// Ignore 404 – the team itself may already be deleted.
			if isNotFound(err) {
				return
			}
			resp.Diagnostics.AddError(
				"Error Deleting Team Enrollment Secrets",
				fmt.Sprintf("Could not clear enrollment secrets for team %d: %s", teamID, err.Error()),
			)
			return
		}
	}
}

// ImportState imports an existing resource into Terraform state.
func (r *EnrollSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID

	tflog.Debug(ctx, "Importing enrollment secrets", map[string]interface{}{
		"id": id,
	})

	var data EnrollSecretResourceModel

	if id == "global" {
		data.ID = types.StringValue("global")
		data.TeamID = types.Int64Null()
	} else if len(id) > 5 && id[:5] == "team-" {
		// Parse team ID from "team-{id}"
		var teamID int64
		_, err := fmt.Sscanf(id, "team-%d", &teamID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Could not parse team ID from import ID '%s'. Expected 'global' or 'team-{team_id}'.", id),
			)
			return
		}
		data.ID = types.StringValue(id)
		data.TeamID = types.Int64Value(teamID)
	} else {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Invalid import ID '%s'. Expected 'global' or 'team-{team_id}'.", id),
		)
		return
	}

	// Read the secrets from API
	if data.TeamID.IsNull() {
		spec, err := r.client.GetEnrollSecretSpec(ctx)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Importing Global Enrollment Secrets",
				"Could not read global enrollment secrets: "+err.Error(),
			)
			return
		}

		data.Secrets = make([]EnrollSecretEntryModel, len(spec.Secrets))
		for i, s := range spec.Secrets {
			data.Secrets[i] = EnrollSecretEntryModel{
				Secret:    types.StringValue(s.Secret),
				CreatedAt: types.StringValue(s.CreatedAt),
			}
		}
	} else {
		teamID := data.TeamID.ValueInt64()
		secrets, err := r.client.GetTeamEnrollSecrets(ctx, teamID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Importing Team Enrollment Secrets",
				fmt.Sprintf("Could not read enrollment secrets for team %d: %s", teamID, err.Error()),
			)
			return
		}

		data.Secrets = make([]EnrollSecretEntryModel, len(secrets))
		for i, s := range secrets {
			data.Secrets[i] = EnrollSecretEntryModel{
				Secret:    types.StringValue(s.Secret),
				CreatedAt: types.StringValue(s.CreatedAt),
			}
		}
	}

	tflog.Info(ctx, "Imported enrollment secrets", map[string]interface{}{
		"id":           data.ID.ValueString(),
		"secret_count": len(data.Secrets),
	})

	// Save imported data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
