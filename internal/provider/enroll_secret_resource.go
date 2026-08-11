package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
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
	_ resource.Resource                   = &EnrollSecretResource{}
	_ resource.ResourceWithConfigure      = &EnrollSecretResource{}
	_ resource.ResourceWithImportState    = &EnrollSecretResource{}
	_ resource.ResourceWithValidateConfig = &EnrollSecretResource{}
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

~> **Note:** Fleet 4.90+ masks enrollment secret values in API responses when the caller's role lacks
permission to read secrets. Use an API token with secret-read permission (e.g. admin or maintainer);
otherwise drift in secret values cannot be detected and the configured values are kept in state.

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

// ValidateConfig rejects empty or whitespace-only secret values at plan time.
// Fleet 4.90+ rejects these server-side; failing earlier gives a clearer error
// and covers older Fleet versions that would accept an unusable secret.
func (r *EnrollSecretResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var secretsList types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("secrets"), &secretsList)...)
	if resp.Diagnostics.HasError() || secretsList.IsNull() || secretsList.IsUnknown() {
		return
	}

	for i, el := range secretsList.Elements() {
		obj, ok := el.(types.Object)
		if !ok {
			continue
		}
		secretAttr, ok := obj.Attributes()["secret"].(types.String)
		if !ok || secretAttr.IsNull() || secretAttr.IsUnknown() {
			continue
		}
		switch {
		case strings.TrimSpace(secretAttr.ValueString()) == "":
			resp.Diagnostics.AddAttributeError(
				path.Root("secrets").AtListIndex(i).AtName("secret"),
				"Invalid Enrollment Secret",
				"Enrollment secrets must not be empty or whitespace-only.",
			)
		case isMaskedSecret(secretAttr.ValueString()):
			resp.Diagnostics.AddAttributeError(
				path.Root("secrets").AtListIndex(i).AtName("secret"),
				"Invalid Enrollment Secret",
				"Enrollment secrets must not consist solely of '*' characters: Fleet uses all-asterisk strings as masked-value placeholders, so such a value would be indistinguishable from a redacted secret.",
			)
		}
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *EnrollSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EnrollSecretResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	validateSecretValues(&data, &resp.Diagnostics)
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
	r.readSecrets(ctx, &data, newEnrollDiagAdapter(&resp.Diagnostics))

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

	r.readSecrets(ctx, &data, newEnrollDiagAdapter(&resp.Diagnostics))

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
	addWarning(summary, detail string)
}

type enrollDiagAdapter struct {
	diags *diag.Diagnostics
}

func (a enrollDiagAdapter) addError(s, d string)   { a.diags.AddError(s, d) }
func (a enrollDiagAdapter) addWarning(s, d string) { a.diags.AddWarning(s, d) }

func newEnrollDiagAdapter(diags *diag.Diagnostics) diagWriter {
	return enrollDiagAdapter{diags: diags}
}

// #nosec G101 -- user-facing warning message, not a credential
const secretReadDeniedWarning = "Fleet denied reading enrollment secrets (the API token's role lacks secret-read permission, enforced by Fleet 4.90+). The configured values were kept in state; drift in secret values cannot be detected."

// #nosec G101 -- user-facing warning message, not a credential
const secretsMaskedWarning = "Fleet returned masked enrollment secret values (the API token's role lacks secret-read permission, enforced by Fleet 4.90+). Masked entries were ignored; drift in secret values cannot be detected."

// validateSecretValues re-checks secret values at apply time. ValidateConfig
// must skip values that are unknown during planning (e.g. produced by another
// resource), so this is the last line of defense before the API call.
func validateSecretValues(data *EnrollSecretResourceModel, diags *diag.Diagnostics) {
	for i, s := range data.Secrets {
		v := s.Secret.ValueString()
		switch {
		case strings.TrimSpace(v) == "":
			diags.AddAttributeError(
				path.Root("secrets").AtListIndex(i).AtName("secret"),
				"Invalid Enrollment Secret",
				"Enrollment secrets must not be empty or whitespace-only.",
			)
		case isMaskedSecret(v):
			diags.AddAttributeError(
				path.Root("secrets").AtListIndex(i).AtName("secret"),
				"Invalid Enrollment Secret",
				"Enrollment secrets must not consist solely of '*' characters: Fleet uses all-asterisk strings as masked-value placeholders, so such a value would be indistinguishable from a redacted secret.",
			)
		}
	}
}

// readSecrets is a helper function to read secrets from the API.
func (r *EnrollSecretResource) readSecrets(ctx context.Context, data *EnrollSecretResourceModel, diag diagWriter) {
	if data.TeamID.IsNull() {
		// Global secrets
		tflog.Debug(ctx, "Reading global enrollment secrets")

		spec, err := r.client.GetEnrollSecretSpec(ctx)
		if err != nil {
			if isForbidden(err) {
				// Fleet 4.90+ can deny secret reads to write-capable tokens
				// without secret-read permission. Keep the configured values
				// instead of failing; drift detection is degraded.
				diag.addWarning("Enrollment Secrets Not Readable", secretReadDeniedWarning)
				data.Secrets = normalizeSecretTimestamps(data.Secrets)
				data.ID = types.StringValue("global")
				return
			}
			diag.addError(
				"Error Reading Global Enrollment Secrets",
				"Could not read global enrollment secrets: "+err.Error(),
			)
			return
		}
		if hasMaskedSecrets(spec.Secrets) {
			diag.addWarning("Enrollment Secrets Masked", secretsMaskedWarning)
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
			if isForbidden(err) {
				diag.addWarning("Enrollment Secrets Not Readable", secretReadDeniedWarning)
				data.Secrets = normalizeSecretTimestamps(data.Secrets)
				data.ID = types.StringValue(fmt.Sprintf("team-%d", teamID))
				return
			}
			diag.addError(
				"Error Reading Team Enrollment Secrets",
				fmt.Sprintf("Could not read enrollment secrets for team %d: %s", teamID, err.Error()),
			)
			return
		}
		if hasMaskedSecrets(secrets) {
			diag.addWarning("Enrollment Secrets Masked", secretsMaskedWarning)
		}

		// Preserve the order from the plan/state, matching by secret value
		data.Secrets = r.matchSecrets(data.Secrets, secrets)
		data.ID = types.StringValue(fmt.Sprintf("team-%d", teamID))
	}
}

// normalizeSecretTimestamps replaces unknown created_at values with null.
// During Create/Update the computed created_at enters as unknown; when a
// degraded read skips the API refresh, persisting an unknown value would make
// Terraform fail with "provider returned invalid result object after apply".
func normalizeSecretTimestamps(secrets []EnrollSecretEntryModel) []EnrollSecretEntryModel {
	for i := range secrets {
		if secrets[i].CreatedAt.IsUnknown() {
			secrets[i].CreatedAt = types.StringNull()
		}
	}
	return secrets
}

// hasMaskedSecrets reports whether any API-returned secret value is a
// redaction placeholder.
func hasMaskedSecrets(secrets []fleetdm.EnrollSecret) bool {
	for _, s := range secrets {
		if isMaskedSecret(s.Secret) {
			return true
		}
	}
	return false
}

// isMaskedSecret reports whether a secret value returned by the API is a
// redaction placeholder rather than a real value. Fleet 4.90+ masks enroll
// secrets (e.g. "********") for callers without secret-read permission.
func isMaskedSecret(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if r != '*' {
			return false
		}
	}
	return true
}

// matchSecrets preserves the order from the plan/state while updating created_at from API
func (r *EnrollSecretResource) matchSecrets(planned []EnrollSecretEntryModel, apiSecrets []fleetdm.EnrollSecret) []EnrollSecretEntryModel {
	// Create a map of API secrets for lookup, ignoring masked placeholders so
	// they can never be mistaken for real values.
	apiSecretMap := make(map[string]fleetdm.EnrollSecret)
	for _, s := range apiSecrets {
		if isMaskedSecret(s.Secret) {
			continue
		}
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
			// Keep the planned value if not found in API (e.g. the API entry
			// was masked). During Create/Update the computed created_at is
			// unknown and must be normalized to null before persisting state.
			if p.CreatedAt.IsUnknown() {
				p.CreatedAt = types.StringNull()
			}
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

	validateSecretValues(&data, &resp.Diagnostics)
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
	r.readSecrets(ctx, &data, newEnrollDiagAdapter(&resp.Diagnostics))

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
	var apiSecrets []fleetdm.EnrollSecret
	if data.TeamID.IsNull() {
		spec, err := r.client.GetEnrollSecretSpec(ctx)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Importing Global Enrollment Secrets",
				"Could not read global enrollment secrets: "+err.Error(),
			)
			return
		}
		apiSecrets = spec.Secrets
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
		apiSecrets = secrets
	}

	// Refuse to import masked placeholders: storing "********" in state (or
	// generated config) and later applying it would replace the real
	// enrollment secrets with a publicly-known value.
	if hasMaskedSecrets(apiSecrets) {
		resp.Diagnostics.AddError(
			"Enrollment Secrets Masked",
			"Fleet returned masked enrollment secret values, so real values cannot be imported. "+
				"The API token's role lacks secret-read permission (enforced by Fleet 4.90+); "+
				"import with a token whose role can read enrollment secrets (e.g. admin or maintainer).",
		)
		return
	}

	data.Secrets = make([]EnrollSecretEntryModel, len(apiSecrets))
	for i, s := range apiSecrets {
		data.Secrets[i] = EnrollSecretEntryModel{
			Secret:    types.StringValue(s.Secret),
			CreatedAt: types.StringValue(s.CreatedAt),
		}
	}

	tflog.Info(ctx, "Imported enrollment secrets", map[string]interface{}{
		"id":           data.ID.ValueString(),
		"secret_count": len(data.Secrets),
	})

	// Save imported data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
