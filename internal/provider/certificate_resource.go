package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &CertificateResource{}
	_ resource.ResourceWithImportState = &CertificateResource{}
)

// NewCertificateResource creates a new certificate template resource.
func NewCertificateResource() resource.Resource {
	return &CertificateResource{}
}

// CertificateResource manages a FleetDM certificate template.
//
// Fleet 4.90 has no update endpoint for certificate templates, so every
// configurable attribute forces replacement and Update is unreachable.
type CertificateResource struct {
	client *fleetdm.Client
}

// CertificateResourceModel describes the resource data model.
type CertificateResourceModel struct {
	ID                       types.Int64  `tfsdk:"id"`
	FleetID                  types.Int64  `tfsdk:"fleet_id"`
	Name                     types.String `tfsdk:"name"`
	CertificateAuthorityID   types.Int64  `tfsdk:"certificate_authority_id"`
	SubjectName              types.String `tfsdk:"subject_name"`
	SubjectAlternativeName   types.String `tfsdk:"subject_alternative_name"`
	CertificateAuthorityName types.String `tfsdk:"certificate_authority_name"`
	CertificateAuthorityType types.String `tfsdk:"certificate_authority_type"`
	CreatedAt                types.String `tfsdk:"created_at"`
}

func (r *CertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *CertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FleetDM certificate template. A certificate template binds a certificate authority to a " +
			"subject name so Fleet issues a client certificate to every enrolled Android host in the fleet.\n\n" +
			"Requires Fleet Premium and Fleet >= 4.90.\n\n" +
			"~> Only a `custom_scep_proxy` certificate authority is supported. Pointing this resource at any other " +
			"`fleetdm_certificate_authority` type fails with `Currently, only the custom_scep_proxy certificate authority is supported.`\n\n" +
			"~> Fleet has no update endpoint for certificate templates, so **every** attribute forces replacement. " +
			"Replacement re-issues the certificate to every host in the fleet.\n\n" +
			"The subject name and subject alternative name may reference these Fleet variables, which Fleet expands " +
			"per host at delivery time: `$FLEET_VAR_HOST_UUID`, `$FLEET_VAR_HOST_HARDWARE_SERIAL`, " +
			"`$FLEET_VAR_HOST_PLATFORM`, `$FLEET_VAR_HOST_END_USER_IDP_USERNAME`, " +
			"`$FLEET_VAR_HOST_END_USER_IDP_USERNAME_LOCAL_PART`, `$FLEET_VAR_HOST_END_USER_IDP_GROUPS`, " +
			"`$FLEET_VAR_HOST_END_USER_IDP_DEPARTMENT`, `$FLEET_VAR_HOST_END_USER_IDP_FULLNAME`. Any other " +
			"variable is rejected by Fleet at apply time.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the certificate template.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"fleet_id": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "The ID of the fleet (team) the certificate template belongs to. Defaults to `0`, which targets hosts that are not assigned to a fleet. Changing this forces a new certificate template to be created.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: fmt.Sprintf("The certificate template name. Must be unique within the fleet, at most %d characters, and may contain "+
					"only letters, numbers, spaces, dashes and underscores — dots are **not** allowed. "+
					"Changing this forces a new certificate template to be created.",
					fleetdm.CertificateTemplateNameMaxLength),
				Validators: []validator.String{
					stringvalidator.LengthAtMost(fleetdm.CertificateTemplateNameMaxLength),
					stringvalidator.RegexMatches(
						fleetdm.CertificateTemplateNamePattern,
						"must contain only letters, numbers, spaces, dashes and underscores",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"certificate_authority_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The ID of the `custom_scep_proxy` certificate authority that issues certificates for this template. Changing this forces a new certificate template to be created.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"subject_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The certificate subject, as an X.500 distinguished name (for example `CN=$FLEET_VAR_HOST_HARDWARE_SERIAL,O=Example`). Changing this forces a new certificate template to be created.",
				Validators: []validator.String{
					nonBlankStringValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subject_alternative_name": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: fmt.Sprintf("The certificate subject alternative names, as a comma-separated list of `KEY=value` entries "+
					"(for example `DNS=host.example.com,UPN=$FLEET_VAR_HOST_END_USER_IDP_USERNAME`). Allowed keys are `%s`. "+
					"At most %d bytes. Changing this forces a new certificate template to be created.",
					strings.Join(fleetdm.CertificateTemplateSubjectAlternativeNameKeys, "`, `"),
					fleetdm.CertificateTemplateSubjectAlternativeNameMaxBytes),
				Validators: []validator.String{
					stringvalidator.LengthAtMost(fleetdm.CertificateTemplateSubjectAlternativeNameMaxBytes),
					nonBlankStringValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"certificate_authority_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the certificate authority that issues certificates for this template.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"certificate_authority_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the certificate authority that issues certificates for this template. Always `custom_scep_proxy` on Fleet 4.90.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp when the certificate template was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// nonBlankStringValidator rejects a value that is empty or consists only of
// whitespace.
//
// Fleet rejects a blank subject_name outright ("Certificate template subject
// name is required."). It accepts a blank subject_alternative_name but stores it
// as absent, so the value read back differs from the one configured and
// Terraform aborts the apply with "Provider produced inconsistent result after
// apply" (verified live on 4.90: a template created with
// `subject_alternative_name` set to three spaces reads back with the field
// absent).
//
// The predicate is strings.TrimSpace rather than a regex on purpose. Go's RE2
// defines \s as the ASCII set [\t\n\f\r ], while strings.TrimSpace uses
// unicode.IsSpace — so a `\S` pattern would accept U+00A0, U+2028, U+3000 and
// friends that Fleet still treats as blank. Reusing Fleet's own predicate keeps
// the two in lockstep by construction.
//
// Note this only rejects *entirely* blank values. Fleet does not trim, so
// padding around a real value is preserved exactly and is left alone here
// (verified live: a name, subject and SAN all created with surrounding spaces
// read back byte-identical).
type nonBlankStringValidator struct{}

var _ validator.String = nonBlankStringValidator{}

func (nonBlankStringValidator) Description(_ context.Context) string {
	return "must not be empty or consist only of whitespace"
}

func (v nonBlankStringValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (nonBlankStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if strings.TrimSpace(value) != "" {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Attribute Value",
		fmt.Sprintf("Attribute %s must not be empty or consist only of whitespace: Fleet either rejects a blank value "+
			"or stores it as absent, which would leave the applied value different from the planned one. Got: %q",
			req.Path, value),
	)
}

func (r *CertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

func (r *CertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CertificateResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateCertificateTemplate(ctx, fleetdm.CreateCertificateTemplateRequest{
		Name:                   data.Name.ValueString(),
		FleetID:                data.FleetID.ValueInt64(),
		CertificateAuthorityID: data.CertificateAuthorityID.ValueInt64(),
		SubjectName:            data.SubjectName.ValueString(),
		SubjectAlternativeName: data.SubjectAlternativeName.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating FleetDM Certificate Template",
			fmt.Sprintf("Unable to create certificate template: %s", err),
		)
		return
	}

	// Record what Fleet echoed first, so state is written even if the follow-up
	// read fails below.
	r.mapTemplateToModel(created, &data)

	// The create response omits certificate_authority_name,
	// certificate_authority_type and created_at, so read the template back to
	// fill them in.
	//
	// A failed read-back deliberately raises a warning rather than an error. The
	// template already exists in Fleet at this point, and returning an error
	// would abort Create before state was written — leaving the template
	// orphaned, with the next apply failing on the duplicate name. Degraded
	// information is not a failed apply: the three attributes stay null and the
	// next refresh fills them in.
	template, err := r.client.GetCertificateTemplate(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Certificate Template Created But Not Read Back",
			fmt.Sprintf("Fleet created certificate template %d, but reading it back failed. The template is saved in "+
				"Terraform state so it stays managed; certificate_authority_name, certificate_authority_type and "+
				"created_at are unset until the next refresh.\n\nFleet reported: %s", created.ID, err),
		)
	} else {
		r.mapTemplateToModel(template, &data)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CertificateResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	template, err := r.client.GetCertificateTemplate(ctx, data.ID.ValueInt64())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Certificate Template",
			fmt.Sprintf("Unable to read certificate template %d: %s", data.ID.ValueInt64(), err),
		)
		return
	}

	r.mapTemplateToModel(template, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is unreachable: Fleet 4.90 has no PATCH or PUT route for certificate
// templates, so every configurable attribute carries RequiresReplace and any
// change is planned as a replacement rather than an update. It reports an error
// instead of silently doing nothing, so a future schema change that forgets
// RequiresReplace surfaces here rather than drifting.
func (r *CertificateResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Certificate Templates Cannot Be Updated In Place",
		"Fleet has no update endpoint for certificate templates, so every attribute of fleetdm_certificate forces "+
			"replacement and this code path should be unreachable. Please report this issue to the provider developers.",
	)
}

func (r *CertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CertificateResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteCertificateTemplate(ctx, data.ID.ValueInt64()); err != nil {
		// Fleet 4.90 answers 500 "forbidden" rather than 404 for a template
		// that is already gone, so this branch does not fire there. It is kept
		// deliberately: Read removes a missing template from state before Delete
		// is reached in normal use, and a Fleet release that fixes the status
		// code should make an already-deleted template a clean no-op rather than
		// a hard failure. The 500 is not swallowed — a real permission failure
		// is indistinguishable from it, and reporting a destroy that never
		// happened would leave the template behind with no state to find it.
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Certificate Template",
			fmt.Sprintf("Unable to delete certificate template %d: %s\n\nIf the template was already deleted in Fleet, "+
				"run `terraform refresh` to drop it from state, or remove it with `terraform state rm`.",
				data.ID.ValueInt64(), err),
		)
		return
	}
}

// ImportState imports a certificate template using the composite ID
// "fleet_id:id". Both parts are required: Fleet never returns the template's
// fleet in any response, so the template ID alone cannot recover it and a plain
// import would leave fleet_id wrong for every template that is not in fleet 0.
//
// Because nothing in the subsequent Read can contradict the fleet the caller
// supplied, this checks it here against the fleet-scoped listing. A silently
// wrong fleet_id would be worse than a failed import: fleet_id carries
// RequiresReplace, so the next plan would destroy and recreate the template —
// re-issuing certificates to every host — or, if the configuration happened to
// match the wrong value, leave state quietly lying about where the template
// lives.
func (r *CertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in format: fleet_id:id",
		)
		return
	}

	fleetID, ok := parseIDFromString(parts[0], "Certificate Template Fleet", &resp.Diagnostics)
	if !ok {
		return
	}
	id, ok := parseIDFromString(parts[1], "Certificate Template", &resp.Diagnostics)
	if !ok {
		return
	}

	templates, err := r.client.ListCertificateTemplates(ctx, fleetID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing FleetDM Certificate Template",
			fmt.Sprintf("Unable to list the certificate templates on fleet %d to confirm template %d belongs to it: %s", fleetID, id, err),
		)
		return
	}
	found := false
	for _, template := range templates {
		if template.ID == id {
			found = true
			break
		}
	}
	if !found {
		resp.Diagnostics.AddError(
			"Error Importing FleetDM Certificate Template",
			fmt.Sprintf("Certificate template %d is not on fleet %d. The import ID's fleet must be the one the template "+
				"actually belongs to — Fleet does not report a template's fleet, so it cannot be inferred. Use `0` for "+
				"templates targeting hosts that are not assigned to a fleet.", id, fleetID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fleet_id"), fleetID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// mapTemplateToModel copies an API certificate template onto the resource model.
//
// fleet_id is deliberately not touched: no Fleet response carries it, so the
// configured (or imported) value is the only source of truth.
func (r *CertificateResource) mapTemplateToModel(template *fleetdm.CertificateTemplate, data *CertificateResourceModel) {
	data.ID = types.Int64Value(template.ID)
	data.Name = types.StringValue(template.Name)
	data.CertificateAuthorityID = types.Int64Value(template.CertificateAuthorityID)
	data.SubjectName = types.StringValue(template.SubjectName)

	// subject_alternative_name is Optional and not Computed, so an unset value
	// has to stay null rather than becoming "": Fleet omits the field when the
	// template has no SAN, and writing an empty string into state would make it
	// disagree with a configuration that never set it.
	if template.SubjectAlternativeName == "" {
		data.SubjectAlternativeName = types.StringNull()
	} else {
		data.SubjectAlternativeName = types.StringValue(template.SubjectAlternativeName)
	}

	// These three come from the read routes only. The create response omits
	// them, so an empty value means "not reported" rather than "empty" and has
	// to stay null — a Computed attribute may be null, but it may not be left
	// unknown once Create returns.
	data.CertificateAuthorityName = emptyStringToNull(template.CertificateAuthorityName)
	data.CertificateAuthorityType = emptyStringToNull(template.CertificateAuthorityType)
	data.CreatedAt = emptyStringToNull(template.CreatedAt)
}

// emptyStringToNull maps an absent API string to a null value rather than an
// empty string, so state does not distinguish "" from "not reported".
func emptyStringToNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}
