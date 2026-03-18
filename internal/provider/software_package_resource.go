package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	gopath "path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

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
	ID                   types.Int64  `tfsdk:"id"`
	TitleID              types.Int64  `tfsdk:"title_id"`
	TeamID               types.Int64  `tfsdk:"team_id"`
	Type                 types.String `tfsdk:"type"`
	Name                 types.String `tfsdk:"name"`
	Version              types.String `tfsdk:"version"`
	Filename             types.String `tfsdk:"filename"`
	PackagePath          types.String `tfsdk:"package_path"`
	PackageS3            types.Object `tfsdk:"package_s3"`
	PackageSHA256        types.String `tfsdk:"package_sha256"`
	Platform             types.String `tfsdk:"platform"`
	InstallScript        types.String `tfsdk:"install_script"`
	UninstallScript      types.String `tfsdk:"uninstall_script"`
	PreInstallQuery      types.String `tfsdk:"pre_install_query"`
	PostInstallScript    types.String `tfsdk:"post_install_script"`
	SelfService          types.Bool   `tfsdk:"self_service"`
	AutomaticInstall     types.Bool   `tfsdk:"automatic_install"`
	LabelsIncludeAny     types.List   `tfsdk:"labels_include_any"`
	LabelsExcludeAny     types.List   `tfsdk:"labels_exclude_any"`
	AppStoreID           types.String `tfsdk:"app_store_id"`
	FleetMaintainedAppID types.Int64  `tfsdk:"fleet_maintained_app_id"`
}

// packageS3Model maps the nested package_s3 attribute.
type packageS3Model struct {
	Bucket      types.String `tfsdk:"bucket"`
	Key         types.String `tfsdk:"key"`
	Region      types.String `tfsdk:"region"`
	EndpointURL types.String `tfsdk:"endpoint_url"`
}

// Metadata returns the resource type name.
func (r *softwarePackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_package"
}

// Schema defines the schema for the resource.
func (r *softwarePackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FleetDM software package, VPP (App Store) app, or Fleet Maintained App. This is a Premium feature.",
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
				Description: "S3 source for the software package. Alternative to package_path. The provider downloads the object from S3 and uploads it to Fleet. Mutually exclusive with package_path. Note: bucket and key must be known at plan time (they cannot reference computed values from resources that haven't been created yet).",
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
				Description: "Whether to automatically install the software during device setup (install during setup). Defaults to false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"labels_include_any": schema.ListAttribute{
				Description: "List of label names. The software will be available for hosts that match any of these labels.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"labels_exclude_any": schema.ListAttribute{
				Description: "List of label names. The software will not be available for hosts that match any of these labels.",
				Optional:    true,
				ElementType: types.StringType,
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

	// Validate package_s3 fields when the block is present.
	if hasS3 {
		var s3Config packageS3Model
		diags := data.PackageS3.As(ctx, &s3Config, basetypes.ObjectAsOptions{})
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		{
			if s3Config.Bucket.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					path.Root("package_s3"),
					"Invalid Configuration",
					"package_s3.bucket must be a known value at plan time. It cannot reference computed attributes from uncreated resources.",
				)
			} else if s3Config.Bucket.ValueString() == "" {
				resp.Diagnostics.AddAttributeError(
					path.Root("package_s3"),
					"Invalid Configuration",
					"package_s3.bucket must not be empty.",
				)
			}
			if s3Config.Key.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					path.Root("package_s3"),
					"Invalid Configuration",
					"package_s3.key must be a known value at plan time. It cannot reference computed attributes from uncreated resources.",
				)
			} else if s3Config.Key.ValueString() == "" {
				resp.Diagnostics.AddAttributeError(
					path.Root("package_s3"),
					"Invalid Configuration",
					"package_s3.key must not be empty.",
				)
			}
		}
	}
}

// ModifyPlan computes package_sha256 at plan time from the package source.
// This ensures that changes to the file content (local or S3) are detected
// even when the Terraform config itself hasn't changed.
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

	// Attempt to read the package and compute SHA at plan time.
	_, computedSHA, err := readPackageContent(ctx, &plan)
	if err != nil {
		hasLocalPath := !plan.PackagePath.IsNull() && !plan.PackagePath.IsUnknown() && plan.PackagePath.ValueString() != ""
		if hasLocalPath {
			// Local file errors (missing file, permission denied) should be reported
			// immediately so users get feedback during plan.
			resp.Diagnostics.AddWarning(
				"Unable to read package file",
				fmt.Sprintf("Could not read package at plan time: %s. The SHA will be computed during apply.", err.Error()),
			)
		}
		// S3 errors are silently suppressed — credentials or endpoints may not be
		// available during plan (e.g. assume-role not yet resolved).
		return
	}
	if computedSHA == "" {
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

	plan.PackageSHA256 = types.StringValue(computedSHA)
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
	// Read package content from local file or S3
	packageContent, packageSHA256, err := readPackageContent(ctx, plan)
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

	// Build the upload request
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

	// Set team_id if specified
	uploadReq.TeamID = optionalIntPtr(plan.TeamID)

	// Extract label names from lists
	var diags = extractLabels(ctx, plan.LabelsIncludeAny, &uploadReq.LabelsIncludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractLabels(ctx, plan.LabelsExcludeAny, &uploadReq.LabelsExcludeAny)
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
	plan.Version = types.StringValue("")
	if len(title.Versions) > 0 {
		plan.Version = types.StringValue(title.Versions[0].Version)
	}
	plan.Platform = types.StringValue(title.Source)
	plan.PackageSHA256 = types.StringValue(packageSHA256)

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

	addReq := &fleetdm.AddFleetMaintainedAppRequest{
		FleetMaintainedAppID: int(plan.FleetMaintainedAppID.ValueInt64()),
		TeamID:               teamID,
		InstallScript:        plan.InstallScript.ValueString(),
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
}

// readPackageOrFMA populates state from a software package or Fleet Maintained App title.
func (r *softwarePackageResource) readPackageOrFMA(_ context.Context, title *fleetdm.SoftwareTitle, state *softwarePackageResourceModel) {
	state.Name = types.StringValue(title.Name)
	if len(title.Versions) > 0 {
		state.Version = types.StringValue(title.Versions[0].Version)
	}

	pkg := title.SoftwarePackage
	// Prefer the platform from the package metadata; fall back to source only when absent.
	if pkg.Platform != "" {
		state.Platform = types.StringValue(pkg.Platform)
	} else {
		state.Platform = types.StringValue(title.Source)
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
		r.updateVPP(ctx, titleID, teamID, &plan, resp)
	default:
		// Both "package" and "fleet_maintained" use PatchSoftwarePackage
		r.updatePackageOrFMA(ctx, titleID, teamID, &plan, &state, resp)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// updateVPP handles updating a VPP (App Store) app.
func (r *softwarePackageResource) updateVPP(ctx context.Context, titleID int, teamID *int, plan *softwarePackageResourceModel, resp *resource.UpdateResponse) {
	tid := 0
	if teamID != nil {
		tid = *teamID
	}

	updateReq := &fleetdm.UpdateAppStoreAppRequest{
		TeamID:           tid,
		SelfService:      plan.SelfService.ValueBool(),
		LabelsIncludeAny: []string{},
		LabelsExcludeAny: []string{},
	}

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

	if err := r.client.UpdateAppStoreApp(ctx, titleID, updateReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating VPP app",
			"Could not update App Store app: "+err.Error(),
		)
	}
}

// updatePackageOrFMA handles updating a software package or Fleet Maintained App.
func (r *softwarePackageResource) updatePackageOrFMA(ctx context.Context, titleID int, teamID *int, plan *softwarePackageResourceModel, priorState *softwarePackageResourceModel, resp *resource.UpdateResponse) {
	// Check if we have a package source (local file or S3)
	packageContent, localSHA, err := readPackageContent(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error reading package", err.Error())
		return
	}

	if packageContent != nil {
		// Compare with the SHA from the prior state (what Fleet had last time we read)
		currentSHA := priorState.PackageSHA256.ValueString()
		if localSHA != currentSHA {
			// SHA differs — delete and re-upload the package
			if err := r.client.DeleteSoftwarePackage(ctx, titleID, teamID); err != nil {
				if !isNotFound(err) {
					resp.Diagnostics.AddError(
						"Error replacing software package",
						"Could not delete existing package before re-upload: "+err.Error(),
					)
					return
				}
			}

			filename := deriveFilename(ctx, plan)
			if filename == "" {
				resp.Diagnostics.AddError("Missing filename", "Could not determine filename for re-upload. Set 'filename' explicitly.")
				return
			}
			plan.Filename = types.StringValue(filename)

			uploadReq := &fleetdm.UploadSoftwarePackageRequest{
				TeamID:            teamID,
				Software:          packageContent,
				Filename:          filename,
				InstallScript:     plan.InstallScript.ValueString(),
				UninstallScript:   plan.UninstallScript.ValueString(),
				PreInstallQuery:   plan.PreInstallQuery.ValueString(),
				PostInstallScript: plan.PostInstallScript.ValueString(),
				SelfService:       plan.SelfService.ValueBool(),
				AutomaticInstall:  plan.AutomaticInstall.ValueBool(),
			}

			uploadDiags := extractLabels(ctx, plan.LabelsIncludeAny, &uploadReq.LabelsIncludeAny)
			resp.Diagnostics.Append(uploadDiags...)
			if resp.Diagnostics.HasError() {
				return
			}
			uploadDiags = extractLabels(ctx, plan.LabelsExcludeAny, &uploadReq.LabelsExcludeAny)
			resp.Diagnostics.Append(uploadDiags...)
			if resp.Diagnostics.HasError() {
				return
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
				return
			}

			plan.ID = types.Int64Value(int64(title.ID))
			plan.TitleID = types.Int64Value(int64(title.ID))
			plan.Name = types.StringValue(title.Name)
			if len(title.Versions) > 0 {
				plan.Version = types.StringValue(title.Versions[0].Version)
			}
			if title.SoftwarePackage != nil && title.SoftwarePackage.Platform != "" {
				plan.Platform = types.StringValue(title.SoftwarePackage.Platform)
			} else {
				plan.Platform = types.StringValue(title.Source)
			}
			plan.PackageSHA256 = types.StringValue(localSHA)
			return
		}

		// SHA matches — just update metadata below
		plan.PackageSHA256 = types.StringValue(localSHA)
	}

	// Update metadata only (scripts, labels, self-service, etc.)
	patchReq := &fleetdm.PatchSoftwarePackageRequest{
		TeamID:             teamID,
		InstallScript:      plan.InstallScript.ValueString(),
		UninstallScript:    plan.UninstallScript.ValueString(),
		PreInstallQuery:    plan.PreInstallQuery.ValueString(),
		PostInstallScript:  plan.PostInstallScript.ValueString(),
		SelfService:        plan.SelfService.ValueBool(),
		InstallDuringSetup: plan.AutomaticInstall.ValueBool(),
		LabelsIncludeAny:   []string{},
		LabelsExcludeAny:   []string{},
	}

	var diags diag.Diagnostics
	diags = extractLabels(ctx, plan.LabelsIncludeAny, &patchReq.LabelsIncludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = extractLabels(ctx, plan.LabelsExcludeAny, &patchReq.LabelsExcludeAny)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.PatchSoftwarePackage(ctx, titleID, patchReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating software package",
			"Could not update software package metadata: "+err.Error(),
		)
	}
}

// readPackageContent reads the package content from package_path or package_s3.
// Returns the content, SHA256 hex digest, and any error.
func readPackageContent(ctx context.Context, model *softwarePackageResourceModel) ([]byte, string, error) {
	hasPath := !model.PackagePath.IsNull() && !model.PackagePath.IsUnknown() && model.PackagePath.ValueString() != ""
	s3Present := !model.PackageS3.IsNull() && !model.PackageS3.IsUnknown()

	if hasPath && s3Present {
		return nil, "", fmt.Errorf("package_path and package_s3 are mutually exclusive; set one or the other")
	}

	var content []byte
	var err error

	if hasPath {
		content, err = os.ReadFile(model.PackagePath.ValueString()) // #nosec G304 -- path comes from Terraform config
		if err != nil {
			return nil, "", fmt.Errorf("could not read package at %s: %w", model.PackagePath.ValueString(), err)
		}
	} else if s3Present {
		var s3Config packageS3Model
		diags := model.PackageS3.As(ctx, &s3Config, basetypes.ObjectAsOptions{})
		if diags.HasError() {
			var details string
			for _, d := range diags.Errors() {
				details += d.Summary() + ": " + d.Detail() + "; "
			}
			return nil, "", fmt.Errorf("could not parse package_s3 configuration: %s", details)
		}

		if s3Config.Bucket.IsUnknown() || s3Config.Key.IsUnknown() {
			return nil, "", fmt.Errorf("package_s3 bucket and key must be known values; they cannot be derived from resources that haven't been created yet")
		}
		if s3Config.Bucket.ValueString() == "" {
			return nil, "", fmt.Errorf("package_s3 bucket must not be empty")
		}
		if s3Config.Key.ValueString() == "" {
			return nil, "", fmt.Errorf("package_s3 key must not be empty")
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

		content, err = fleetdm.DownloadS3Object(ctx, src)
		if err != nil {
			return nil, "", err
		}
	} else {
		return nil, "", nil // no source specified, that's OK for tracked-only packages
	}

	hash := sha256.Sum256(content)
	return content, hex.EncodeToString(hash[:]), nil
}

// deriveFilename returns the filename to use for the package upload.
// Priority: explicit filename attribute > package_path basename > package_s3 key basename.
func deriveFilename(ctx context.Context, model *softwarePackageResourceModel) string {
	if !model.Filename.IsNull() && !model.Filename.IsUnknown() && model.Filename.ValueString() != "" {
		return model.Filename.ValueString()
	}
	if !model.PackagePath.IsNull() && !model.PackagePath.IsUnknown() && model.PackagePath.ValueString() != "" {
		return filepath.Base(model.PackagePath.ValueString())
	}
	if !model.PackageS3.IsNull() && !model.PackageS3.IsUnknown() {
		var s3Cfg packageS3Model
		if d := model.PackageS3.As(ctx, &s3Cfg, basetypes.ObjectAsOptions{}); !d.HasError() {
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
