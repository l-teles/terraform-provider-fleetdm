package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	gopath "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// hexSHA256Re matches a lowercase hex-encoded SHA256.
var hexSHA256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// errS3SourceUnknown signals that package_s3.bucket or package_s3.key has not
// yet been resolved to a concrete string. Returned by buildS3Source for the
// "deferred resolution" case (module output backed by a resource being
// created in the same apply). Callers in ModifyPlan / resolveRemoteSHA treat
// this as a soft-skip, not an error: Terraform's graph evaluation will
// resolve the value before Create/Update is invoked at apply time.
var errS3SourceUnknown = errors.New("package_s3 bucket or key not yet known")

// fetchS3SHA256 is the function used to resolve the SHA256 of an S3 object via
// HeadObject. It is a package-level variable so tests can stub it without
// needing real S3 / httptest plumbing for every unit-style assertion.
var fetchS3SHA256 = fleetdm.FetchS3ObjectSHA256

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                   = &softwarePackageResource{}
	_ resource.ResourceWithConfigure      = &softwarePackageResource{}
	_ resource.ResourceWithImportState    = &softwarePackageResource{}
	_ resource.ResourceWithValidateConfig = &softwarePackageResource{}
	_ resource.ResourceWithModifyPlan     = &softwarePackageResource{}
)

// NewSoftwarePackageResource is a helper function to simplify the provider implementation.
func NewSoftwarePackageResource() resource.Resource {
	return &softwarePackageResource{}
}

// softwarePackageResource is the resource implementation.
type softwarePackageResource struct {
	client *fleetdm.Client
}

// softwarePackageResourceModel maps the resource schema data.
type softwarePackageResourceModel struct {
	ID                       types.Int64  `tfsdk:"id"`
	TitleID                  types.Int64  `tfsdk:"title_id"`
	TeamID                   types.Int64  `tfsdk:"team_id"`
	Type                     types.String `tfsdk:"type"`
	Name                     types.String `tfsdk:"name"`
	Version                  types.String `tfsdk:"version"`
	DisplayName              types.String `tfsdk:"display_name"`
	Filename                 types.String `tfsdk:"filename"`
	PackagePath              types.String `tfsdk:"package_path"`
	PackageS3                types.Object `tfsdk:"package_s3"`
	PackageSHA256            types.String `tfsdk:"package_sha256"`
	Platform                 types.String `tfsdk:"platform"`
	InstallScript            types.String `tfsdk:"install_script"`
	UninstallScript          types.String `tfsdk:"uninstall_script"`
	PreInstallQuery          types.String `tfsdk:"pre_install_query"`
	PostInstallScript        types.String `tfsdk:"post_install_script"`
	SelfService              types.Bool   `tfsdk:"self_service"`
	AutomaticInstall         types.Bool   `tfsdk:"automatic_install"`
	Categories               types.List   `tfsdk:"categories"`
	LabelsIncludeAny         types.List   `tfsdk:"labels_include_any"`
	LabelsExcludeAny         types.List   `tfsdk:"labels_exclude_any"`
	LabelsIncludeAll         types.List   `tfsdk:"labels_include_all"`
	AppStoreID               types.String `tfsdk:"app_store_id"`
	FleetMaintainedAppID     types.Int64  `tfsdk:"fleet_maintained_app_id"`
	AutomaticInstallPolicies types.List   `tfsdk:"automatic_install_policies"`
}

// packageS3Model maps the nested package_s3 attribute.
type packageS3Model struct {
	Bucket         types.String `tfsdk:"bucket"`
	Key            types.String `tfsdk:"key"`
	Region         types.String `tfsdk:"region"`
	EndpointURL    types.String `tfsdk:"endpoint_url"`
	ExpectedSHA256 types.String `tfsdk:"expected_sha256"`
}

// Metadata returns the resource type name.
func (r *softwarePackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_package"
}

// Schema defines the schema for the resource.
func (r *softwarePackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FleetDM software package, VPP (App Store) app, or Fleet Maintained App. This is a Premium feature.",
		DeprecationMessage: "fleetdm_software_package is deprecated and will be removed in a future major release. " +
			"Use one of the type-specific resources depending on the value of `type`:\n" +
			"  - type = \"package\"          -> fleetdm_software_custom_package\n" +
			"  - type = \"vpp\"              -> fleetdm_software_app_store_app\n" +
			"  - type = \"fleet_maintained\" -> fleetdm_software_fleet_maintained_app\n" +
			"See the \"Migrating from fleetdm_software_package\" guide in the provider documentation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "The unique identifier (internal, same as title_id).",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"title_id": schema.Int64Attribute{
				Description: "The software title ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.Int64Attribute{
				Description: "The ID of the team this software package belongs to. Required for Fleet Premium.",
				Optional:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Description: "The type of software to manage. One of: `package` (default) — upload a local installer file (.pkg, .msi, .deb, .rpm, .exe); `vpp` — add an App Store app via Apple Volume Purchase Program, requires `app_store_id`; `fleet_maintained` — add a Fleet-curated app, requires `fleet_maintained_app_id`. Changing this value forces a new resource.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("package"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the software (extracted from the package or App Store).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.StringAttribute{
				Description: "The version of the software.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"filename": schema.StringAttribute{
				Description: "The filename of the package (e.g., 'myapp-1.0.0.pkg'). Required for type 'package'.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"package_path": schema.StringAttribute{
				Description: "The filesystem path to the software package file. If set, the file will be uploaded to Fleet when its SHA256 differs from the current package. Supports .pkg, .msi, .deb, .rpm, and .exe files. Mutually exclusive with package_s3.",
				Optional:    true,
			},
			"package_s3": schema.SingleNestedAttribute{
				Description: "S3 source for the software package. Alternative to package_path. The provider reads the SHA256 via HeadObject and only downloads + re-uploads to Fleet when the hash differs from what Fleet has stored. Mutually exclusive with package_path. `bucket`, `key`, and `region` may reference module outputs or other resources' attributes — when their values aren't yet known at plan time, the SHA comparison is deferred to apply time.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"bucket": schema.StringAttribute{
						Description: "The S3 bucket name.",
						Required:    true,
					},
					"key": schema.StringAttribute{
						Description: "The S3 object key.",
						Required:    true,
					},
					"region": schema.StringAttribute{
						Description: "The AWS region. Uses AWS_REGION or default config if omitted.",
						Optional:    true,
					},
					"endpoint_url": schema.StringAttribute{
						Description: "Custom S3 endpoint URL. Useful for S3-compatible services like LocalStack or MinIO.",
						Optional:    true,
					},
					"expected_sha256": schema.StringAttribute{
						Description: "Lowercase hex SHA256 of the S3 object's content, asserted out-of-band. " +
							"When set, the provider skips HeadObject and trusts this value as the remote SHA. " +
							"Use this when the bucket is read-only to your runner and you cannot add a SHA256 " +
							"checksum or `x-amz-meta-sha256` metadata to the object. You are responsible for " +
							"keeping this value in sync with the actual object — if it's wrong, Fleet will " +
							"think the installer is unchanged and the package will NOT be re-uploaded. " +
							"See the 'SHA256 verification with S3 sources' section of the documentation.",
						Optional: true,
					},
				},
			},
			"package_sha256": schema.StringAttribute{
				Description: "The SHA256 hash of the package in Fleet. Computed from the local file (package_path) or S3 object (package_s3) on create/update, or read from Fleet API. Can be set explicitly to avoid drift on import.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform": schema.StringAttribute{
				Description: "The platform (darwin, windows, linux, ipados, ios). Computed for packages, optional for VPP apps.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"install_script": schema.StringAttribute{
				Description: "The script to run during installation. Optional. Used by type 'package' and 'fleet_maintained'.",
				Optional:    true,
			},
			"uninstall_script": schema.StringAttribute{
				Description: "The script to run during uninstallation. Optional. Used by type 'package'.",
				Optional:    true,
			},
			"pre_install_query": schema.StringAttribute{
				Description: "An osquery SQL query to run before installation. Installation proceeds only if the query returns results. Optional.",
				Optional:    true,
			},
			"post_install_script": schema.StringAttribute{
				Description: "The script to run after installation. Optional.",
				Optional:    true,
			},
			"self_service": schema.BoolAttribute{
				Description: "Whether the software is available for self-service installation by end users. Defaults to false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"automatic_install": schema.BoolAttribute{
				Description: "Type-dependent flag. " +
					"For `type=package` and `type=vpp`: flags the title for install during the device's first-boot Setup Assistant via Fleet's " +
					"`PUT /setup_experience/software` endpoint. " +
					"For `type=fleet_maintained`: creates a Fleet *policy* that installs the software on hosts missing it. Fleet only honors this " +
					"at Create time for FMA — changing the value after Create has no supported wire path and will produce a plan-time error. " +
					"Deprecated: prefer the type-specific resources (`fleetdm_software_custom_package`, `fleetdm_software_app_store_app`, " +
					"`fleetdm_software_fleet_maintained_app`) which expose `install_during_setup` and `automatic_install_policy` as separate attributes. " +
					"Defaults to false.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"labels_include_any": schema.ListAttribute{
				Description: "List of label names. The software will be available for hosts that match *any* of these labels. " +
					"Mutually exclusive with `labels_exclude_any` and `labels_include_all` — Fleet's API rejects requests that set more than one of the three. " +
					"To clear previously-set labels, set this attribute to `[]` explicitly; omitting the attribute preserves Fleet's existing labels.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					listvalidator.ConflictsWith(path.Expressions{
						path.MatchRoot("labels_exclude_any"),
						path.MatchRoot("labels_include_all"),
					}...),
				},
			},
			"labels_exclude_any": schema.ListAttribute{
				Description: "List of label names. The software will not be available for hosts that match any of these labels. " +
					"Mutually exclusive with `labels_include_any` and `labels_include_all`. " +
					"To clear previously-set labels, set this attribute to `[]` explicitly; omitting the attribute preserves Fleet's existing labels.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					listvalidator.ConflictsWith(path.Expressions{
						path.MatchRoot("labels_include_all"),
					}...),
				},
			},
			"app_store_id": schema.StringAttribute{
				Description: "The App Store ID (Adam ID) for VPP apps. Required when type is 'vpp'.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"fleet_maintained_app_id": schema.Int64Attribute{
				Description: "The Fleet Maintained App ID. Required when type is 'fleet_maintained'.",
				Optional:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "End-user-visible name shown for this software in Fleet's UI. Optional override for Fleet's auto-derived name; Computed when omitted. " +
					"Added in the same release that introduces the three type-specific replacement resources; the new resources expose the same attribute.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"categories": schema.ListAttribute{
				Description: "Self-service categories the software appears under on the end-user's *My device* page. Only applicable to `type = \"package\"` and `type = \"fleet_maintained\"` (VPP doesn't support categories).",
				Optional:    true,
				ElementType: types.StringType,
			},
			"labels_include_all": schema.ListAttribute{
				Description: "List of label names. The software will be available for hosts that match *all* of these labels. " +
					"Mutually exclusive with `labels_include_any` and `labels_exclude_any` — Fleet's API rejects requests that set more than one. " +
					"To clear previously-set labels, set this attribute to `[]` explicitly; omitting the attribute preserves Fleet's existing labels.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"automatic_install_policies": schema.ListNestedAttribute{
				Description: "Computed. List of Fleet policies that auto-install this software title on hosts that fail the policy.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.Int64Attribute{Computed: true},
						"name": schema.StringAttribute{Computed: true},
					},
				},
				PlanModifiers: []planmodifier.List{
					automaticInstallPoliciesUseStateForUnknown{},
				},
			},
		},
	}
}

// ValidateConfig validates the resource configuration at plan time.
func (r *softwarePackageResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data softwarePackageResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasPath := !data.PackagePath.IsNull() && !data.PackagePath.IsUnknown() && data.PackagePath.ValueString() != ""
	hasS3 := !data.PackageS3.IsNull() && !data.PackageS3.IsUnknown()
	swType := data.Type.ValueString()

	if hasPath && hasS3 {
		resp.Diagnostics.AddAttributeError(
			path.Root("package_s3"),
			"Conflicting Configuration",
			"package_path and package_s3 are mutually exclusive. Set one or the other, not both.",
		)
	}

	if (hasPath || hasS3) && swType != "" && swType != "package" {
		resp.Diagnostics.AddAttributeError(
			path.Root("type"),
			"Invalid Configuration",
			"package_path and package_s3 can only be used with type = \"package\". "+
				"VPP and Fleet Maintained apps are managed through the Fleet API directly.",
		)
	}

	// Fleet's VPP endpoints don't accept categories or labels_include_all.
	// Silently dropping these would cause a perpetual diff (HCL has the
	// value, Fleet doesn't, refresh wipes state). Error at plan-time.
	if swType == "vpp" {
		if !data.Categories.IsNull() && !data.Categories.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("categories"),
				"Invalid Configuration",
				"`categories` is not supported for type = \"vpp\". Fleet's VPP API doesn't accept categories. Remove the attribute or migrate to type=package via fleetdm_software_custom_package.",
			)
		}
		if !data.LabelsIncludeAll.IsNull() && !data.LabelsIncludeAll.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("labels_include_all"),
				"Invalid Configuration",
				"`labels_include_all` is not supported for type = \"vpp\". Fleet's Add App Store App endpoint doesn't accept labels at create time. Use the new `fleetdm_software_app_store_app` resource (which exposes labels_include_all on Update) instead.",
			)
		}
	}

	// Validate package_s3 fields when the block is present.
	if hasS3 {
		var s3Config packageS3Model
		diags := data.PackageS3.As(ctx, &s3Config, basetypes.ObjectAsOptions{})
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		resp.Diagnostics.Append(validatePackageS3(s3Config)...)
	}
}

// validatePackageS3 checks the inner fields of a package_s3 block. It is
// intentionally separated from ValidateConfig so unit tests can drive it
// with crafted packageS3Model values (Unknown bucket/key, etc.) without
// reconstructing a full tfsdk.Config.
//
// Rules:
//   - Unknown bucket/key/region are accepted (validate runs without state,
//     so references to other resources show as Unknown — that's not an error).
//   - Literal empty strings for bucket or key are rejected.
//   - expected_sha256 must match 64 lowercase hex chars when set.
func validatePackageS3(s3Config packageS3Model) diag.Diagnostics {
	var diags diag.Diagnostics

	if !s3Config.Bucket.IsNull() && !s3Config.Bucket.IsUnknown() && s3Config.Bucket.ValueString() == "" {
		diags.AddAttributeError(
			path.Root("package_s3"),
			"Invalid Configuration",
			"package_s3.bucket must not be empty.",
		)
	}
	if !s3Config.Key.IsNull() && !s3Config.Key.IsUnknown() && s3Config.Key.ValueString() == "" {
		diags.AddAttributeError(
			path.Root("package_s3"),
			"Invalid Configuration",
			"package_s3.key must not be empty.",
		)
	}
	if !s3Config.ExpectedSHA256.IsNull() && !s3Config.ExpectedSHA256.IsUnknown() {
		v := s3Config.ExpectedSHA256.ValueString()
		if !hexSHA256Re.MatchString(v) {
			diags.AddAttributeError(
				path.Root("package_s3").AtName("expected_sha256"),
				"Invalid Configuration",
				"package_s3.expected_sha256 must be 64 lowercase hexadecimal characters (the hex-encoded SHA256 of the object's content).",
			)
		}
	}
	return diags
}

// ModifyPlan computes package_sha256 at plan time from the package source.
// For S3 sources this resolves the SHA via HeadObject when possible, falling
// back to downloading the body only when neither a server-managed checksum nor
// an x-amz-meta-sha256 header is available.
func (r *softwarePackageResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip during destroy or when there's no plan (initial import).
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan softwarePackageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only compute SHA for type=package with a source configured.
	swType := plan.Type.ValueString()
	if swType != "" && swType != "package" {
		return
	}

	// Try the cheap path first: HeadObject for S3, file hash for package_path.
	sha, _, requiresDownload, diags := resolveRemoteSHA(ctx, &plan, true)

	// Local-file errors are emitted by resolveRemoteSHA as errors. During plan
	// we want them as warnings instead (the file may not exist yet on this
	// machine even though it will at apply time). S3 errors stay as errors so
	// the user sees them at plan time when they're real.
	hasLocalPath := !plan.PackagePath.IsNull() && !plan.PackagePath.IsUnknown() && plan.PackagePath.ValueString() != ""
	if diags.HasError() && hasLocalPath {
		for _, d := range diags.Errors() {
			resp.Diagnostics.AddWarning(d.Summary(), d.Detail())
		}
		// Don't append warnings on top.
		return
	}
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if requiresDownload {
		// Fall back to the body-download path. This is today's behavior for
		// S3 objects with no SHA available.
		_, downloadedSHA, err := readPackageContentForUpload(ctx, &plan)
		if err != nil {
			// S3 errors during plan can be transient (credentials not resolved
			// yet, etc.) — silently suppress, the apply will surface them.
			return
		}
		sha = downloadedSHA
	}

	if sha == "" {
		return
	}

	// Only set computed SHA if the user didn't explicitly configure package_sha256.
	var config softwarePackageResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !config.PackageSHA256.IsNull() && !config.PackageSHA256.IsUnknown() {
		// User explicitly set package_sha256 — respect their value.
		return
	}

	plan.PackageSHA256 = types.StringValue(sha)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// Configure adds the provider configured client to the resource.
func (r *softwarePackageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *softwarePackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan softwarePackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	softwareType := plan.Type.ValueString()
	if softwareType == "" {
		softwareType = "package"
	}

	switch softwareType {
	case "vpp":
		r.createVPP(ctx, &plan, resp)
	case "fleet_maintained":
		r.createFleetMaintained(ctx, &plan, resp)
	default:
		r.createPackage(ctx, &plan, resp)
	}
}

// createPackage handles creating a software package (upload).
func (r *softwarePackageResource) createPackage(ctx context.Context, plan *softwarePackageResourceModel, resp *resource.CreateResponse) {
	// Read package content from local file or S3 — Create always needs the body.
	packageContent, packageSHA256, err := readPackageContentForUpload(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error reading package", err.Error())
		return
	}
	if packageContent == nil {
		resp.Diagnostics.AddError("Missing package source", "Either package_path or package_s3 must be set for type 'package'.")
		return
	}

	// Derive filename: explicit > package_path > S3 key
	filename := deriveFilename(ctx, plan)
	if filename == "" {
		resp.Diagnostics.AddError("Missing filename", "Could not determine filename. Set 'filename' explicitly, or ensure package_path or package_s3.key is set.")
		return
	}
	// Persist the derived filename to state so it's available on subsequent plans.
	plan.Filename = types.StringValue(filename)

	// Build the upload request. NOTE: AutomaticInstall is intentionally
	// NOT set from plan.AutomaticInstall here — for the legacy resource
	// with type=package, the documented semantic of `automatic_install`
	// is the setup-experience flag (despite the misleading wire name).
	// We route that path through PUT /setup_experience/software after
	// the upload completes; setting AutomaticInstall here would create
	// a Fleet auto-install POLICY, which is a behavioral change. Users
	// who want the policy-based behavior should migrate to the new
	// fleetdm_software_custom_package resource and set
	// automatic_install_policy = true.
	uploadReq := &fleetdm.UploadSoftwarePackageRequest{
		Software:          packageContent,
		Filename:          filename,
		DisplayName:       plan.DisplayName.ValueString(),
		InstallScript:     plan.InstallScript.ValueString(),
		UninstallScript:   plan.UninstallScript.ValueString(),
		PreInstallQuery:   plan.PreInstallQuery.ValueString(),
		PostInstallScript: plan.PostInstallScript.ValueString(),
		SelfService:       plan.SelfService.ValueBool(),
	}

	// Set team_id if specified
	uploadReq.TeamID = optionalIntPtr(plan.TeamID)

	// Extract label names from lists, preserving nil/empty distinction.
	var diags = extractOptionalLabels(ctx, plan.LabelsIncludeAny, &uploadReq.LabelsIncludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractOptionalLabels(ctx, plan.LabelsExcludeAny, &uploadReq.LabelsExcludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractOptionalLabels(ctx, plan.LabelsIncludeAll, &uploadReq.LabelsIncludeAll)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractLabels(ctx, plan.Categories, &uploadReq.Categories)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Upload the software package
	title, err := r.client.UploadSoftwarePackage(ctx, uploadReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error uploading software package",
			"Could not upload software package: "+err.Error(),
		)
		return
	}

	// Update state with computed values
	plan.ID = types.Int64Value(int64(title.ID))
	plan.TitleID = types.Int64Value(int64(title.ID))
	plan.Name = types.StringValue(title.Name)
	plan.DisplayName = types.StringValue(title.DisplayName)
	plan.Version = types.StringValue("")
	if len(title.Versions) > 0 {
		plan.Version = types.StringValue(title.Versions[0].Version)
	}
	if title.SoftwarePackage != nil && title.SoftwarePackage.Platform != "" {
		plan.Platform = types.StringValue(title.SoftwarePackage.Platform)
	} else if plan.Platform.IsNull() || plan.Platform.IsUnknown() {
		// Fallback: set empty string to satisfy Computed requirement.
		// The real Fleet API always populates SoftwarePackage.Platform.
		plan.Platform = types.StringValue("")
	}
	plan.PackageSHA256 = types.StringValue(packageSHA256)
	plan.AutomaticInstallPolicies = automaticInstallPoliciesFromTitle(title)

	// Persist state BEFORE attempting the setup-experience flip so a flip
	// failure doesn't strand the just-created title outside Terraform
	// state. See software_custom_package_resource.go for the rationale.
	preFlipPlan := *plan
	if plan.AutomaticInstall.IsNull() || plan.AutomaticInstall.IsUnknown() {
		preFlipPlan.AutomaticInstall = types.BoolValue(false)
	}
	preDiags := resp.State.Set(ctx, preFlipPlan)
	resp.Diagnostics.Append(preDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Legacy semantic: type=package's `automatic_install` flag means
	// "install during setup". Route via the setup_experience endpoint.
	if plan.AutomaticInstall.ValueBool() {
		if err := r.client.SetSetupExperienceSoftwareInclude(ctx, optionalIntPtr(plan.TeamID), plan.Platform.ValueString(), title.ID); err != nil {
			resp.Diagnostics.AddError(
				"Error enabling automatic_install (setup-experience) for package",
				err.Error()+". The package was uploaded successfully and is tracked in state; re-running `terraform apply` will retry the flip.",
			)
			return
		}
	}

	// Set the state
	diags = resp.State.Set(ctx, *plan)
	resp.Diagnostics.Append(diags...)
}

// createVPP handles creating a VPP (App Store) app.
func (r *softwarePackageResource) createVPP(ctx context.Context, plan *softwarePackageResourceModel, resp *resource.CreateResponse) {
	if plan.AppStoreID.IsNull() || plan.AppStoreID.IsUnknown() || plan.AppStoreID.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"'app_store_id' is required when type is 'vpp'.",
		)
		return
	}

	teamID := 0
	if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
		teamID = int(plan.TeamID.ValueInt64())
	}

	addReq := &fleetdm.AddAppStoreAppRequest{
		AppStoreID:  plan.AppStoreID.ValueString(),
		TeamID:      teamID,
		Platform:    plan.Platform.ValueString(),
		SelfService: plan.SelfService.ValueBool(),
		DisplayName: plan.DisplayName.ValueString(),
	}

	title, err := r.client.AddAppStoreApp(ctx, addReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding VPP app",
			"Could not add App Store app: "+err.Error(),
		)
		return
	}

	plan.ID = types.Int64Value(int64(title.ID))
	plan.TitleID = types.Int64Value(int64(title.ID))
	plan.Name = types.StringValue(title.Name)
	plan.DisplayName = types.StringValue(title.DisplayName)
	plan.Version = types.StringValue("")
	if title.AppStoreApp != nil && title.AppStoreApp.LatestVersion != "" {
		plan.Version = types.StringValue(title.AppStoreApp.LatestVersion)
	} else if len(title.Versions) > 0 {
		plan.Version = types.StringValue(title.Versions[0].Version)
	}
	if title.AppStoreApp != nil && title.AppStoreApp.Platform != "" {
		plan.Platform = types.StringValue(title.AppStoreApp.Platform)
	}
	plan.PackageSHA256 = types.StringNull()
	if plan.Filename.IsNull() || plan.Filename.IsUnknown() {
		plan.Filename = types.StringNull()
	}
	plan.AutomaticInstallPolicies = automaticInstallPoliciesFromTitle(title)

	// Fleet's AddAppStoreApp endpoint doesn't accept labels. If the user
	// set any of the three label attributes in HCL, follow up with an
	// UpdateAppStoreApp call to apply them — otherwise the state would
	// permanently diverge from Fleet (Fleet returns no labels, Read's
	// non-null-state guard keeps the HCL value in state forever).
	// labels_include_all on VPP is rejected by ValidateConfig, so only
	// labels_include_any / labels_exclude_any can reach here.
	if !plan.LabelsIncludeAny.IsNull() || !plan.LabelsExcludeAny.IsNull() {
		tid := 0
		if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
			tid = int(plan.TeamID.ValueInt64())
		}
		labelReq := &fleetdm.UpdateAppStoreAppRequest{
			TeamID:      tid,
			SelfService: plan.SelfService.ValueBool(),
			DisplayName: plan.DisplayName.ValueString(),
		}
		var d diag.Diagnostics
		d = extractLabels(ctx, plan.LabelsIncludeAny, &labelReq.LabelsIncludeAny)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		d = extractLabels(ctx, plan.LabelsExcludeAny, &labelReq.LabelsExcludeAny)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.client.UpdateAppStoreApp(ctx, title.ID, labelReq); err != nil {
			resp.Diagnostics.AddError(
				"Error applying labels on VPP create",
				"The VPP app was added successfully, but the follow-up call to apply labels failed: "+err.Error()+
					". The resource is tracked in state; re-running `terraform apply` will retry.",
			)
			// Persist state so the title isn't stranded.
			_ = resp.State.Set(ctx, *plan)
			return
		}
	}

	// Persist state before the setup-experience flip; see the analogous
	// block in createPackage for the rationale.
	preFlipPlan := *plan
	if plan.AutomaticInstall.IsNull() || plan.AutomaticInstall.IsUnknown() {
		preFlipPlan.AutomaticInstall = types.BoolValue(false)
	}
	preDiags := resp.State.Set(ctx, preFlipPlan)
	resp.Diagnostics.Append(preDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Legacy semantic for VPP: automatic_install means install_during_setup.
	if plan.AutomaticInstall.ValueBool() {
		if err := r.client.SetSetupExperienceSoftwareInclude(ctx, optionalIntPtr(plan.TeamID), plan.Platform.ValueString(), title.ID); err != nil {
			resp.Diagnostics.AddError(
				"Error enabling automatic_install (setup-experience) for VPP",
				err.Error()+". The VPP app was created successfully and is tracked in state; re-running `terraform apply` will retry the flip.",
			)
			return
		}
	}

	// Set the state
	diags := resp.State.Set(ctx, *plan)
	resp.Diagnostics.Append(diags...)
}

// createFleetMaintained handles creating a Fleet Maintained App.
func (r *softwarePackageResource) createFleetMaintained(ctx context.Context, plan *softwarePackageResourceModel, resp *resource.CreateResponse) {
	if plan.FleetMaintainedAppID.IsNull() || plan.FleetMaintainedAppID.IsUnknown() {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"'fleet_maintained_app_id' is required when type is 'fleet_maintained'.",
		)
		return
	}

	teamID := 0
	if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
		teamID = int(plan.TeamID.ValueInt64())
	}

	// For legacy fleet_maintained: automatic_install was always policy-based
	// here (Fleet's AddFMA endpoint accepts the automatic_install JSON field
	// and creates a policy). This branch keeps that behavior — it's been
	// working for FMA users. The package and VPP branches above route
	// automatic_install through setup_experience instead.
	addReq := &fleetdm.AddFleetMaintainedAppRequest{
		FleetMaintainedAppID: int(plan.FleetMaintainedAppID.ValueInt64()),
		TeamID:               teamID,
		InstallScript:        plan.InstallScript.ValueString(),
		UninstallScript:      plan.UninstallScript.ValueString(),
		PreInstallQuery:      plan.PreInstallQuery.ValueString(),
		PostInstallScript:    plan.PostInstallScript.ValueString(),
		SelfService:          plan.SelfService.ValueBool(),
		AutomaticInstall:     plan.AutomaticInstall.ValueBool(),
	}

	var diags = extractLabels(ctx, plan.LabelsIncludeAny, &addReq.LabelsIncludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractLabels(ctx, plan.LabelsExcludeAny, &addReq.LabelsExcludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractLabels(ctx, plan.LabelsIncludeAll, &addReq.LabelsIncludeAll)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	title, err := r.client.AddFleetMaintainedApp(ctx, addReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding Fleet Maintained App",
			"Could not add Fleet Maintained App: "+err.Error(),
		)
		return
	}

	plan.ID = types.Int64Value(int64(title.ID))
	plan.TitleID = types.Int64Value(int64(title.ID))
	plan.Name = types.StringValue(title.Name)
	plan.DisplayName = types.StringValue(title.DisplayName)
	plan.Version = types.StringValue("")
	if len(title.Versions) > 0 {
		plan.Version = types.StringValue(title.Versions[0].Version)
	}
	if title.SoftwarePackage != nil && title.SoftwarePackage.Platform != "" {
		plan.Platform = types.StringValue(title.SoftwarePackage.Platform)
	}
	plan.PackageSHA256 = types.StringNull()
	if plan.Filename.IsNull() || plan.Filename.IsUnknown() {
		plan.Filename = types.StringNull()
	}
	plan.AutomaticInstallPolicies = automaticInstallPoliciesFromTitle(title)

	diags = resp.State.Set(ctx, *plan)
	resp.Diagnostics.Append(diags...)
}

// extractLabels extracts string labels from a types.List into a []string target.
func extractLabels(ctx context.Context, list types.List, target *[]string) diag.Diagnostics {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var labels []string
	diags := list.ElementsAs(ctx, &labels, false)
	if !diags.HasError() {
		*target = labels
	}
	return diags
}

// extractOptionalLabels mirrors extractLabels but preserves the nil-vs-empty
// distinction needed for Fleet's software label endpoints. When the HCL
// attribute is null/unknown, *target is left nil ("no change"). When the
// HCL attribute is an empty list, *target points to an empty slice ("clear
// all labels"). When populated, *target points to the list of names. See
// PatchSoftwarePackageRequest / UploadSoftwarePackageRequest doc comments
// in internal/fleetdm/software.go for the wire-level translation.
func extractOptionalLabels(ctx context.Context, list types.List, target **[]string) diag.Diagnostics {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	labels := []string{}
	diags := list.ElementsAs(ctx, &labels, false)
	if diags.HasError() {
		return diags
	}
	*target = &labels
	return nil
}

// labelsToStringListValue converts a SoftwareLabel slice from Fleet's API
// response into a types.List of label-name strings for state. A nil
// response slice maps to a null list — Fleet's JSON cannot distinguish
// "field absent" from "field present and empty" once Go-decoded, so
// "absent in response" is interpreted as "preserve prior state intent",
// which is consistent with how other Optional fields in this resource
// (e.g. Platform) handle empty responses.
func labelsToStringListValue(labels []fleetdm.SoftwareLabel) types.List {
	if labels == nil {
		return types.ListNull(types.StringType)
	}
	values := make([]attr.Value, 0, len(labels))
	for _, l := range labels {
		values = append(values, types.StringValue(l.Name))
	}
	return types.ListValueMust(types.StringType, values)
}

// Read refreshes the Terraform state with the latest data.
func (r *softwarePackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state softwarePackageResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	teamID := optionalIntPtr(state.TeamID)

	// Get the software title
	title, err := r.client.GetSoftwareTitle(ctx, titleID, teamID)
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading software package",
			"Could not read software package: "+err.Error(),
		)
		return
	}

	if title == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Determine the type from the API response
	detectedType := detectSoftwareType(title)

	// If type is set in state, verify consistency; otherwise set it
	if !state.Type.IsNull() && !state.Type.IsUnknown() && state.Type.ValueString() != "" {
		// Keep existing type from state
	} else {
		state.Type = types.StringValue(detectedType)
	}

	switch detectedType {
	case "vpp":
		if title.AppStoreApp == nil {
			resp.State.RemoveResource(ctx)
			return
		}
		r.readVPP(ctx, title, &state)
	case "package", "fleet_maintained":
		if title.SoftwarePackage == nil {
			resp.State.RemoveResource(ctx)
			return
		}
		r.readPackageOrFMA(ctx, title, &state)
	default:
		// Neither software_package nor app_store_app
		resp.State.RemoveResource(ctx)
		return
	}

	// Set the state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// detectSoftwareType determines the type of software from an API response.
func detectSoftwareType(title *fleetdm.SoftwareTitle) string {
	if title.AppStoreApp != nil {
		return "vpp"
	}
	if title.SoftwarePackage != nil {
		// Fleet Maintained Apps also show as software_package in the response.
		// We rely on the state type field to distinguish them.
		return "package"
	}
	return "package"
}

// readVPP populates state from a VPP app title.
func (r *softwarePackageResource) readVPP(_ context.Context, title *fleetdm.SoftwareTitle, state *softwarePackageResourceModel) {
	state.Name = types.StringValue(title.Name)
	state.DisplayName = types.StringValue(title.DisplayName)
	app := title.AppStoreApp
	if app.LatestVersion != "" {
		state.Version = types.StringValue(app.LatestVersion)
	} else if len(title.Versions) > 0 {
		state.Version = types.StringValue(title.Versions[0].Version)
	}
	if app.Platform != "" {
		state.Platform = types.StringValue(app.Platform)
	}
	// If app.Platform is empty, leave state.Platform unchanged (UseStateForUnknown handles this).
	state.AppStoreID = types.StringValue(app.AdamID)
	state.SelfService = types.BoolValue(app.SelfService)
	state.AutomaticInstallPolicies = automaticInstallPoliciesFromTitle(title)
	if app.LabelsIncludeAll != nil && !state.LabelsIncludeAll.IsNull() {
		state.LabelsIncludeAll = labelsToStringListValue(app.LabelsIncludeAll)
	}
	// Refresh labels only when Fleet returned a concrete value AND the
	// prior state already tracked the attribute. Writing into an
	// Optional-not-Computed list whose prior state is null would: (a)
	// trip the framework's "inconsistent result after apply" guard when
	// the plan said null, and (b) create a perpetual diff loop because
	// extractOptionalLabels translates HCL-null to "PATCH omits the
	// field" — Fleet keeps the labels, Read pulls them back in, plan
	// shows a diff that the next apply can't actually resolve.
	if app.LabelsIncludeAny != nil && !state.LabelsIncludeAny.IsNull() {
		state.LabelsIncludeAny = labelsToStringListValue(app.LabelsIncludeAny)
	}
	if app.LabelsExcludeAny != nil && !state.LabelsExcludeAny.IsNull() {
		state.LabelsExcludeAny = labelsToStringListValue(app.LabelsExcludeAny)
	}
}

// readPackageOrFMA populates state from a software package or Fleet Maintained App title.
//
// For the legacy `automatic_install` attribute we have to pick which Fleet
// signal to mirror — the answer is type-dependent:
//   - type=package: legacy semantic is "install during setup". Mirror
//     pkg.InstallDuringSetup (the value Fleet returns from the title's
//     setup-experience flag).
//   - type=fleet_maintained: legacy semantic is "policy-based auto-install".
//     Mirror presence of an automatic_install policy.
//
// The Read function gets called for both types via this helper, so the
// caller (Read) sets state.Type before invoking us. Here we branch on the
// already-populated state.Type.
func (r *softwarePackageResource) readPackageOrFMA(_ context.Context, title *fleetdm.SoftwareTitle, state *softwarePackageResourceModel) {
	state.Name = types.StringValue(title.Name)
	state.DisplayName = types.StringValue(title.DisplayName)
	if len(title.Versions) > 0 {
		state.Version = types.StringValue(title.Versions[0].Version)
	}

	pkg := title.SoftwarePackage
	if pkg.Platform != "" {
		state.Platform = types.StringValue(pkg.Platform)
	}
	// Don't fall back to title.Source — it contains package source types
	// ("pkg_packages", "programs"), not OS platforms ("darwin", "windows").
	if pkg.InstallScript != "" {
		state.InstallScript = types.StringValue(pkg.InstallScript)
	}
	if pkg.UninstallScript != "" {
		state.UninstallScript = types.StringValue(pkg.UninstallScript)
	}
	if pkg.PreInstallQuery != "" {
		state.PreInstallQuery = types.StringValue(pkg.PreInstallQuery)
	}
	if pkg.PostInstallScript != "" {
		state.PostInstallScript = types.StringValue(pkg.PostInstallScript)
	}
	state.SelfService = types.BoolValue(pkg.SelfService)
	switch state.Type.ValueString() {
	case "fleet_maintained":
		// Policy-based auto-install is the legacy FMA semantic.
		state.AutomaticInstall = types.BoolValue(len(pkg.AutomaticInstallPolicies) > 0)
	default:
		// type=package — setup-experience semantic.
		if pkg.InstallDuringSetup != nil {
			state.AutomaticInstall = types.BoolValue(*pkg.InstallDuringSetup)
		}
	}
	state.AutomaticInstallPolicies = automaticInstallPoliciesFromTitle(title)
	if pkg.HashSHA256 != "" {
		state.PackageSHA256 = types.StringValue(pkg.HashSHA256)
	}
	// Refresh labels only when Fleet returned a concrete value AND the
	// prior state already tracked the attribute (same convention and
	// rationale as readVPP — see comment there).
	if pkg.LabelsIncludeAny != nil && !state.LabelsIncludeAny.IsNull() {
		state.LabelsIncludeAny = labelsToStringListValue(pkg.LabelsIncludeAny)
	}
	if pkg.LabelsExcludeAny != nil && !state.LabelsExcludeAny.IsNull() {
		state.LabelsExcludeAny = labelsToStringListValue(pkg.LabelsExcludeAny)
	}
	if pkg.LabelsIncludeAll != nil && !state.LabelsIncludeAll.IsNull() {
		state.LabelsIncludeAll = labelsToStringListValue(pkg.LabelsIncludeAll)
	}
	if pkg.Categories != nil && !state.Categories.IsNull() {
		state.Categories = stringSliceToStringList(pkg.Categories)
	}
}

// Update updates the resource and sets the updated Terraform state.
func (r *softwarePackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan softwarePackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state softwarePackageResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	softwareType := plan.Type.ValueString()
	if softwareType == "" {
		softwareType = "package"
	}

	titleID := int(plan.TitleID.ValueInt64())
	teamID := optionalIntPtr(plan.TeamID)

	switch softwareType {
	case "vpp":
		r.updateVPP(ctx, titleID, teamID, &plan, &state, resp)
	default:
		// Both "package" and "fleet_maintained" use PatchSoftwarePackage
		r.updatePackageOrFMA(ctx, titleID, teamID, &plan, &state, resp)
	}

	// Carry over Computed attributes that the type-specific update paths
	// don't refresh — the next Read will overwrite with Fleet's current
	// values. Without this, the framework treats them as Unknown after
	// apply and errors with "Provider returned invalid result object".
	if plan.AutomaticInstallPolicies.IsUnknown() {
		plan.AutomaticInstallPolicies = state.AutomaticInstallPolicies
	}
	if plan.DisplayName.IsUnknown() {
		plan.DisplayName = state.DisplayName
	}

	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// updateVPP handles updating a VPP (App Store) app.
func (r *softwarePackageResource) updateVPP(ctx context.Context, titleID int, teamID *int, plan *softwarePackageResourceModel, priorState *softwarePackageResourceModel, resp *resource.UpdateResponse) {
	tid := 0
	if teamID != nil {
		tid = *teamID
	}

	updateReq := &fleetdm.UpdateAppStoreAppRequest{
		TeamID:      tid,
		SelfService: plan.SelfService.ValueBool(),
		DisplayName: plan.DisplayName.ValueString(),
	}

	// UpdateAppStoreAppRequest is JSON-encoded with no `omitempty` on the
	// label fields, so a nil slice serializes as `null` (Fleet treats as
	// "no change") and an empty slice as `[]` (Fleet treats as "clear").
	// This matches the convention documented on UpdatePolicyRequest in
	// policies.go. Pre-initializing to []string{} would force both fields
	// to serialize as `[]`, violating Fleet's "only one of …" invariant.
	var diags diag.Diagnostics
	diags = extractLabels(ctx, plan.LabelsIncludeAny, &updateReq.LabelsIncludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractLabels(ctx, plan.LabelsExcludeAny, &updateReq.LabelsExcludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractLabels(ctx, plan.LabelsIncludeAll, &updateReq.LabelsIncludeAll)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateAppStoreApp(ctx, titleID, updateReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating VPP app",
			"Could not update App Store app: "+err.Error(),
		)
		return
	}

	// Legacy VPP `automatic_install` means setup-experience flag. Route
	// the diff through the setup_experience endpoint — only when it
	// actually changed (else every Update would emit a redundant PUT).
	if !plan.AutomaticInstall.Equal(priorState.AutomaticInstall) {
		if plan.AutomaticInstall.ValueBool() {
			if err := r.client.SetSetupExperienceSoftwareInclude(ctx, teamID, plan.Platform.ValueString(), titleID); err != nil {
				resp.Diagnostics.AddError("Error enabling automatic_install (setup-experience) for VPP", err.Error())
			}
		} else {
			if err := r.client.SetSetupExperienceSoftwareExclude(ctx, teamID, plan.Platform.ValueString(), titleID); err != nil {
				resp.Diagnostics.AddError("Error disabling automatic_install (setup-experience) for VPP", err.Error())
			}
		}
	}
}

// updatePackageOrFMA handles updating a software package or Fleet Maintained App.
//
// Fast path: resolve the remote SHA cheaply (HeadObject for S3, file hash for
// package_path). If it matches the SHA Fleet currently has stored (mirrored in
// priorState.PackageSHA256), we know the binary is unchanged — skip the
// download + delete + re-upload entirely and just PATCH metadata.
//
// Slow path: only when the cheap path is unavailable (S3 object has no SHA256)
// or the SHA actually differs, we download the body, delete the old package,
// and re-upload.
func (r *softwarePackageResource) updatePackageOrFMA(ctx context.Context, titleID int, teamID *int, plan *softwarePackageResourceModel, priorState *softwarePackageResourceModel, resp *resource.UpdateResponse) {
	// Plan-time guard: for type=fleet_maintained, automatic_install is a
	// Create-time-only flag (Fleet's AddFMA endpoint creates the policy at
	// title creation; the PATCH endpoint doesn't accept it). Fail BEFORE
	// any wire operation — otherwise a partial PATCH would leave Fleet
	// updated but the resource error'd-out, with the user unable to see
	// what already applied.
	if plan.Type.ValueString() == "fleet_maintained" && !plan.AutomaticInstall.Equal(priorState.AutomaticInstall) {
		resp.Diagnostics.AddError(
			"automatic_install cannot be changed for type=fleet_maintained",
			"Fleet only honors automatic_install at creation time for Fleet Maintained Apps. "+
				"Recreate the resource (terraform taint + apply) or migrate to fleetdm_software_fleet_maintained_app "+
				"and use automatic_install_policy (ForceNew).",
		)
		return
	}

	hasPath := !plan.PackagePath.IsNull() && !plan.PackagePath.IsUnknown() && plan.PackagePath.ValueString() != ""
	hasS3 := !plan.PackageS3.IsNull() && !plan.PackageS3.IsUnknown()
	hasSource := hasPath || hasS3

	if hasSource {
		// Try the cheap path first.
		sha, _, requiresDownload, diags := resolveRemoteSHA(ctx, plan, true)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		currentSHA := priorState.PackageSHA256.ValueString()

		// Determine whether the binary actually needs to change.
		needsUpload := false
		var preFetched []byte
		var resolvedSHA string

		if !requiresDownload {
			resolvedSHA = sha
			needsUpload = sha != currentSHA || currentSHA == ""
		} else {
			// Cheap path unavailable — fall back to downloading and hashing.
			content, localSHA, err := readPackageContentForUpload(ctx, plan)
			if err != nil {
				resp.Diagnostics.AddError("Error reading package", err.Error())
				return
			}
			resolvedSHA = localSHA
			preFetched = content
			needsUpload = localSHA != currentSHA
		}

		if !needsUpload {
			// Fleet already has this exact binary. Skip the heavy work and
			// fall through to the metadata PATCH at the bottom.
			plan.PackageSHA256 = types.StringValue(resolvedSHA)
		} else {
			// Need to re-upload. If we don't have the content yet (cheap path
			// proved a difference), download it now.
			if preFetched == nil {
				content, localSHA, err := readPackageContentForUpload(ctx, plan)
				if err != nil {
					resp.Diagnostics.AddError("Error reading package", err.Error())
					return
				}
				preFetched = content
				resolvedSHA = localSHA
			}
			if !r.replaceSoftwarePackage(ctx, titleID, teamID, plan, preFetched, resolvedSHA, resp) {
				return
			}
		}
	}

	// Update metadata only (scripts, labels, self-service, etc.). Label
	// pointers stay nil unless the HCL attribute is set — that's how we
	// avoid sending both labels_include_any and labels_exclude_any in the
	// same multipart body, which Fleet rejects ("Only one of …").
	patchReq := &fleetdm.PatchSoftwarePackageRequest{
		TeamID:            teamID,
		InstallScript:     plan.InstallScript.ValueString(),
		UninstallScript:   plan.UninstallScript.ValueString(),
		PreInstallQuery:   plan.PreInstallQuery.ValueString(),
		PostInstallScript: plan.PostInstallScript.ValueString(),
		SelfService:       plan.SelfService.ValueBool(),
		DisplayName:       plan.DisplayName.ValueString(),
	}

	var diags diag.Diagnostics
	diags = extractOptionalLabels(ctx, plan.LabelsIncludeAny, &patchReq.LabelsIncludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractOptionalLabels(ctx, plan.LabelsExcludeAny, &patchReq.LabelsExcludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractOptionalLabels(ctx, plan.LabelsIncludeAll, &patchReq.LabelsIncludeAll)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractOptionalLabels(ctx, plan.Categories, &patchReq.Categories)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.PatchSoftwarePackage(ctx, titleID, patchReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating software package",
			"Could not update software package metadata: "+err.Error(),
		)
		return
	}

	// For type=package, route automatic_install via setup-experience —
	// only when the value actually changed. The type=fleet_maintained
	// case is rejected at the top of this function, before any wire
	// operation, so it never reaches here.
	if plan.Type.ValueString() == "package" && !plan.AutomaticInstall.Equal(priorState.AutomaticInstall) {
		if plan.AutomaticInstall.ValueBool() {
			if err := r.client.SetSetupExperienceSoftwareInclude(ctx, teamID, plan.Platform.ValueString(), titleID); err != nil {
				resp.Diagnostics.AddError("Error enabling automatic_install (setup-experience) for package", err.Error())
			}
		} else {
			if err := r.client.SetSetupExperienceSoftwareExclude(ctx, teamID, plan.Platform.ValueString(), titleID); err != nil {
				resp.Diagnostics.AddError("Error disabling automatic_install (setup-experience) for package", err.Error())
			}
		}
	}
}

// replaceSoftwarePackage deletes the existing software package in Fleet and
// uploads the provided content as a replacement. On success it mutates `plan`
// with the new title metadata. Returns true on success, false on failure (in
// which case it has also appended diagnostics to resp).
//
// Fleet refuses to delete a software title that any policy references via
// install_software automation (HTTP 409: "Couldn't delete. Policy automation
// uses this software."). Before issuing the delete, the function scans for
// such policies and clears their software_title_id; after the re-upload
// succeeds, it re-points them at the new title id returned by Fleet (which
// may or may not equal the previous id, depending on whether Fleet matches
// by bundle id).
func (r *softwarePackageResource) replaceSoftwarePackage(ctx context.Context, titleID int, teamID *int, plan *softwarePackageResourceModel, content []byte, sha string, resp *resource.UpdateResponse) bool {
	// Step 1: detach any install_software / patch_software automation pointing at this title.
	attachedInstall, attachedPatch, err := listPoliciesBlockingTitleDelete(ctx, r.client, titleID, teamID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error replacing software package",
			"Could not list policies before re-upload (needed to clear policy automation): "+err.Error(),
		)
		return false
	}
	for _, p := range attachedInstall {
		if err := r.client.SetPolicyInstallSoftwareTitleID(ctx, p.ID, teamID, nil); err != nil {
			resp.Diagnostics.AddError(
				"Error replacing software package",
				fmt.Sprintf("Could not detach install_software automation from policy %d (%q) before re-upload: %s", p.ID, p.Name, err.Error()),
			)
			return false
		}
	}
	for _, p := range attachedPatch {
		if err := r.client.SetPolicyPatchSoftwareTitleID(ctx, p.ID, teamID, nil); err != nil {
			resp.Diagnostics.AddError(
				"Error replacing software package",
				fmt.Sprintf("Could not detach patch_software automation from policy %d (%q) before re-upload: %s", p.ID, p.Name, err.Error()),
			)
			return false
		}
	}

	// Step 2: delete the existing package.
	if err := r.client.DeleteSoftwarePackage(ctx, titleID, teamID); err != nil {
		if !isNotFound(err) {
			resp.Diagnostics.AddError(
				"Error replacing software package",
				"Could not delete existing package before re-upload: "+err.Error(),
			)
			return false
		}
	}

	filename := deriveFilename(ctx, plan)
	if filename == "" {
		resp.Diagnostics.AddError("Missing filename", "Could not determine filename for re-upload. Set 'filename' explicitly.")
		return false
	}
	plan.Filename = types.StringValue(filename)

	// Legacy `automatic_install` for type=package now means setup-experience
	// (see createPackage). Don't forward to the Upload request's
	// AutomaticInstall (which would create a Fleet policy); the
	// setup_experience reconciliation happens in updatePackageOrFMA's
	// post-PATCH block.
	uploadReq := &fleetdm.UploadSoftwarePackageRequest{
		TeamID:            teamID,
		Software:          content,
		Filename:          filename,
		DisplayName:       plan.DisplayName.ValueString(),
		InstallScript:     plan.InstallScript.ValueString(),
		UninstallScript:   plan.UninstallScript.ValueString(),
		PreInstallQuery:   plan.PreInstallQuery.ValueString(),
		PostInstallScript: plan.PostInstallScript.ValueString(),
		SelfService:       plan.SelfService.ValueBool(),
	}

	uploadDiags := extractOptionalLabels(ctx, plan.LabelsIncludeAny, &uploadReq.LabelsIncludeAny)
	resp.Diagnostics.Append(uploadDiags...)
	if resp.Diagnostics.HasError() {
		return false
	}
	uploadDiags = extractOptionalLabels(ctx, plan.LabelsExcludeAny, &uploadReq.LabelsExcludeAny)
	resp.Diagnostics.Append(uploadDiags...)
	if resp.Diagnostics.HasError() {
		return false
	}
	uploadDiags = extractOptionalLabels(ctx, plan.LabelsIncludeAll, &uploadReq.LabelsIncludeAll)
	resp.Diagnostics.Append(uploadDiags...)
	if resp.Diagnostics.HasError() {
		return false
	}
	uploadDiags = extractLabels(ctx, plan.Categories, &uploadReq.Categories)
	resp.Diagnostics.Append(uploadDiags...)
	if resp.Diagnostics.HasError() {
		return false
	}

	// Step 3: upload the new package.
	title, err := r.client.UploadSoftwarePackage(ctx, uploadReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error re-uploading software package",
			"The existing package was deleted but the re-upload failed. "+
				"The resource has been removed from state; run 'terraform apply' again to recreate it. "+
				"Error: "+err.Error(),
		)
		resp.State.RemoveResource(ctx)
		return false
	}

	plan.ID = types.Int64Value(int64(title.ID))
	plan.TitleID = types.Int64Value(int64(title.ID))
	plan.Name = types.StringValue(title.Name)
	if len(title.Versions) > 0 {
		plan.Version = types.StringValue(title.Versions[0].Version)
	}
	if title.SoftwarePackage != nil && title.SoftwarePackage.Platform != "" {
		plan.Platform = types.StringValue(title.SoftwarePackage.Platform)
	}
	plan.PackageSHA256 = types.StringValue(sha)

	// Step 4: reattach install_software / patch_software automation to the new title id.
	newTitleID := title.ID
	var reattachFailed []string
	for _, p := range attachedInstall {
		if err := r.client.SetPolicyInstallSoftwareTitleID(ctx, p.ID, teamID, &newTitleID); err != nil {
			reattachFailed = append(reattachFailed, fmt.Sprintf("install_software policy %d (%q): %s", p.ID, p.Name, err.Error()))
		}
	}
	for _, p := range attachedPatch {
		if err := r.client.SetPolicyPatchSoftwareTitleID(ctx, p.ID, teamID, &newTitleID); err != nil {
			reattachFailed = append(reattachFailed, fmt.Sprintf("patch_software policy %d (%q): %s", p.ID, p.Name, err.Error()))
		}
	}
	if len(reattachFailed) > 0 {
		// Persist the new title metadata before bailing — the package itself
		// was replaced successfully, only the reattach step failed. Saving
		// state here means the next 'terraform apply' will see no drift on
		// the package itself and let the affected fleetdm_policy resources
		// self-heal via their normal drift-detection path.
		stateDiags := resp.State.Set(ctx, *plan)
		resp.Diagnostics.Append(stateDiags...)

		resp.Diagnostics.AddError(
			"Error re-attaching policy automation after package replace",
			"The software package was replaced successfully (new title_id="+strconv.Itoa(newTitleID)+"), but re-attaching policy automation to the following policies failed:\n  - "+
				strings.Join(reattachFailed, "\n  - ")+
				"\n\nThe affected fleetdm_policy resources will show drift on `software_title_id` / `patch_software_title_id` on the next plan; re-running 'terraform apply' should heal them automatically.",
		)
		return false
	}

	return true
}

// Adapter methods so the legacy model implements packageSource (defined
// in software_common_schema.go) for the binary-source helpers below.
func (m *softwarePackageResourceModel) PackagePathField() types.String { return m.PackagePath }
func (m *softwarePackageResourceModel) PackageS3Field() types.Object   { return m.PackageS3 }
func (m *softwarePackageResourceModel) FilenameField() types.String    { return m.Filename }

// buildS3Source parses the package_s3 nested object into an fleetdm.S3Source and
// the original model. It enforces bucket/key being known + non-empty.
func buildS3Source(ctx context.Context, model packageSource) (fleetdm.S3Source, packageS3Model, error) {
	var s3Config packageS3Model
	diags := model.PackageS3Field().As(ctx, &s3Config, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		var details string
		for _, d := range diags.Errors() {
			details += d.Summary() + ": " + d.Detail() + "; "
		}
		return fleetdm.S3Source{}, s3Config, fmt.Errorf("could not parse package_s3 configuration: %s", details)
	}

	if s3Config.Bucket.IsUnknown() || s3Config.Key.IsUnknown() {
		// Soft-signal: caller decides whether to skip (plan time, expected)
		// or surface (apply time, defensive — should not happen because
		// Terraform resolves dependent values before Create/Update is
		// invoked).
		return fleetdm.S3Source{}, s3Config, errS3SourceUnknown
	}
	if s3Config.Bucket.ValueString() == "" {
		return fleetdm.S3Source{}, s3Config, fmt.Errorf("package_s3.bucket must not be empty")
	}
	if s3Config.Key.ValueString() == "" {
		return fleetdm.S3Source{}, s3Config, fmt.Errorf("package_s3.key must not be empty")
	}

	src := fleetdm.S3Source{
		Bucket: s3Config.Bucket.ValueString(),
		Key:    s3Config.Key.ValueString(),
	}
	if !s3Config.Region.IsNull() && !s3Config.Region.IsUnknown() {
		src.Region = s3Config.Region.ValueString()
	}
	if !s3Config.EndpointURL.IsNull() && !s3Config.EndpointURL.IsUnknown() {
		src.EndpointURL = s3Config.EndpointURL.ValueString()
	}
	return src, s3Config, nil
}

// resolveRemoteSHA returns the SHA256 of the configured package source without
// downloading the body from S3. It is used by ModifyPlan and Update to decide
// whether the installer in S3 differs from what Fleet already has stored.
//
// Returns:
//   - sha: lowercase hex SHA256 of the package source (empty when no source is
//     configured, or when requiresDownload is true).
//   - source: human-readable label describing where the SHA came from
//     ("local-file", "expected_sha256", "s3-checksum", "object-metadata").
//   - requiresDownload: true when we could not get the SHA cheaply and the
//     caller must fall back to downloading the body. Currently only happens
//     for the S3 source when no checksum or metadata SHA is available.
//   - diags: warnings (e.g. "falling back to download") and errors.
//
// allowExpected gates use of package_s3.expected_sha256. Set it true for
// ModifyPlan / Update (where trusting the user is the whole point); false has
// no current callers but is reserved.
func resolveRemoteSHA(ctx context.Context, model packageSource, allowExpected bool) (string, string, bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	path := model.PackagePathField()
	pkgS3 := model.PackageS3Field()
	hasPath := !path.IsNull() && !path.IsUnknown() && path.ValueString() != ""
	s3Present := !pkgS3.IsNull() && !pkgS3.IsUnknown()

	if hasPath && s3Present {
		diags.AddError("Conflicting Configuration", "package_path and package_s3 are mutually exclusive; set one or the other.")
		return "", "", false, diags
	}

	switch {
	case hasPath:
		// Hashing a local file is cheap; just do it. We don't keep the bytes
		// here — the caller calls readPackageContentForUpload when it actually
		// needs to upload.
		content, err := os.ReadFile(path.ValueString()) // #nosec G304 -- path comes from Terraform config
		if err != nil {
			diags.AddError("Unable to read package file", fmt.Sprintf("Could not read %s: %s", path.ValueString(), err.Error()))
			return "", "", false, diags
		}
		sum := sha256.Sum256(content)
		return hex.EncodeToString(sum[:]), "local-file", false, diags

	case s3Present:
		// buildS3Source returns the parsed packageS3Model even when bucket/key
		// are Unknown, so we can read expected_sha256 off it whether or not
		// the S3 source itself is resolvable. That lets users who pinned the
		// SHA out-of-band still hit the fast path when their bucket/key
		// reference only resolves at apply time.
		src, s3Cfg, err := buildS3Source(ctx, model)

		if allowExpected && !s3Cfg.ExpectedSHA256.IsNull() && !s3Cfg.ExpectedSHA256.IsUnknown() && s3Cfg.ExpectedSHA256.ValueString() != "" {
			return s3Cfg.ExpectedSHA256.ValueString(), "expected_sha256", false, diags
		}

		if err != nil {
			if errors.Is(err, errS3SourceUnknown) {
				// Bucket and/or key not yet known. Defer the SHA computation
				// to apply time; the plan will show package_sha256 as
				// (known after apply).
				return "", "", false, diags
			}
			diags.AddError("Invalid package_s3", err.Error())
			return "", "", false, diags
		}

		sha, source, err := fetchS3SHA256(ctx, src)
		switch {
		case err == nil:
			return sha, source, false, diags
		case errors.Is(err, fleetdm.ErrUnsupportedChecksum):
			diags.AddError("Unsupported S3 checksum", err.Error())
			return "", "", false, diags
		case errors.Is(err, fleetdm.ErrNoSHA256Available):
			diags.AddWarning(
				"S3 object has no SHA256 — falling back to download",
				fmt.Sprintf(
					"%s. The provider will download the object on every plan/apply to compute the SHA locally. "+
						"To skip downloads on unchanged installers, see the 'SHA256 verification with S3 sources' section of the fleetdm_software_package documentation.",
					err.Error(),
				),
			)
			return "", "", true, diags
		default:
			diags.AddError("Error resolving S3 SHA256", err.Error())
			return "", "", false, diags
		}

	default:
		// No source configured — nothing to resolve, not an error.
		return "", "", false, diags
	}
}

// readPackageContentForUpload reads the full package content from package_path
// or package_s3 (downloading from S3 when needed) and returns the content along
// with its lowercase hex SHA256. Used by Create and by the slow path of Update.
func readPackageContentForUpload(ctx context.Context, model packageSource) ([]byte, string, error) {
	path := model.PackagePathField()
	pkgS3 := model.PackageS3Field()
	hasPath := !path.IsNull() && !path.IsUnknown() && path.ValueString() != ""
	s3Present := !pkgS3.IsNull() && !pkgS3.IsUnknown()

	if hasPath && s3Present {
		return nil, "", fmt.Errorf("package_path and package_s3 are mutually exclusive; set one or the other")
	}

	var content []byte
	var err error

	switch {
	case hasPath:
		content, err = os.ReadFile(path.ValueString()) // #nosec G304 -- path comes from Terraform config
		if err != nil {
			return nil, "", fmt.Errorf("could not read package at %s: %w", path.ValueString(), err)
		}
	case s3Present:
		src, _, err := buildS3Source(ctx, model)
		if err != nil {
			if errors.Is(err, errS3SourceUnknown) {
				// Defensive: Terraform should always resolve dependent values
				// before invoking Create/Update. If we land here, something
				// has gone wrong in the graph evaluation upstream — give the
				// user something actionable rather than the raw sentinel.
				return nil, "", fmt.Errorf(
					"package_s3.bucket or package_s3.key did not resolve to a known string by apply time; " +
						"this usually means the resource providing the value failed to apply, or a dependency was declared incorrectly — " +
						"verify the referenced resource exists and check `terraform plan` output for unresolved (known after apply) markers")
			}
			return nil, "", err
		}
		content, err = fleetdm.DownloadS3Object(ctx, src)
		if err != nil {
			return nil, "", err
		}
	default:
		return nil, "", nil // no source specified, that's OK for tracked-only packages
	}

	hash := sha256.Sum256(content)
	return content, hex.EncodeToString(hash[:]), nil
}

// deriveFilename returns the filename to use for the package upload.
// Priority: explicit filename attribute > package_path basename > package_s3 key basename.
func deriveFilename(ctx context.Context, model packageSource) string {
	filename := model.FilenameField()
	if !filename.IsNull() && !filename.IsUnknown() && filename.ValueString() != "" {
		return filename.ValueString()
	}
	path := model.PackagePathField()
	if !path.IsNull() && !path.IsUnknown() && path.ValueString() != "" {
		return filepath.Base(path.ValueString())
	}
	pkgS3 := model.PackageS3Field()
	if !pkgS3.IsNull() && !pkgS3.IsUnknown() {
		var s3Cfg packageS3Model
		if d := pkgS3.As(ctx, &s3Cfg, basetypes.ObjectAsOptions{}); !d.HasError() {
			if !s3Cfg.Key.IsNull() && !s3Cfg.Key.IsUnknown() && s3Cfg.Key.ValueString() != "" {
				return gopath.Base(s3Cfg.Key.ValueString())
			}
		}
	}
	return ""
}

// Delete deletes the resource and removes the Terraform state.
func (r *softwarePackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state softwarePackageResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	teamID := optionalIntPtr(state.TeamID)

	if diags := detachPoliciesBeforeTitleDelete(ctx, r.client, titleID, teamID); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Delete the software package
	err := r.client.DeleteSoftwarePackage(ctx, titleID, teamID)
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting software package",
			"Could not delete software package: "+err.Error(),
		)
		return
	}
}

// ImportState imports an existing resource by ID.
// Import format: title_id or title_id:team_id
func (r *softwarePackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) < 1 || len(parts) > 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in format: title_id or title_id:team_id",
		)
		return
	}

	titleID, err := strconv.Atoi(parts[0])
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid title ID",
			fmt.Sprintf("Could not parse title ID '%s': %s", parts[0], err.Error()),
		)
		return
	}

	var teamID *int
	if len(parts) == 2 {
		tid, err := strconv.Atoi(parts[1])
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid team ID",
				fmt.Sprintf("Could not parse team ID '%s': %s", parts[1], err.Error()),
			)
			return
		}
		teamID = &tid
	}

	// Fetch the software title which contains installer metadata
	title, err := r.client.GetSoftwareTitle(ctx, titleID, teamID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading software package during import",
			fmt.Sprintf("Could not read software title for ID %d: %s", titleID, err.Error()),
		)
		return
	}

	if title == nil {
		resp.Diagnostics.AddError(
			"Error reading software package during import",
			fmt.Sprintf("Software title %d not found", titleID),
		)
		return
	}

	// Detect type from API response
	detectedType := detectSoftwareType(title)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), titleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("title_id"), titleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), detectedType)...)

	if teamID != nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), int64(*teamID))...)
	}

	switch detectedType {
	case "vpp":
		if title.AppStoreApp != nil {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app_store_id"), title.AppStoreApp.AdamID)...)
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("filename"), "")...)
		}
	default:
		if title.SoftwarePackage != nil {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("filename"), title.SoftwarePackage.Name)...)
		} else {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("filename"), "")...)
		}
	}

	// Neither package_path nor package_s3 are stored by Fleet.
	// After import, set one of them in your Terraform config to manage the package binary.

	// Set package_sha256 from the Fleet API if available
	packageSHA := ""
	if title.SoftwarePackage != nil && title.SoftwarePackage.HashSHA256 != "" {
		packageSHA = title.SoftwarePackage.HashSHA256
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("package_sha256"), packageSHA)...)
}
