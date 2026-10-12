package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// configurationIntegrationsModel is the opt-in `integrations` block of
// fleetdm_configuration. Each attribute is only sent and refreshed while it is
// configured, so settings made in the Fleet UI or by GitOps are left alone.
type configurationIntegrationsModel struct {
	CertificatesIdPIntrospectionURLs      types.Set  `tfsdk:"certificates_idp_introspection_urls"`
	CertificatesIdPClientIDs              types.Set  `tfsdk:"certificates_idp_client_ids"`
	CertificatesDisableHostEndUserBinding types.Bool `tfsdk:"certificates_disable_host_end_user_binding"`
}

func configurationIntegrationsSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		MarkdownDescription: "Global integration settings. Only the attributes set here are managed; Jira, Zendesk and " +
			"Google Calendar integrations are not touched.\n\n" +
			"The `certificates_*` settings (Fleet 4.93+) control the request certificate API. Since 4.93 it is closed by " +
			"default: a request signed with a host identity certificate must name the end user Fleet recorded for that " +
			"host, and IdP credentials are refused unless their introspection endpoint is listed here.",
		Attributes: map[string]schema.Attribute{
			"certificates_idp_introspection_urls": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				MarkdownDescription: "OAuth 2.0 token introspection endpoints allowed to vouch for certificate requests. " +
					"Each must be an absolute `https` URL without credentials. Once any are listed, IdP credentials are " +
					"required on every request. An empty set clears the list; leaving the attribute out leaves Fleet's " +
					"value alone. _Available in Fleet Premium 4.93+._",
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(idpIntrospectionURLValidator{}),
				},
			},
			"certificates_idp_client_ids": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				MarkdownDescription: "OAuth client IDs allowed on certificate requests, as a further constraint on " +
					"`certificates_idp_introspection_urls`, which must not be empty while this is set. An empty set " +
					"clears the list. _Available in Fleet Premium 4.93+._",
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(trimmedNonEmptyValidator{}),
				},
			},
			"certificates_disable_host_end_user_binding": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "Turn off the check that a certificate request signed by a host names that host's " +
					"recorded end user (both the CSR email and UPN). The check is on by default and refuses hosts with no " +
					"recorded identity. **Security:** `true` lets any enrolled host request a certificate for any end user. " +
					"Removing the attribute stops managing it but leaves Fleet's value in place, so set it to `false` to turn the check back on. " +
					"_Available in Fleet 4.93+._",
			},
		},
		Validators: []validator.Object{integrationsClientIDsNeedURLs{}},
	}
}

// buildConfigurationIntegrationsUpdate returns the integrations object for PATCH /config,
// or nil when nothing in it is managed.
func buildConfigurationIntegrationsUpdate(ctx context.Context, m *configurationIntegrationsModel, diags *diag.Diagnostics) *fleetdm.IntegrationsUpdate {
	if m == nil {
		return nil
	}
	var out fleetdm.IntegrationsUpdate
	managed := false
	if !m.CertificatesIdPIntrospectionURLs.IsNull() && !m.CertificatesIdPIntrospectionURLs.IsUnknown() {
		urls := stringSetToSlice(ctx, m.CertificatesIdPIntrospectionURLs, diags)
		out.CertificatesIdPIntrospectionURLs = &urls
		managed = true
	}
	if !m.CertificatesIdPClientIDs.IsNull() && !m.CertificatesIdPClientIDs.IsUnknown() {
		ids := stringSetToSlice(ctx, m.CertificatesIdPClientIDs, diags)
		out.CertificatesIdPClientIDs = &ids
		managed = true
	}
	if v := optionalBoolPtr(m.CertificatesDisableHostEndUserBinding); v != nil {
		out.CertificatesDisableHostEndUserBinding = v
		managed = true
	}
	if !managed {
		return nil
	}
	return &out
}

// refreshConfigurationIntegrations copies Fleet's values into the attributes that are
// already managed. Fleet reports a list cleared with null as null, which is
// stored as an empty set so it matches a configured []. A server without the
// settings (before 4.93) leaves state alone; checkConfigurationIntegrationsApplied
// reports that on apply.
func refreshConfigurationIntegrations(m *configurationIntegrationsModel, api *fleetdm.AppConfigIntegrations, diags *diag.Diagnostics) {
	if m == nil || api == nil || !api.HasCertificateControls {
		return
	}
	if !m.CertificatesIdPIntrospectionURLs.IsNull() {
		m.CertificatesIdPIntrospectionURLs = stringsToSet(api.CertificatesIdPIntrospectionURLs, diags)
	}
	if !m.CertificatesIdPClientIDs.IsNull() {
		m.CertificatesIdPClientIDs = stringsToSet(api.CertificatesIdPClientIDs, diags)
	}
	if !m.CertificatesDisableHostEndUserBinding.IsNull() {
		m.CertificatesDisableHostEndUserBinding = types.BoolValue(api.CertificatesDisableHostEndUserBinding != nil && *api.CertificatesDisableHostEndUserBinding)
	}
}

// checkConfigurationIntegrationsApplied turns a server that silently dropped
// the managed settings into a named error rather than an inconsistent result.
func checkConfigurationIntegrationsApplied(m *configurationIntegrationsModel, api *fleetdm.AppConfigIntegrations, diags *diag.Diagnostics) {
	if m == nil || (api != nil && api.HasCertificateControls) {
		return
	}
	if m.CertificatesIdPIntrospectionURLs.IsNull() && m.CertificatesIdPClientIDs.IsNull() && m.CertificatesDisableHostEndUserBinding.IsNull() {
		return
	}
	diags.AddAttributeError(path.Root("integrations"),
		"Certificate request settings not supported",
		"Fleet accepted the configuration but did not report the certificates_* settings. They need Fleet 4.93.0 or later. Remove them from the integrations block or upgrade Fleet.")
}

func stringsToSet(values []string, diags *diag.Diagnostics) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	set, d := types.SetValue(types.StringType, elems)
	diags.Append(d...)
	return set
}

// idpIntrospectionURLValidator mirrors Fleet's isAbsoluteHTTPSURL check, plus
// the whitespace rule: Fleet trims entries before storing them, which would
// otherwise read back as a different value.
type idpIntrospectionURLValidator struct{}

func (idpIntrospectionURLValidator) Description(context.Context) string {
	return "must be an absolute https URL without credentials or surrounding whitespace"
}

func (v idpIntrospectionURLValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (idpIntrospectionURLValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	if strings.TrimSpace(raw) != raw {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid introspection URL",
			fmt.Sprintf("%q has leading or trailing whitespace, which Fleet trims and would show as a permanent diff.", raw))
		return
	}
	u, err := url.ParseRequestURI(raw)
	if err == nil && u.User != nil {
		// Not echoed, so the credentials stay out of plan output and CI logs.
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid introspection URL",
			"The URL must be an absolute https URL without credentials; this one contains credentials.")
		return
	}
	if err != nil || u.Scheme != "https" || u.Host == "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid introspection URL",
			fmt.Sprintf("%q must be an absolute https URL without credentials.", raw))
	}
}

// trimmedNonEmptyValidator rejects empty values and values with surrounding
// whitespace, which Fleet would trim or reject.
type trimmedNonEmptyValidator struct{}

func (trimmedNonEmptyValidator) Description(context.Context) string {
	return "must not be empty or have leading or trailing whitespace"
}

func (v trimmedNonEmptyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (trimmedNonEmptyValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	if raw == "" || strings.TrimSpace(raw) != raw {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value",
			fmt.Sprintf("%q must not be empty or have leading or trailing whitespace.", raw))
	}
}

// integrationsClientIDsNeedURLs mirrors Fleet's 422 for client IDs without
// introspection URLs. It only fires when the URLs are managed here and known to
// be empty; unmanaged URLs may be set in Fleet, so the API decides.
type integrationsClientIDsNeedURLs struct{}

func (integrationsClientIDsNeedURLs) Description(context.Context) string {
	return "certificates_idp_client_ids requires certificates_idp_introspection_urls"
}

func (v integrationsClientIDsNeedURLs) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (integrationsClientIDsNeedURLs) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attrs := req.ConfigValue.Attributes()
	ids, okIDs := attrs["certificates_idp_client_ids"].(types.Set)
	urls, okURLs := attrs["certificates_idp_introspection_urls"].(types.Set)
	if !okIDs || !okURLs || ids.IsNull() || ids.IsUnknown() || urls.IsNull() || urls.IsUnknown() {
		return
	}
	if len(ids.Elements()) > 0 && len(urls.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(req.Path.AtName("certificates_idp_client_ids"), "Missing introspection URLs",
			"certificates_idp_client_ids requires at least one certificates_idp_introspection_urls entry.")
	}
}
