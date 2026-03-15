package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &softwarePackageResource{}
	_ resource.ResourceWithConfigure   = &softwarePackageResource{}
	_ resource.ResourceWithImportState = &softwarePackageResource{}
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
	ID                types.Int64  `tfsdk:"id"`
	TitleID           types.Int64  `tfsdk:"title_id"`
	TeamID            types.Int64  `tfsdk:"team_id"`
	Name              types.String `tfsdk:"name"`
	Version           types.String `tfsdk:"version"`
	Filename          types.String `tfsdk:"filename"`
	PackagePath       types.String `tfsdk:"package_path"`
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

// Metadata returns the resource type name.
func (r *softwarePackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_package"
}

// Schema defines the schema for the resource.
func (r *softwarePackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FleetDM software package. This is a Premium feature.",
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
			"name": schema.StringAttribute{
				Description: "The name of the software (extracted from the package).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.StringAttribute{
				Description: "The version of the software (extracted from the package).",
				Computed:    true,
			},
			"filename": schema.StringAttribute{
				Description: "The filename of the package (e.g., 'myapp-1.0.0.pkg').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"package_path": schema.StringAttribute{
				Description: "The filesystem path to the software package file. Supports .pkg, .msi, .deb, .rpm, and .exe files.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"package_sha256": schema.StringAttribute{
				Description: "The SHA256 hash of the package file content. Used to detect changes.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform": schema.StringAttribute{
				Description: "The platform the package is for (darwin, windows, linux).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"install_script": schema.StringAttribute{
				Description: "The script to run during installation. Optional.",
				Optional:    true,
			},
			"uninstall_script": schema.StringAttribute{
				Description: "The script to run during uninstallation. Optional.",
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
		},
	}
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

	// Read package file from disk
	packagePath := plan.PackagePath.ValueString()
	packageContent, err := os.ReadFile(packagePath) // #nosec G304 -- path comes from Terraform config
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading package file",
			fmt.Sprintf("Could not read package file at %s: %s", packagePath, err.Error()),
		)
		return
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(packageContent)
	packageSHA256 := hex.EncodeToString(hash[:])

	// Build the upload request
	uploadReq := &fleetdm.UploadSoftwarePackageRequest{
		Software:          packageContent,
		Filename:          plan.Filename.ValueString(),
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
	if !plan.LabelsIncludeAny.IsNull() && !plan.LabelsIncludeAny.IsUnknown() {
		var labels []string
		diags = plan.LabelsIncludeAny.ElementsAs(ctx, &labels, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		uploadReq.LabelsIncludeAny = labels
	}

	if !plan.LabelsExcludeAny.IsNull() && !plan.LabelsExcludeAny.IsUnknown() {
		var labels []string
		diags = plan.LabelsExcludeAny.ElementsAs(ctx, &labels, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		uploadReq.LabelsExcludeAny = labels
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
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
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

	// Check if this title has a software package associated
	if title.SoftwarePackage == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with read values
	state.Name = types.StringValue(title.Name)
	if len(title.Versions) > 0 {
		state.Version = types.StringValue(title.Versions[0].Version)
	}
	state.Platform = types.StringValue(title.Source)

	// Set the state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state.
func (r *softwarePackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan softwarePackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(plan.TitleID.ValueInt64())
	teamID := optionalIntPtr(plan.TeamID)

	// Build patch request with all mutable fields.
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

	if !plan.LabelsIncludeAny.IsNull() && !plan.LabelsIncludeAny.IsUnknown() {
		var labels []string
		diags = plan.LabelsIncludeAny.ElementsAs(ctx, &labels, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		patchReq.LabelsIncludeAny = labels
	}

	if !plan.LabelsExcludeAny.IsNull() && !plan.LabelsExcludeAny.IsUnknown() {
		var labels []string
		diags = plan.LabelsExcludeAny.ElementsAs(ctx, &labels, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		patchReq.LabelsExcludeAny = labels
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

	// Fetch the installer metadata so we can set the real filename in state,
	// avoiding a spurious RequiresReplace on the next plan.
	installer, err := r.client.GetSoftwareInstaller(ctx, titleID, teamID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading software package during import",
			fmt.Sprintf("Could not read software installer for title ID %d: %s", titleID, err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), titleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("title_id"), titleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("filename"), installer.Filename)...)

	if teamID != nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), int64(*teamID))...)
	}

	// package_path is a local filesystem path that Fleet does not store.
	// After import, set package_path in your Terraform config to the local file.
	// The first apply will re-upload the package only if the file content differs
	// from what is already installed (detected via SHA256).
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("package_path"), "")...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("package_sha256"), "")...)
}
