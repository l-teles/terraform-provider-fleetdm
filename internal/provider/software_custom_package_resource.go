package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                   = &softwareCustomPackageResource{}
	_ resource.ResourceWithConfigure      = &softwareCustomPackageResource{}
	_ resource.ResourceWithImportState    = &softwareCustomPackageResource{}
	_ resource.ResourceWithValidateConfig = &softwareCustomPackageResource{}
	_ resource.ResourceWithModifyPlan     = &softwareCustomPackageResource{}
)

// NewSoftwareCustomPackageResource is the constructor registered with the
// provider.
func NewSoftwareCustomPackageResource() resource.Resource {
	return &softwareCustomPackageResource{}
}

// softwareCustomPackageResource manages a user-uploaded software package
// (.pkg, .msi, .deb, .rpm, .exe). The package binary is sourced either from
// a local file (`package_path`) or an S3 object (`package_s3`); Fleet
// computes the install/uninstall scripts when none are provided.
//
// This is the heaviest of the three split resources because it owns the S3
// SHA-resolution path, the binary-replace flow (delete + re-upload while
// detaching/reattaching install_software policy automation), and a
// ValidateConfig hook for the package source mutual exclusion.
type softwareCustomPackageResource struct {
	client *fleetdm.Client
}

// softwareCustomPackageResourceModel maps the resource schema data. Drops
// type, app_store_id, fleet_maintained_app_id from the legacy model; the
// resource is unambiguously a custom upload.
type softwareCustomPackageResourceModel struct {
	ID                types.Int64  `tfsdk:"id"`
	TitleID           types.Int64  `tfsdk:"title_id"`
	TeamID            types.Int64  `tfsdk:"team_id"`
	Name              types.String `tfsdk:"name"`
	Version           types.String `tfsdk:"version"`
	Filename          types.String `tfsdk:"filename"`
	PackagePath       types.String `tfsdk:"package_path"`
	PackageS3         types.Object `tfsdk:"package_s3"`
	PackageSHA256     types.String `tfsdk:"package_sha256"`
	Platform          types.String `tfsdk:"platform"`
	InstallScript     types.String `tfsdk:"install_script"`
	UninstallScript   types.String `tfsdk:"uninstall_script"`
	PreInstallQuery   types.String `tfsdk:"pre_install_query"`
	PostInstallScript types.String `tfsdk:"post_install_script"`
	SelfService       types.Bool   `tfsdk:"self_service"`
	AutomaticInstall  types.Bool   `tfsdk:"automatic_install"`
	LabelsIncludeAny  types.List   `tfsdk:"labels_include_any"`
	LabelsExcludeAny  types.List   `tfsdk:"labels_exclude_any"`
}

// packageSource adapters so the shared binary-source helpers
// (resolveRemoteSHA / readPackageContentForUpload / deriveFilename /
// buildS3Source) accept this model alongside the legacy one.
func (m *softwareCustomPackageResourceModel) PackagePathField() types.String { return m.PackagePath }
func (m *softwareCustomPackageResourceModel) PackageS3Field() types.Object   { return m.PackageS3 }
func (m *softwareCustomPackageResourceModel) FilenameField() types.String    { return m.Filename }

// Metadata returns the resource type name.
func (r *softwareCustomPackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_custom_package"
}

// Schema defines the schema for the resource. It's the union of the shared
// software attributes and the custom-package-specific attributes
// (package_path, package_s3, package_sha256, filename, scripts).
func (r *softwareCustomPackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := softwareCommonSchemaAttributes()
	for k, v := range softwareScriptAttributes() {
		attrs[k] = v
	}
	attrs["filename"] = schema.StringAttribute{
		Description: "The filename of the package (e.g., 'myapp-1.0.0.pkg'). Required if the filename cannot be derived from package_path or package_s3.key.",
		Optional:    true,
		Computed:    true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
	attrs["package_path"] = schema.StringAttribute{
		Description: "Filesystem path to the package file. If set, the file is uploaded to Fleet whenever its SHA256 differs from the current package. " +
			"Supports .pkg, .msi, .deb, .rpm, and .exe files. Mutually exclusive with `package_s3`.",
		Optional: true,
	}
	attrs["package_s3"] = schema.SingleNestedAttribute{
		Description: "S3 source for the package binary. Alternative to `package_path`. The provider reads the SHA256 via HeadObject and only " +
			"downloads + re-uploads to Fleet when the hash differs from what Fleet has stored. Mutually exclusive with `package_path`. " +
			"`bucket`, `key`, and `region` may reference module outputs or other resources' attributes — when their values aren't yet known " +
			"at plan time, the SHA comparison is deferred to apply time.",
		Optional: true,
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
					"think the installer is unchanged and the package will NOT be re-uploaded.",
				Optional: true,
			},
		},
	}
	attrs["package_sha256"] = schema.StringAttribute{
		Description: "The SHA256 hash of the package in Fleet. Computed at plan time from the local file (package_path) or S3 object " +
			"(package_s3), or read from Fleet's API. Can be set explicitly to avoid drift on import.",
		Optional: true,
		Computed: true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
	resp.Schema = schema.Schema{
		Description: "Manages a user-uploaded software package (.pkg, .msi, .deb, .rpm, .exe) bound to a Fleet team. " +
			"The package binary is sourced from a local file (`package_path`) or an S3 object (`package_s3`). " +
			"Fleet Premium only.",
		Attributes: attrs,
	}
}

// ValidateConfig enforces that package_path and package_s3 are mutually
// exclusive and that package_s3 inner fields are well-formed.
func (r *softwareCustomPackageResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data softwareCustomPackageResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasPath := !data.PackagePath.IsNull() && !data.PackagePath.IsUnknown() && data.PackagePath.ValueString() != ""
	hasS3 := !data.PackageS3.IsNull() && !data.PackageS3.IsUnknown()

	if hasPath && hasS3 {
		resp.Diagnostics.AddAttributeError(
			path.Root("package_s3"),
			"Conflicting Configuration",
			"package_path and package_s3 are mutually exclusive. Set one or the other, not both.",
		)
	}

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

// ModifyPlan computes package_sha256 at plan time from the package source.
// For S3 sources this resolves the SHA via HeadObject when possible, falling
// back to downloading the body only when neither a server-managed checksum
// nor an x-amz-meta-sha256 header is available. Adapted from the legacy
// resource's ModifyPlan — the only change is that this resource is always
// type=package, so the swType gating check is dropped.
func (r *softwareCustomPackageResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan softwareCustomPackageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sha, _, requiresDownload, diags := resolveRemoteSHA(ctx, &plan, true)

	// Local-file errors are emitted as warnings during plan (the file may
	// not exist yet on this machine even though it will at apply time).
	// S3 errors stay as errors.
	hasLocalPath := !plan.PackagePath.IsNull() && !plan.PackagePath.IsUnknown() && plan.PackagePath.ValueString() != ""
	if diags.HasError() && hasLocalPath {
		for _, d := range diags.Errors() {
			resp.Diagnostics.AddWarning(d.Summary(), d.Detail())
		}
		return
	}
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if requiresDownload {
		_, downloadedSHA, err := readPackageContentForUpload(ctx, &plan)
		if err != nil {
			return
		}
		sha = downloadedSHA
	}

	if sha == "" {
		return
	}

	var config softwareCustomPackageResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !config.PackageSHA256.IsNull() && !config.PackageSHA256.IsUnknown() {
		return
	}

	plan.PackageSHA256 = types.StringValue(sha)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// Configure injects the API client.
func (r *softwareCustomPackageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create uploads the package binary and installs it on the specified team.
func (r *softwareCustomPackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan softwareCustomPackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	packageContent, packageSHA256, err := readPackageContentForUpload(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error reading package", err.Error())
		return
	}
	if packageContent == nil {
		resp.Diagnostics.AddError(
			"Missing package source",
			"Either package_path or package_s3 must be set for fleetdm_software_custom_package.",
		)
		return
	}

	filename := deriveFilename(ctx, &plan)
	if filename == "" {
		resp.Diagnostics.AddError(
			"Missing filename",
			"Could not determine filename. Set 'filename' explicitly, or ensure package_path or package_s3.key is set.",
		)
		return
	}
	plan.Filename = types.StringValue(filename)

	uploadReq := &fleetdm.UploadSoftwarePackageRequest{
		Software:          packageContent,
		Filename:          filename,
		InstallScript:     plan.InstallScript.ValueString(),
		UninstallScript:   plan.UninstallScript.ValueString(),
		PreInstallQuery:   plan.PreInstallQuery.ValueString(),
		PostInstallScript: plan.PostInstallScript.ValueString(),
		SelfService:       plan.SelfService.ValueBool(),
		AutomaticInstall:  plan.AutomaticInstall.ValueBool(),
	}
	uploadReq.TeamID = optionalIntPtr(plan.TeamID)

	var d diag.Diagnostics
	d = extractOptionalLabels(ctx, plan.LabelsIncludeAny, &uploadReq.LabelsIncludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	d = extractOptionalLabels(ctx, plan.LabelsExcludeAny, &uploadReq.LabelsExcludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	title, err := r.client.UploadSoftwarePackage(ctx, uploadReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error uploading software package",
			"Could not upload software package: "+err.Error(),
		)
		return
	}

	plan.ID = types.Int64Value(int64(title.ID))
	plan.TitleID = types.Int64Value(int64(title.ID))
	plan.Name = types.StringValue(title.Name)
	plan.Version = types.StringValue("")
	if len(title.Versions) > 0 {
		plan.Version = types.StringValue(title.Versions[0].Version)
	}
	if title.SoftwarePackage != nil && title.SoftwarePackage.Platform != "" {
		plan.Platform = types.StringValue(title.SoftwarePackage.Platform)
	} else if plan.Platform.IsNull() || plan.Platform.IsUnknown() {
		plan.Platform = types.StringValue("")
	}
	plan.PackageSHA256 = types.StringValue(packageSHA256)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes state from Fleet. Wrong-type protection: rejects VPP
// titles being imported into this resource. Cannot distinguish a
// user-uploaded custom package from an FMA-managed title via Fleet's GET
// response — both expose a `software_package` block. That's a limitation
// inherited from Fleet's API.
func (r *softwareCustomPackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state softwareCustomPackageResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	teamID := optionalIntPtr(state.TeamID)

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

	if title.AppStoreApp != nil {
		// Fresh import vs previously-managed resource — see the VPP
		// resource's Read for the rationale.
		if state.Name.IsNull() {
			resp.Diagnostics.AddError(
				"Wrong software type",
				fmt.Sprintf("title %d is a VPP/App Store app; use fleetdm_software_app_store_app instead", titleID),
			)
			return
		}
		resp.State.RemoveResource(ctx)
		return
	}
	if title.SoftwarePackage == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Name = types.StringValue(title.Name)
	if len(title.Versions) > 0 {
		state.Version = types.StringValue(title.Versions[0].Version)
	}
	pkg := title.SoftwarePackage
	if pkg.Platform != "" {
		state.Platform = types.StringValue(pkg.Platform)
	}
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
	if pkg.InstallDuringSetup != nil {
		state.AutomaticInstall = types.BoolValue(*pkg.InstallDuringSetup)
	}
	if pkg.HashSHA256 != "" {
		state.PackageSHA256 = types.StringValue(pkg.HashSHA256)
	}
	if pkg.LabelsIncludeAny != nil && !state.LabelsIncludeAny.IsNull() {
		state.LabelsIncludeAny = labelsToStringListValue(pkg.LabelsIncludeAny)
	}
	if pkg.LabelsExcludeAny != nil && !state.LabelsExcludeAny.IsNull() {
		state.LabelsExcludeAny = labelsToStringListValue(pkg.LabelsExcludeAny)
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update may replace the binary (when SHA differs) and always PATCHes the
// metadata. The fast path (cheap SHA resolve) avoids downloading the body
// when Fleet already has the exact bytes; the slow path falls back to
// fetching and rehashing.
func (r *softwareCustomPackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan softwareCustomPackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state softwareCustomPackageResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(plan.TitleID.ValueInt64())
	teamID := optionalIntPtr(plan.TeamID)

	hasPath := !plan.PackagePath.IsNull() && !plan.PackagePath.IsUnknown() && plan.PackagePath.ValueString() != ""
	hasS3 := !plan.PackageS3.IsNull() && !plan.PackageS3.IsUnknown()
	hasSource := hasPath || hasS3

	if hasSource {
		sha, _, requiresDownload, d := resolveRemoteSHA(ctx, &plan, true)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}

		currentSHA := state.PackageSHA256.ValueString()

		needsUpload := false
		var preFetched []byte
		var resolvedSHA string

		if !requiresDownload {
			resolvedSHA = sha
			needsUpload = sha != currentSHA || currentSHA == ""
		} else {
			content, localSHA, err := readPackageContentForUpload(ctx, &plan)
			if err != nil {
				resp.Diagnostics.AddError("Error reading package", err.Error())
				return
			}
			resolvedSHA = localSHA
			preFetched = content
			needsUpload = localSHA != currentSHA
		}

		if !needsUpload {
			plan.PackageSHA256 = types.StringValue(resolvedSHA)
		} else {
			if preFetched == nil {
				content, localSHA, err := readPackageContentForUpload(ctx, &plan)
				if err != nil {
					resp.Diagnostics.AddError("Error reading package", err.Error())
					return
				}
				preFetched = content
				resolvedSHA = localSHA
			}
			if !r.replacePackage(ctx, titleID, teamID, &plan, preFetched, resolvedSHA, resp) {
				return
			}
		}
	}

	patchReq := &fleetdm.PatchSoftwarePackageRequest{
		TeamID:             teamID,
		InstallScript:      plan.InstallScript.ValueString(),
		UninstallScript:    plan.UninstallScript.ValueString(),
		PreInstallQuery:    plan.PreInstallQuery.ValueString(),
		PostInstallScript:  plan.PostInstallScript.ValueString(),
		SelfService:        plan.SelfService.ValueBool(),
		InstallDuringSetup: plan.AutomaticInstall.ValueBool(),
	}

	var d diag.Diagnostics
	d = extractOptionalLabels(ctx, plan.LabelsIncludeAny, &patchReq.LabelsIncludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	d = extractOptionalLabels(ctx, plan.LabelsExcludeAny, &patchReq.LabelsExcludeAny)
	resp.Diagnostics.Append(d...)
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

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// replacePackage deletes the existing package and uploads a replacement
// while detaching/reattaching install_software policy automation. Lifted
// from the legacy resource's replaceSoftwarePackage; the behavior is
// identical, only the model type differs.
func (r *softwareCustomPackageResource) replacePackage(ctx context.Context, titleID int, teamID *int, plan *softwareCustomPackageResourceModel, content []byte, sha string, resp *resource.UpdateResponse) bool {
	attachedPolicies, err := r.client.ListPoliciesByInstallSoftwareTitleID(ctx, titleID, teamID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error replacing software package",
			"Could not list policies before re-upload (needed to clear install_software automation): "+err.Error(),
		)
		return false
	}
	for _, p := range attachedPolicies {
		if err := r.client.SetPolicyInstallSoftwareTitleID(ctx, p.ID, teamID, nil); err != nil {
			resp.Diagnostics.AddError(
				"Error replacing software package",
				fmt.Sprintf("Could not detach install_software automation from policy %d (%q) before re-upload: %s", p.ID, p.Name, err.Error()),
			)
			return false
		}
	}

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

	uploadReq := &fleetdm.UploadSoftwarePackageRequest{
		TeamID:            teamID,
		Software:          content,
		Filename:          filename,
		InstallScript:     plan.InstallScript.ValueString(),
		UninstallScript:   plan.UninstallScript.ValueString(),
		PreInstallQuery:   plan.PreInstallQuery.ValueString(),
		PostInstallScript: plan.PostInstallScript.ValueString(),
		SelfService:       plan.SelfService.ValueBool(),
		AutomaticInstall:  plan.AutomaticInstall.ValueBool(),
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

	newTitleID := title.ID
	var reattachFailed []string
	for _, p := range attachedPolicies {
		if err := r.client.SetPolicyInstallSoftwareTitleID(ctx, p.ID, teamID, &newTitleID); err != nil {
			reattachFailed = append(reattachFailed, fmt.Sprintf("%d (%q): %s", p.ID, p.Name, err.Error()))
		}
	}
	if len(reattachFailed) > 0 {
		stateDiags := resp.State.Set(ctx, *plan)
		resp.Diagnostics.Append(stateDiags...)

		resp.Diagnostics.AddError(
			"Error re-attaching install_software automation after package replace",
			"The software package was replaced successfully (new title_id="+strconv.Itoa(newTitleID)+"), but re-attaching install_software automation to the following policies failed:\n  - "+
				strings.Join(reattachFailed, "\n  - ")+
				"\n\nThe affected fleetdm_policy resources will show drift on `software_title_id` on the next plan; re-running 'terraform apply' should heal them automatically.",
		)
		return false
	}

	return true
}

// Delete removes the package from Fleet.
func (r *softwareCustomPackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state softwareCustomPackageResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	teamID := optionalIntPtr(state.TeamID)

	err := r.client.DeleteSoftwarePackage(ctx, titleID, teamID)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError(
			"Error deleting software package",
			"Could not delete software package: "+err.Error(),
		)
	}
}

// ImportState imports an existing custom-package title by ID. Format:
// `title_id` or `title_id:team_id`. Users must set `package_path` or
// `package_s3` in HCL after import — Fleet doesn't store those, so the
// provider can't reconstruct them from the GET response.
func (r *softwareCustomPackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
			fmt.Sprintf("Could not parse title ID %q: %s", parts[0], err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), int64(titleID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("title_id"), int64(titleID))...)

	if len(parts) == 2 {
		tid, err := strconv.Atoi(parts[1])
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid team ID",
				fmt.Sprintf("Could not parse team ID %q: %s", parts[1], err.Error()),
			)
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), int64(tid))...)
	}
}
