package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
	"golang.org/x/text/unicode/norm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                     = &CertificateAuthorityResource{}
	_ resource.ResourceWithConfigure        = &CertificateAuthorityResource{}
	_ resource.ResourceWithImportState      = &CertificateAuthorityResource{}
	_ resource.ResourceWithModifyPlan       = &CertificateAuthorityResource{}
	_ resource.ResourceWithConfigValidators = &CertificateAuthorityResource{}
)

// caTypeBlocks lists the schema attribute name of every certificate authority
// type block. The names double as Fleet's CA type identifiers.
var caTypeBlocks = []string{
	fleetdm.CATypeDigiCert,
	fleetdm.CATypeNDESSCEPProxy,
	fleetdm.CATypeCustomSCEPProxy,
	fleetdm.CATypeCustomESTProxy,
	fleetdm.CATypeHydrant,
	fleetdm.CATypeSmallstep,
}

// caSecretPair names the credential attribute a CA type block carries. Every
// one has a write-only sibling suffixed `_wo`. Held as a slice rather than a
// map so validator and diagnostic ordering stays deterministic.
type caSecretPair struct {
	block string
	attr  string
}

var caSecretPairs = []caSecretPair{
	{fleetdm.CATypeDigiCert, "api_token"},
	{fleetdm.CATypeNDESSCEPProxy, "password"},
	{fleetdm.CATypeCustomSCEPProxy, "challenge"},
	{fleetdm.CATypeCustomESTProxy, "password"},
	{fleetdm.CATypeHydrant, "client_secret"},
	{fleetdm.CATypeSmallstep, "password"},
}

// caTypeBlockPaths returns a path expression for every CA type block, for use
// with the ExactlyOneOf validator.
func caTypeBlockPaths() []path.Expression {
	exprs := make([]path.Expression, 0, len(caTypeBlocks))
	for _, name := range caTypeBlocks {
		exprs = append(exprs, path.MatchRoot(name))
	}
	return exprs
}

// NewCertificateAuthorityResource creates a new resource for managing
// certificate authorities.
func NewCertificateAuthorityResource() resource.Resource {
	return &CertificateAuthorityResource{}
}

// CertificateAuthorityResource defines the resource implementation.
type CertificateAuthorityResource struct {
	client *fleetdm.Client
}

// CertificateAuthorityResourceModel describes the resource data model.
type CertificateAuthorityResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Type             types.String `tfsdk:"type"`
	SecretsWOVersion types.Int64  `tfsdk:"secrets_wo_version"`

	DigiCert        *digiCertCAModel        `tfsdk:"digicert"`
	NDESSCEPProxy   *ndesSCEPProxyCAModel   `tfsdk:"ndes_scep_proxy"`
	CustomSCEPProxy *customSCEPProxyCAModel `tfsdk:"custom_scep_proxy"`
	CustomESTProxy  *customESTProxyCAModel  `tfsdk:"custom_est_proxy"`
	Hydrant         *hydrantCAModel         `tfsdk:"hydrant"`
	Smallstep       *smallstepCAModel       `tfsdk:"smallstep"`
}

type digiCertCAModel struct {
	Name                          types.String `tfsdk:"name"`
	URL                           types.String `tfsdk:"url"`
	APIToken                      types.String `tfsdk:"api_token"`
	APITokenWO                    types.String `tfsdk:"api_token_wo"`
	ProfileID                     types.String `tfsdk:"profile_id"`
	CertificateCommonName         types.String `tfsdk:"certificate_common_name"`
	CertificateSeatID             types.String `tfsdk:"certificate_seat_id"`
	CertificateUserPrincipalNames types.List   `tfsdk:"certificate_user_principal_names"`
}

type ndesSCEPProxyCAModel struct {
	URL        types.String `tfsdk:"url"`
	AdminURL   types.String `tfsdk:"admin_url"`
	Username   types.String `tfsdk:"username"`
	Password   types.String `tfsdk:"password"`
	PasswordWO types.String `tfsdk:"password_wo"`
}

type customSCEPProxyCAModel struct {
	Name        types.String `tfsdk:"name"`
	URL         types.String `tfsdk:"url"`
	Challenge   types.String `tfsdk:"challenge"`
	ChallengeWO types.String `tfsdk:"challenge_wo"`
}

type customESTProxyCAModel struct {
	Name       types.String `tfsdk:"name"`
	URL        types.String `tfsdk:"url"`
	Username   types.String `tfsdk:"username"`
	Password   types.String `tfsdk:"password"`
	PasswordWO types.String `tfsdk:"password_wo"`
}

type hydrantCAModel struct {
	Name           types.String `tfsdk:"name"`
	URL            types.String `tfsdk:"url"`
	ClientID       types.String `tfsdk:"client_id"`
	ClientSecret   types.String `tfsdk:"client_secret"`
	ClientSecretWO types.String `tfsdk:"client_secret_wo"`
}

type smallstepCAModel struct {
	Name         types.String `tfsdk:"name"`
	URL          types.String `tfsdk:"url"`
	ChallengeURL types.String `tfsdk:"challenge_url"`
	Username     types.String `tfsdk:"username"`
	Password     types.String `tfsdk:"password"`
	PasswordWO   types.String `tfsdk:"password_wo"`
}

// configuredType reports the CA type whose block is set on the model, or an
// empty string when none is.
func (m *CertificateAuthorityResourceModel) configuredType() string {
	switch {
	case m.DigiCert != nil:
		return fleetdm.CATypeDigiCert
	case m.NDESSCEPProxy != nil:
		return fleetdm.CATypeNDESSCEPProxy
	case m.CustomSCEPProxy != nil:
		return fleetdm.CATypeCustomSCEPProxy
	case m.CustomESTProxy != nil:
		return fleetdm.CATypeCustomESTProxy
	case m.Hydrant != nil:
		return fleetdm.CATypeHydrant
	case m.Smallstep != nil:
		return fleetdm.CATypeSmallstep
	default:
		return ""
	}
}

// configuredName reports the name Fleet will store for the configured block.
// NDES is a singleton whose name Fleet fixes, so its name is not taken from
// configuration. Returns a null value when no block is set and an unknown value
// when the configured name is not yet known.
func (m *CertificateAuthorityResourceModel) configuredName() types.String {
	switch {
	case m.DigiCert != nil:
		return m.DigiCert.Name
	case m.NDESSCEPProxy != nil:
		return types.StringValue(fleetdm.NDESCAName)
	case m.CustomSCEPProxy != nil:
		return m.CustomSCEPProxy.Name
	case m.CustomESTProxy != nil:
		return m.CustomESTProxy.Name
	case m.Hydrant != nil:
		return m.Hydrant.Name
	case m.Smallstep != nil:
		return m.Smallstep.Name
	default:
		return types.StringNull()
	}
}

// Metadata returns the resource type name.
func (r *CertificateAuthorityResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate_authority"
}

// fleetNormalizedValidator rejects values Fleet would rewrite server-side.
//
// Fleet preprocesses names and URLs by trimming surrounding whitespace and
// normalising them to Unicode NFC. A configuration value that is not already in
// that form never matches what Fleet stores, so every refresh reports a diff
// that the next apply cannot settle — and for a name it is worse than cosmetic,
// because the update carries a name Fleet considers unchanged and answers with
// 409, leaving the resource permanently un-appliable.
type fleetNormalizedValidator struct{}

func (v fleetNormalizedValidator) Description(_ context.Context) string {
	return "must have no leading or trailing whitespace and be in Unicode NFC form"
}

func (v fleetNormalizedValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v fleetNormalizedValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()

	if strings.TrimSpace(value) != value {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Value",
			"Value must not have leading or trailing whitespace: Fleet trims it server-side, which would cause a permanent diff.",
		)
		return
	}

	if !norm.NFC.IsNormalString(value) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Value",
			"Value must be in Unicode NFC normalization form: Fleet normalizes it server-side, which would cause a "+
				"permanent diff. This usually means a combining accent is stored decomposed — re-enter the value or "+
				"normalize it to NFC.",
		)
	}
}

// notMaskedSecretValidator rejects Fleet's redaction placeholder as a
// configured credential.
//
// Fleet shows `********` in place of every stored CA secret, in the API and in
// its UI. Pasting that back into configuration is an easy mistake after an
// import, and it fails silently in a dangerous way: Fleet treats the mask in an
// update as "leave the stored secret unchanged", so the apply reports success
// while state records the mask as the believed credential. A later replacement
// would then POST the literal mask as the real secret, leaving a certificate
// authority whose credential is a publicly-known string.
type notMaskedSecretValidator struct{}

func (v notMaskedSecretValidator) Description(_ context.Context) string {
	return "must not be Fleet's redaction placeholder"
}

func (v notMaskedSecretValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v notMaskedSecretValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() == fleetdm.MaskedCASecret {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Certificate Authority Secret",
			fmt.Sprintf("%q is the placeholder Fleet substitutes for a stored secret, not a usable credential. "+
				"It was most likely copied from the Fleet UI or an API response. Supply the real secret; if you do "+
				"not have it, rotate the credential at the certificate authority and configure the new value.",
				fleetdm.MaskedCASecret),
		)
	}
}

// caNameAttribute builds the schema for a CA name attribute.
func caNameAttribute(caType string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: fmt.Sprintf("Name of the certificate authority. Must be unique among %s certificate authorities. "+
			"Referenced from configuration profiles as part of Fleet's `$FLEET_VAR_*` certificate variables.", caType),
		Required: true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
			fleetNormalizedValidator{},
		},
	}
}

// caURLAttribute builds the schema for a CA URL attribute.
func caURLAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Required:            true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
			fleetNormalizedValidator{},
		},
	}
}

// caSecretAttribute builds the schema for a sensitive CA credential that
// Terraform persists to state. Exactly one of it and its write-only sibling
// must be set; the constraint lives here so it is declared once per pair.
func caSecretAttribute(name, description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description + " Exactly one of `" + name + "` and `" + name + "_wo` must be set. " +
			"This attribute is stored in Terraform state — anyone who can read the state file can read it. " +
			"Prefer `" + name + "_wo` on Terraform 1.11 and later.",
		Optional:  true,
		Sensitive: true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
			notMaskedSecretValidator{},
			stringvalidator.ExactlyOneOf(
				path.MatchRelative().AtParent().AtName(name),
				path.MatchRelative().AtParent().AtName(name+"_wo"),
			),
		},
	}
}

// caSecretWOAttribute builds the schema for the write-only counterpart of a CA
// credential. Terraform keeps a write-only value out of both the plan and the
// state, so it is readable only from the configuration of the running apply,
// and a change to it is invisible to Terraform — secrets_wo_version is what
// makes a rotation visible.
func caSecretWOAttribute(name, description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description + " Write-only: Terraform never persists it to the plan or the state file. " +
			"Exactly one of `" + name + "` and `" + name + "_wo` must be set. Requires Terraform 1.11 or later. " +
			"Because Terraform cannot see a write-only value, changing it has no effect on its own — increment " +
			"`secrets_wo_version` in the same change to push the new value.",
		Optional:  true,
		Sensitive: true,
		WriteOnly: true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
			notMaskedSecretValidator{},
		},
	}
}

// caPlainAttribute builds the schema for a required non-secret CA attribute.
func caPlainAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Required:            true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
	}
}

// Schema defines the schema for the resource.
func (r *CertificateAuthorityResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages a FleetDM certificate authority (CA).

Certificate authorities let Fleet issue client certificates to hosts through configuration profiles,
using Fleet's ` + "`$FLEET_VAR_*`" + ` certificate variables. Exactly one CA type block must be set;
the type of a CA cannot be changed in place, so switching blocks replaces the resource.

~> **Note:** This is a Fleet Premium feature. It also requires Fleet's server private key
(` + "`FLEET_SERVER_PRIVATE_KEY`" + `) to be configured, because Fleet encrypts CA credentials at rest.
Without it Fleet rejects every write with "Private key must be configured".

~> **Note:** Fleet validates the configured URL when the CA is saved — it fetches the SCEP/EST CA
certificate, the DigiCert profile, or the Smallstep challenge endpoint, as applicable. An endpoint
that is unreachable from the Fleet server makes ` + "`apply`" + ` fail, even if the configuration is
otherwise correct.

## Drift detection

Fleet reports CA configuration asymmetrically, which bounds what Terraform can detect:

* **Non-secret fields** (` + "`name`" + `, ` + "`url`" + `, ` + "`admin_url`" + `, ` + "`username`" + `,
  ` + "`profile_id`" + `, ` + "`client_id`" + `, the certificate fields) **are** returned by Fleet and
  refreshed on every read, so out-of-band changes to them are detected and corrected.
* **Secrets** (` + "`challenge`" + `, ` + "`password`" + `, ` + "`api_token`" + `, ` + "`client_secret`" + `)
  are **always** masked as ` + "`********`" + `. Fleet's API hardcodes secret redaction, so no token or
  parameter returns the real value. Terraform therefore **cannot detect** a secret changed in the
  Fleet UI or by another client, whichever variant you use.
* To recover from suspected secret drift, increment ` + "`secrets_wo_version`" + `. That forces an
  update that re-sends the configured block to Fleet, re-asserting the secrets you declared.
* The list endpoint returns only ` + "`id`" + `, ` + "`name`" + ` and ` + "`type`" + `, so the
  ` + "`fleetdm_certificate_authorities`" + ` data source exposes those three fields only.

### Choosing how to supply secrets

Every credential comes in two forms — pick one per resource:

* ` + "`<name>_wo`" + ` (**preferred**, Terraform 1.11+) is write-only: Terraform reads it from
  configuration during apply and never writes it to the plan or the state file. Rotating it requires
  incrementing ` + "`secrets_wo_version`" + ` in the same change, because Terraform cannot otherwise
  see that it changed.
* ` + "`<name>`" + ` is stored in Terraform state. Rotating it is a normal attribute change, but
  **anyone who can read the state file can read the secret** — treat state as a secret store, or use
  the write-only form instead.

## Importing

Import by the CA's numeric Fleet id. Identity and every non-secret field are imported; the
credential cannot be, because Fleet never returns it. Both secret attributes import as null, so
after importing you must supply the credential and push it:

* with the in-state attribute — the next ` + "`apply`" + ` sends it, because Terraform sees the
  null-to-value change as a diff.
* with ` + "`<name>_wo`" + ` — set ` + "`secrets_wo_version`" + ` as well. A write-only value is
  invisible to Terraform, so on its own it produces no diff and no update, and Fleet keeps the
  secret it already has.

## Example Usage

### Custom SCEP proxy

` + "```hcl" + `
resource "fleetdm_certificate_authority" "scep" {
  custom_scep_proxy = {
    name      = "SCEP_WIFI"
    url       = "https://scep.example.com/scep"
    challenge = var.scep_challenge
  }
}
` + "```" + `

### Custom EST proxy

` + "```hcl" + `
resource "fleetdm_certificate_authority" "est" {
  custom_est_proxy = {
    name     = "EST_WIFI"
    url      = "https://est.example.com/.well-known/est"
    username = "fleet"
    password = var.est_password
  }
}
` + "```" + `

### DigiCert

` + "```hcl" + `
resource "fleetdm_certificate_authority" "digicert" {
  digicert = {
    name                    = "DIGICERT_WIFI"
    url                     = "https://one.digicert.com"
    api_token               = var.digicert_api_token
    profile_id              = var.digicert_profile_id
    certificate_common_name = "$FLEET_VAR_HOST_HARDWARE_SERIAL"
    certificate_seat_id     = "$FLEET_VAR_HOST_HARDWARE_SERIAL"
  }
}
` + "```" + `

### NDES SCEP proxy (singleton)

` + "```hcl" + `
resource "fleetdm_certificate_authority" "ndes" {
  ndes_scep_proxy = {
    url       = "https://ndes.example.com/certsrv/mscep/mscep.dll"
    admin_url = "https://ndes.example.com/certsrv/mscep_admin/"
    username  = "fleet@example.com"
    password  = var.ndes_password
  }
}
` + "```" + `

### Hydrant

` + "```hcl" + `
resource "fleetdm_certificate_authority" "hydrant" {
  hydrant = {
    name          = "HYDRANT_WIFI"
    url           = "https://hydrant.example.com"
    client_id     = var.hydrant_client_id
    client_secret = var.hydrant_client_secret
  }
}
` + "```" + `

### Smallstep

` + "```hcl" + `
resource "fleetdm_certificate_authority" "smallstep" {
  smallstep = {
    name          = "SMALLSTEP_WIFI"
    url           = "https://example.scep.smallstep.com/scep/agents"
    challenge_url = "https://example.scep.smallstep.com/challenge"
    username      = "fleet"
    password      = var.smallstep_password
  }
}
` + "```" + `
`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the certificate authority, assigned by Fleet.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the certificate authority as stored by Fleet. Mirrors the `name` of the " +
					"configured block; for `ndes_scep_proxy` Fleet fixes it to `NDES`.",
				Computed: true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Type of the certificate authority. One of `digicert`, `ndes_scep_proxy`, " +
					"`custom_scep_proxy`, `custom_est_proxy`, `hydrant`, `smallstep`.",
				Computed: true,
			},
			"secrets_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the block's secrets. Incrementing it forces an update that " +
					"re-sends the whole configured block to Fleet. Changing it never destroys the CA.\n\n" +
					"With a write-only credential (`*_wo`) this is **required** to rotate: Terraform cannot see a " +
					"write-only value, so editing it alone produces no diff and no update.\n\n" +
					"With an in-state credential it is optional, and useful to re-assert the declared secrets when " +
					"they may have been changed in the Fleet UI — Fleet never returns a secret, so that drift is " +
					"otherwise undetectable.",
				Optional: true,
			},

			fleetdm.CATypeDigiCert: schema.SingleNestedAttribute{
				MarkdownDescription: "DigiCert ONE certificate authority.",
				Optional:            true,
				Validators: []validator.Object{
					// Applied to a single block so a violation yields one clear
					// diagnostic; the message enumerates every block.
					objectvalidator.ExactlyOneOf(caTypeBlockPaths()...),
				},
				Attributes: map[string]schema.Attribute{
					"name":         caNameAttribute(fleetdm.CATypeDigiCert),
					"url":          caURLAttribute("Base URL of the DigiCert ONE instance, for example `https://one.digicert.com`."),
					"api_token":    caSecretAttribute("api_token", "DigiCert API token. Fleet never returns this value."),
					"api_token_wo": caSecretWOAttribute("api_token", "DigiCert API token. Fleet never returns this value."),
					"profile_id":   caPlainAttribute("GUID of the DigiCert certificate profile to issue from."),
					"certificate_common_name": caPlainAttribute("Common name (CN) of the issued certificate. " +
						"Supports Fleet host variables such as `$FLEET_VAR_HOST_HARDWARE_SERIAL`."),
					"certificate_seat_id": caPlainAttribute("Seat ID to associate the issued certificate with. " +
						"Supports Fleet host variables."),
					"certificate_user_principal_names": schema.ListAttribute{
						MarkdownDescription: "User principal names (UPNs) to place in the issued certificate's " +
							"subject alternative name. Supports Fleet host variables.",
						Optional:    true,
						ElementType: types.StringType,
					},
				},
			},

			fleetdm.CATypeNDESSCEPProxy: schema.SingleNestedAttribute{
				MarkdownDescription: "Microsoft NDES SCEP proxy certificate authority. Fleet allows only one NDES CA " +
					"per server and fixes its name to `NDES`, so this block has no `name` attribute.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"url":         caURLAttribute("NDES SCEP endpoint, for example `https://ndes.example.com/certsrv/mscep/mscep.dll`."),
					"admin_url":   caURLAttribute("NDES SCEP admin endpoint Fleet reads the enrollment challenge from, for example `https://ndes.example.com/certsrv/mscep_admin/`."),
					"username":    caPlainAttribute("Username for the NDES admin endpoint, in `user@domain` form."),
					"password":    caSecretAttribute("password", "Password for the NDES admin endpoint. Fleet never returns this value."),
					"password_wo": caSecretWOAttribute("password", "Password for the NDES admin endpoint. Fleet never returns this value."),
				},
			},

			fleetdm.CATypeCustomSCEPProxy: schema.SingleNestedAttribute{
				MarkdownDescription: "Custom SCEP proxy certificate authority.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"name":         caNameAttribute(fleetdm.CATypeCustomSCEPProxy),
					"url":          caURLAttribute("SCEP endpoint of the certificate authority."),
					"challenge":    caSecretAttribute("challenge", "Static SCEP challenge password. Fleet never returns this value."),
					"challenge_wo": caSecretWOAttribute("challenge", "Static SCEP challenge password. Fleet never returns this value."),
				},
			},

			fleetdm.CATypeCustomESTProxy: schema.SingleNestedAttribute{
				MarkdownDescription: "Custom EST (Enrollment over Secure Transport) proxy certificate authority.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"name":        caNameAttribute(fleetdm.CATypeCustomESTProxy),
					"url":         caURLAttribute("EST endpoint of the certificate authority. Fleet reads `<url>/cacerts` to validate it."),
					"username":    caPlainAttribute("Username for EST HTTP basic authentication."),
					"password":    caSecretAttribute("password", "Password for EST HTTP basic authentication. Fleet never returns this value."),
					"password_wo": caSecretWOAttribute("password", "Password for EST HTTP basic authentication. Fleet never returns this value."),
				},
			},

			fleetdm.CATypeHydrant: schema.SingleNestedAttribute{
				MarkdownDescription: "Hydrant certificate authority.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"name":             caNameAttribute(fleetdm.CATypeHydrant),
					"url":              caURLAttribute("Base URL of the Hydrant instance. Fleet reads `<url>/cacerts` to validate it."),
					"client_id":        caPlainAttribute("Hydrant OAuth client ID."),
					"client_secret":    caSecretAttribute("client_secret", "Hydrant OAuth client secret. Fleet never returns this value."),
					"client_secret_wo": caSecretWOAttribute("client_secret", "Hydrant OAuth client secret. Fleet never returns this value."),
				},
			},

			fleetdm.CATypeSmallstep: schema.SingleNestedAttribute{
				MarkdownDescription: "Smallstep SCEP certificate authority.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"name":          caNameAttribute(fleetdm.CATypeSmallstep),
					"url":           caURLAttribute("Smallstep SCEP endpoint."),
					"challenge_url": caURLAttribute("Smallstep challenge webhook endpoint Fleet requests a SCEP challenge from."),
					"username":      caPlainAttribute("Username for the Smallstep challenge endpoint."),
					"password":      caSecretAttribute("password", "Password for the Smallstep challenge endpoint. Fleet never returns this value."),
					"password_wo":   caSecretWOAttribute("password", "Password for the Smallstep challenge endpoint. Fleet never returns this value."),
				},
			},
		},
	}
}

// ConfigValidators nudges practitioners on Terraform 1.11 and later towards the
// write-only variant of each credential.
//
// Unlike the ExactlyOneOf constraint on each pair, which is declared on the
// attribute itself so it only fires for the block actually configured, these
// are resource-level and cover every pair.
//
// Note that `secrets_wo_version` is deliberately *not* declared as conflicting
// with the in-state credentials. It is required to push a `_wo` rotation, but it
// is also meaningful on the in-state path, where bumping it re-sends the
// configured block and re-asserts secrets Fleet may have had changed out of
// band — the only remedy for drift Fleet will not report.
func (r *CertificateAuthorityResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	validators := make([]resource.ConfigValidator, 0, len(caSecretPairs))
	for _, pair := range caSecretPairs {
		validators = append(validators, resourcevalidator.PreferWriteOnlyAttribute(
			path.MatchRoot(pair.block).AtName(pair.attr),
			path.MatchRoot(pair.block).AtName(pair.attr+"_wo"),
		))
	}
	return validators
}

// Configure adds the provider configured client to the resource.
func (r *CertificateAuthorityResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// ModifyPlan resolves the computed name and type from the configured block and
// forces replacement when the CA type changes. Fleet rejects a type change on
// its update endpoint ("The certificate authority types must be the same"), and
// both name and type are fully determined by configuration, so planning them
// here avoids a spurious "known after apply".
// It reads each block as a types.Object rather than decoding the whole model,
// because a block can be wholly unknown at plan time when it is derived from
// another resource's output. An unknown object cannot be converted into the
// typed model's *struct, and attempting it fails the plan with a Value
// Conversion Error before any of this logic runs.
func (r *CertificateAuthorityResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		// Destroy plan: nothing to resolve.
		return
	}

	planType, planBlock := configuredBlock(ctx, blockGetter(req.Plan.GetAttribute), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if planType == "" {
		// The ExactlyOneOf validator reports this; nothing to plan.
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("type"), types.StringValue(planType))...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("name"), plannedCAName(planType, planBlock))...)
	if resp.Diagnostics.HasError() {
		return
	}

	if req.State.Raw.IsNull() {
		// Create plan: no prior type to compare against.
		return
	}

	stateType, _ := configuredBlock(ctx, blockGetter(req.State.GetAttribute), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if stateType != "" && stateType != planType {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root(stateType), path.Root(planType))
	}
}

// blockGetter adapts tfsdk.Plan/State GetAttribute to a common signature.
type blockGetter func(ctx context.Context, p path.Path, target interface{}) diag.Diagnostics

// configuredBlock reports which CA type block is set and returns it as an
// object. A block that is unknown as a whole still counts as set.
func configuredBlock(ctx context.Context, get blockGetter, diags *diag.Diagnostics) (string, types.Object) {
	for _, name := range caTypeBlocks {
		var block types.Object
		diags.Append(get(ctx, path.Root(name), &block)...)
		if diags.HasError() {
			return "", types.ObjectNull(nil)
		}
		if !block.IsNull() {
			return name, block
		}
	}
	return "", types.ObjectNull(nil)
}

// plannedCAName resolves the name Fleet will store for the block being planned.
// NDES is a singleton whose name Fleet fixes, so it never depends on the block.
// Anything not yet knowable is planned as unknown rather than guessed.
func plannedCAName(caType string, block types.Object) types.String {
	if caType == fleetdm.CATypeNDESSCEPProxy {
		return types.StringValue(fleetdm.NDESCAName)
	}
	if block.IsUnknown() || block.IsNull() {
		return types.StringUnknown()
	}
	name, ok := block.Attributes()["name"].(types.String)
	if !ok {
		return types.StringUnknown()
	}
	return name
}

// resolveCASecret returns the secret to send for one CA credential, taking it
// from whichever of the pair is configured.
//
// The write-only variant is present only in the configuration of the running
// apply — Terraform keeps it out of both the plan and the state — so it has to
// be read straight from config rather than from the model built out of the plan.
func resolveCASecret(ctx context.Context, config tfsdk.Config, blockName, attrName string, planned types.String, diags *diag.Diagnostics) string {
	var writeOnly types.String
	diags.Append(config.GetAttribute(ctx, path.Root(blockName).AtName(attrName+"_wo"), &writeOnly)...)
	if diags.HasError() {
		return ""
	}

	if !writeOnly.IsNull() && !writeOnly.IsUnknown() {
		return writeOnly.ValueString()
	}
	if !planned.IsNull() && !planned.IsUnknown() {
		return planned.ValueString()
	}

	diags.AddError(
		"Missing Certificate Authority Secret",
		fmt.Sprintf("Neither `%[1]s.%[2]s` nor `%[1]s.%[2]s_wo` resolved to a usable value at apply time. "+
			"Set exactly one of them to a non-empty string.", blockName, attrName),
	)
	return ""
}

// nullifyWriteOnlySecrets clears every write-only attribute before the model is
// written to state. Terraform requires write-only values to be absent from
// state, and the framework rejects a non-null one.
func (m *CertificateAuthorityResourceModel) nullifyWriteOnlySecrets() {
	switch {
	case m.DigiCert != nil:
		m.DigiCert.APITokenWO = types.StringNull()
	case m.NDESSCEPProxy != nil:
		m.NDESSCEPProxy.PasswordWO = types.StringNull()
	case m.CustomSCEPProxy != nil:
		m.CustomSCEPProxy.ChallengeWO = types.StringNull()
	case m.CustomESTProxy != nil:
		m.CustomESTProxy.PasswordWO = types.StringNull()
	case m.Hydrant != nil:
		m.Hydrant.ClientSecretWO = types.StringNull()
	case m.Smallstep != nil:
		m.Smallstep.PasswordWO = types.StringNull()
	}
}

// buildPayload converts the model into a Fleet API payload. The complete block
// is always sent: Fleet refuses to change a URL unless the type's secret
// accompanies it in the same request.
func (m *CertificateAuthorityResourceModel) buildPayload(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) *fleetdm.CertificateAuthorityPayload {
	payload := &fleetdm.CertificateAuthorityPayload{}

	switch {
	case m.DigiCert != nil:
		upns := stringListToSlice(ctx, m.DigiCert.CertificateUserPrincipalNames, diags)
		payload.DigiCert = &fleetdm.DigiCertCA{
			Name:                          m.DigiCert.Name.ValueStringPointer(),
			URL:                           m.DigiCert.URL.ValueString(),
			APIToken:                      resolveCASecret(ctx, config, fleetdm.CATypeDigiCert, "api_token", m.DigiCert.APIToken, diags),
			ProfileID:                     m.DigiCert.ProfileID.ValueString(),
			CertificateCommonName:         m.DigiCert.CertificateCommonName.ValueString(),
			CertificateSeatID:             m.DigiCert.CertificateSeatID.ValueString(),
			CertificateUserPrincipalNames: &upns,
		}
	case m.NDESSCEPProxy != nil:
		payload.NDESSCEPProxy = &fleetdm.NDESSCEPProxyCA{
			URL:      m.NDESSCEPProxy.URL.ValueString(),
			AdminURL: m.NDESSCEPProxy.AdminURL.ValueString(),
			Username: m.NDESSCEPProxy.Username.ValueString(),
			Password: resolveCASecret(ctx, config, fleetdm.CATypeNDESSCEPProxy, "password", m.NDESSCEPProxy.Password, diags),
		}
	case m.CustomSCEPProxy != nil:
		payload.CustomSCEPProxy = &fleetdm.CustomSCEPProxyCA{
			Name:      m.CustomSCEPProxy.Name.ValueStringPointer(),
			URL:       m.CustomSCEPProxy.URL.ValueString(),
			Challenge: resolveCASecret(ctx, config, fleetdm.CATypeCustomSCEPProxy, "challenge", m.CustomSCEPProxy.Challenge, diags),
		}
	case m.CustomESTProxy != nil:
		payload.CustomESTProxy = &fleetdm.CustomESTProxyCA{
			Name:     m.CustomESTProxy.Name.ValueStringPointer(),
			URL:      m.CustomESTProxy.URL.ValueString(),
			Username: m.CustomESTProxy.Username.ValueString(),
			Password: resolveCASecret(ctx, config, fleetdm.CATypeCustomESTProxy, "password", m.CustomESTProxy.Password, diags),
		}
	case m.Hydrant != nil:
		payload.Hydrant = &fleetdm.HydrantCA{
			Name:         m.Hydrant.Name.ValueStringPointer(),
			URL:          m.Hydrant.URL.ValueString(),
			ClientID:     m.Hydrant.ClientID.ValueString(),
			ClientSecret: resolveCASecret(ctx, config, fleetdm.CATypeHydrant, "client_secret", m.Hydrant.ClientSecret, diags),
		}
	case m.Smallstep != nil:
		payload.Smallstep = &fleetdm.SmallstepSCEPProxyCA{
			Name:         m.Smallstep.Name.ValueStringPointer(),
			URL:          m.Smallstep.URL.ValueString(),
			ChallengeURL: m.Smallstep.ChallengeURL.ValueString(),
			Username:     m.Smallstep.Username.ValueString(),
			Password:     resolveCASecret(ctx, config, fleetdm.CATypeSmallstep, "password", m.Smallstep.Password, diags),
		}
	default:
		diags.AddError(
			"No Certificate Authority Configured",
			"Exactly one certificate authority type block must be set.",
		)
		return nil
	}

	return payload
}

// stringListToSlice converts a types.List of strings into a slice, mapping a
// null or unknown list to an empty (non-nil) slice so Fleet receives `[]` and
// clears any stored value rather than leaving it untouched.
func stringListToSlice(ctx context.Context, list types.List, diags *diag.Diagnostics) []string {
	result := []string{}
	if list.IsNull() || list.IsUnknown() {
		return result
	}
	diags.Append(list.ElementsAs(ctx, &result, false)...)
	if result == nil {
		result = []string{}
	}
	return result
}

// Create creates the resource and sets the initial Terraform state.
func (r *CertificateAuthorityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CertificateAuthorityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := data.buildPayload(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating certificate authority", map[string]interface{}{
		"type": payload.Type(),
	})

	summary, err := r.client.CreateCertificateAuthority(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Certificate Authority",
			"Could not create certificate authority: "+err.Error(),
		)
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%d", summary.ID))
	data.Name = types.StringValue(summary.Name)
	data.Type = types.StringValue(summary.Type)

	tflog.Info(ctx, "Created certificate authority", map[string]interface{}{
		"id":   data.ID.ValueString(),
		"type": data.Type.ValueString(),
	})

	data.nullifyWriteOnlySecrets()
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the Terraform state with the latest data. Non-secret fields
// are refreshed from Fleet; secrets are preserved from prior state because
// Fleet always masks them.
func (r *CertificateAuthorityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CertificateAuthorityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := parseIDFromString(data.ID.ValueString(), "certificate authority", &resp.Diagnostics)
	if !ok {
		return
	}

	ca, err := r.client.GetCertificateAuthority(ctx, int(id))
	if err != nil {
		if isNotFound(err) {
			tflog.Info(ctx, "Certificate authority not found, removing from state", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Certificate Authority",
			"Could not read certificate authority: "+err.Error(),
		)
		return
	}

	stateType := data.configuredType()
	if stateType != "" && ca.Type != stateType {
		// The id now points at a different kind of CA, which Fleet's update
		// endpoint cannot reconcile. Refuse rather than silently rewriting the
		// configuration block.
		resp.Diagnostics.AddError(
			"Certificate Authority Type Changed Outside Terraform",
			fmt.Sprintf("Certificate authority %s is of type %q in Fleet but %q in Terraform state. "+
				"Fleet cannot change a CA's type in place, so this id refers to a different CA than the one "+
				"Terraform created. Remove it from state and re-import it.", data.ID.ValueString(), ca.Type, stateType),
		)
		return
	}

	data.Name = types.StringValue(ca.Name)
	data.Type = types.StringValue(ca.Type)
	data.refreshFromAPI(ctx, ca, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	data.nullifyWriteOnlySecrets()
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// preserveSecret returns the value to store for a secret attribute.
//
// Fleet masks every stored secret, so the prior value is kept unless Fleet
// returned something that is neither empty nor the mask. A null prior value is
// never replaced: that is the write-only path, and an imported resource before
// its credential is supplied. Fleet is not supposed to return a real secret at
// all, so if it ever did, persisting it there would write a credential into
// state that the practitioner deliberately kept out of it.
func preserveSecret(apiValue string, stateValue types.String) types.String {
	if stateValue.IsNull() {
		return stateValue
	}
	if apiValue != "" && apiValue != fleetdm.MaskedCASecret {
		return types.StringValue(apiValue)
	}
	return stateValue
}

// preserveUPNList returns the value to store for the DigiCert user principal
// name list.
//
// The provider sends `[]` for both an unset list and an explicitly empty one,
// so Fleet reports the two identically and cannot tell them apart. Overwriting
// either form with the other would produce a diff that never converges, so when
// the response is semantically equal to what state already holds the configured
// form is kept verbatim. A genuine change — including an out-of-band clear —
// still wins, so drift on this field stays detectable.
func preserveUPNList(ctx context.Context, apiUPNs []string, stateValue types.List, diags *diag.Diagnostics) types.List {
	stateUPNs := []string{}
	if !stateValue.IsNull() && !stateValue.IsUnknown() {
		diags.Append(stateValue.ElementsAs(ctx, &stateUPNs, false)...)
	}
	if slices.Equal(stateUPNs, apiUPNs) {
		return stateValue
	}
	if len(apiUPNs) == 0 {
		return types.ListNull(types.StringType)
	}
	upns, d := types.ListValueFrom(ctx, types.StringType, apiUPNs)
	diags.Append(d...)
	return upns
}

// refreshFromAPI updates the configured block's non-secret fields from Fleet's
// response, leaving secrets as they are in state.
func (m *CertificateAuthorityResourceModel) refreshFromAPI(ctx context.Context, ca *fleetdm.CertificateAuthority, diags *diag.Diagnostics) {
	switch {
	case m.DigiCert != nil:
		m.DigiCert.Name = types.StringValue(ca.Name)
		m.DigiCert.URL = types.StringValue(ca.URL)
		m.DigiCert.ProfileID = types.StringValue(ca.ProfileID)
		m.DigiCert.CertificateCommonName = types.StringValue(ca.CertificateCommonName)
		m.DigiCert.CertificateSeatID = types.StringValue(ca.CertificateSeatID)
		m.DigiCert.APIToken = preserveSecret(ca.APIToken, m.DigiCert.APIToken)
		m.DigiCert.CertificateUserPrincipalNames = preserveUPNList(
			ctx, ca.CertificateUserPrincipalNames, m.DigiCert.CertificateUserPrincipalNames, diags)
	case m.NDESSCEPProxy != nil:
		m.NDESSCEPProxy.URL = types.StringValue(ca.URL)
		m.NDESSCEPProxy.AdminURL = types.StringValue(ca.AdminURL)
		m.NDESSCEPProxy.Username = types.StringValue(ca.Username)
		m.NDESSCEPProxy.Password = preserveSecret(ca.Password, m.NDESSCEPProxy.Password)
	case m.CustomSCEPProxy != nil:
		m.CustomSCEPProxy.Name = types.StringValue(ca.Name)
		m.CustomSCEPProxy.URL = types.StringValue(ca.URL)
		m.CustomSCEPProxy.Challenge = preserveSecret(ca.Challenge, m.CustomSCEPProxy.Challenge)
	case m.CustomESTProxy != nil:
		m.CustomESTProxy.Name = types.StringValue(ca.Name)
		m.CustomESTProxy.URL = types.StringValue(ca.URL)
		m.CustomESTProxy.Username = types.StringValue(ca.Username)
		m.CustomESTProxy.Password = preserveSecret(ca.Password, m.CustomESTProxy.Password)
	case m.Hydrant != nil:
		m.Hydrant.Name = types.StringValue(ca.Name)
		m.Hydrant.URL = types.StringValue(ca.URL)
		m.Hydrant.ClientID = types.StringValue(ca.ClientID)
		m.Hydrant.ClientSecret = preserveSecret(ca.ClientSecret, m.Hydrant.ClientSecret)
	case m.Smallstep != nil:
		m.Smallstep.Name = types.StringValue(ca.Name)
		m.Smallstep.URL = types.StringValue(ca.URL)
		m.Smallstep.ChallengeURL = types.StringValue(ca.ChallengeURL)
		m.Smallstep.Username = types.StringValue(ca.Username)
		m.Smallstep.Password = preserveSecret(ca.Password, m.Smallstep.Password)
	}
}

// Update updates the resource in place. Fleet 4.90 supports
// PATCH /certificate_authorities/{id} for every CA type; a type change is
// rejected and is planned as a replacement instead.
func (r *CertificateAuthorityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CertificateAuthorityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state CertificateAuthorityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := parseIDFromString(state.ID.ValueString(), "certificate authority", &resp.Diagnostics)
	if !ok {
		return
	}

	payload := data.buildPayload(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fleet checks name uniqueness on update without excluding the CA being
	// updated, so re-sending the current name comes back as 409 "a certificate
	// authority with this name already exists". Send the name only on a rename.
	if name, ok := payload.Name(); ok && name == state.Name.ValueString() {
		payload.ClearName()
	}

	tflog.Debug(ctx, "Updating certificate authority", map[string]interface{}{
		"id":   state.ID.ValueString(),
		"type": payload.Type(),
	})

	if err := r.client.UpdateCertificateAuthority(ctx, int(id), payload); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Certificate Authority",
			"Could not update certificate authority: "+err.Error(),
		)
		return
	}

	data.ID = state.ID
	data.Type = types.StringValue(payload.Type())
	data.Name = data.configuredName()

	tflog.Info(ctx, "Updated certificate authority", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	data.nullifyWriteOnlySecrets()
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource. Fleet returns a conflict when certificate
// templates still reference the CA.
func (r *CertificateAuthorityResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CertificateAuthorityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := parseIDFromString(data.ID.ValueString(), "certificate authority", &resp.Diagnostics)
	if !ok {
		return
	}

	tflog.Debug(ctx, "Deleting certificate authority", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	if err := r.client.DeleteCertificateAuthority(ctx, int(id)); err != nil {
		if isNotFound(err) {
			return
		}
		if isConflict(err) {
			resp.Diagnostics.AddError(
				"Certificate Authority Still Referenced",
				"Could not delete certificate authority because Fleet still has references to it "+
					"(for example certificate templates). Remove those references first: "+err.Error(),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Certificate Authority",
			"Could not delete certificate authority: "+err.Error(),
		)
	}
}

// ImportState imports an existing certificate authority by its numeric id.
//
// The import is necessarily partial: Fleet masks every CA secret, so the
// secret attributes cannot be populated. They are left null, which makes the
// first plan after import show a diff that pushes the secrets from
// configuration to Fleet. Non-secret fields are imported and comparable.
func (r *CertificateAuthorityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, ok := parseIDFromString(req.ID, "certificate authority", &resp.Diagnostics)
	if !ok {
		return
	}

	ca, err := r.client.GetCertificateAuthority(ctx, int(id))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Certificate Authority",
			fmt.Sprintf("Could not read certificate authority %s: %s", req.ID, err.Error()),
		)
		return
	}

	data := CertificateAuthorityResourceModel{
		ID:               types.StringValue(fmt.Sprintf("%d", ca.ID)),
		Name:             types.StringValue(ca.Name),
		Type:             types.StringValue(ca.Type),
		SecretsWOVersion: types.Int64Null(),
	}

	switch ca.Type {
	case fleetdm.CATypeDigiCert:
		upns := types.ListNull(types.StringType)
		if len(ca.CertificateUserPrincipalNames) > 0 {
			converted, d := types.ListValueFrom(ctx, types.StringType, ca.CertificateUserPrincipalNames)
			resp.Diagnostics.Append(d...)
			upns = converted
		}
		data.DigiCert = &digiCertCAModel{
			Name:                          types.StringValue(ca.Name),
			URL:                           types.StringValue(ca.URL),
			APIToken:                      types.StringNull(),
			ProfileID:                     types.StringValue(ca.ProfileID),
			CertificateCommonName:         types.StringValue(ca.CertificateCommonName),
			CertificateSeatID:             types.StringValue(ca.CertificateSeatID),
			CertificateUserPrincipalNames: upns,
		}
	case fleetdm.CATypeNDESSCEPProxy:
		data.NDESSCEPProxy = &ndesSCEPProxyCAModel{
			URL:      types.StringValue(ca.URL),
			AdminURL: types.StringValue(ca.AdminURL),
			Username: types.StringValue(ca.Username),
			Password: types.StringNull(),
		}
	case fleetdm.CATypeCustomSCEPProxy:
		data.CustomSCEPProxy = &customSCEPProxyCAModel{
			Name:      types.StringValue(ca.Name),
			URL:       types.StringValue(ca.URL),
			Challenge: types.StringNull(),
		}
	case fleetdm.CATypeCustomESTProxy:
		data.CustomESTProxy = &customESTProxyCAModel{
			Name:     types.StringValue(ca.Name),
			URL:      types.StringValue(ca.URL),
			Username: types.StringValue(ca.Username),
			Password: types.StringNull(),
		}
	case fleetdm.CATypeHydrant:
		data.Hydrant = &hydrantCAModel{
			Name:         types.StringValue(ca.Name),
			URL:          types.StringValue(ca.URL),
			ClientID:     types.StringValue(ca.ClientID),
			ClientSecret: types.StringNull(),
		}
	case fleetdm.CATypeSmallstep:
		data.Smallstep = &smallstepCAModel{
			Name:         types.StringValue(ca.Name),
			URL:          types.StringValue(ca.URL),
			ChallengeURL: types.StringValue(ca.ChallengeURL),
			Username:     types.StringValue(ca.Username),
			Password:     types.StringNull(),
		}
	default:
		resp.Diagnostics.AddError(
			"Unsupported Certificate Authority Type",
			fmt.Sprintf("Certificate authority %s has type %q, which this provider version does not support.", req.ID, ca.Type),
		)
		return
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddWarning(
		"Certificate Authority Secrets Not Imported",
		fmt.Sprintf("Fleet never returns certificate authority secrets, so the secret attributes of %q were imported as null. "+
			"Supply the credential in configuration, then push it to Fleet:\n\n"+
			"  * with the in-state attribute, the next apply sends it — Terraform sees the null-to-value change as a diff;\n"+
			"  * with the write-only `_wo` attribute, set `secrets_wo_version` as well. Terraform cannot see a write-only "+
			"value, so a `_wo` credential on its own produces no diff and no update, and Fleet keeps the secret it already has.",
			ca.Name),
	)

	tflog.Info(ctx, "Imported certificate authority", map[string]interface{}{
		"id":   data.ID.ValueString(),
		"type": ca.Type,
	})

	data.nullifyWriteOnlySecrets()
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
