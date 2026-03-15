package provider

import (
	"context"
	"encoding/base64"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &bootstrapPackageResource{}
	_ resource.ResourceWithConfigure   = &bootstrapPackageResource{}
	_ resource.ResourceWithImportState = &bootstrapPackageResource{}
)

// NewBootstrapPackageResource is a helper function to simplify the provider implementation.
func NewBootstrapPackageResource() resource.Resource {
	return &bootstrapPackageResource{}
}

// bootstrapPackageResource is the resource implementation.
type bootstrapPackageResource struct {
	client *fleetdm.Client
}

// bootstrapPackageResourceModel maps the resource schema data.
type bootstrapPackageResourceModel struct {
	ID             types.Int64  `tfsdk:"id"`
	TeamID         types.Int64  `tfsdk:"team_id"`
	Name           types.String `tfsdk:"name"`
	PackageContent types.String `tfsdk:"package_content"`
	Sha256         types.String `tfsdk:"sha256"`
	Token          types.String `tfsdk:"token"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

// Metadata returns the resource type name.
func (r *bootstrapPackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bootstrap_package"
}

// Schema defines the schema for the resource.
func (r *bootstrapPackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FleetDM bootstrap package. This is a Premium feature. Bootstrap packages are automatically installed on macOS hosts during DEP enrollment.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "The unique identifier (same as team_id).",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.Int64Attribute{
				Description: "The ID of the team this bootstrap package belongs to. Required.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The filename of the bootstrap package (e.g., 'bootstrap.pkg').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"package_content": schema.StringAttribute{
				Description: "The base64-encoded content of the bootstrap package file (.pkg).",
				Required:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"sha256": schema.StringAttribute{
				Description: "The SHA256 hash of the package.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"token": schema.StringAttribute{
				Description: "The token for downloading the bootstrap package.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Description: "When the bootstrap package was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *bootstrapPackageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *bootstrapPackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan bootstrapPackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Decode base64 package content
	packageContent, err := base64.StdEncoding.DecodeString(plan.PackageContent.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error decoding package content",
			"Could not decode base64 package content: "+err.Error(),
		)
		return
	}

	teamID := int(plan.TeamID.ValueInt64())

	// Build the upload request
	uploadReq := &fleetdm.UploadBootstrapPackageRequest{
		TeamID:  teamID,
		Package: packageContent,
		Name:    plan.Name.ValueString(),
	}

	// Upload the bootstrap package
	err = r.client.UploadBootstrapPackage(ctx, uploadReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error uploading bootstrap package",
			"Could not upload bootstrap package: "+err.Error(),
		)
		return
	}

	// Fetch the metadata to get computed fields
	metadata, err := r.client.GetBootstrapPackageMetadata(ctx, teamID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading bootstrap package metadata",
			"Could not read bootstrap package metadata after upload: "+err.Error(),
		)
		return
	}

	// Update state with computed values
	plan.ID = types.Int64Value(int64(teamID))
	plan.Sha256 = types.StringValue(metadata.Sha256)
	plan.Token = types.StringValue(metadata.Token)
	plan.CreatedAt = types.StringValue(metadata.CreatedAt)

	// Set the state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *bootstrapPackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state bootstrapPackageResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := int(state.TeamID.ValueInt64())

	// Get the bootstrap package metadata
	metadata, err := r.client.GetBootstrapPackageMetadata(ctx, teamID)
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading bootstrap package",
			"Could not read bootstrap package: "+err.Error(),
		)
		return
	}

	// Update state with read values
	state.Name = types.StringValue(metadata.Name)
	state.Sha256 = types.StringValue(metadata.Sha256)
	state.Token = types.StringValue(metadata.Token)
	state.CreatedAt = types.StringValue(metadata.CreatedAt)

	// Set the state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state.
func (r *bootstrapPackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan bootstrapPackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Bootstrap packages cannot be updated in-place, they must be replaced.
	// The schema has RequiresReplace on the critical fields.
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state.
func (r *bootstrapPackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state bootstrapPackageResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := int(state.TeamID.ValueInt64())

	// Delete the bootstrap package
	err := r.client.DeleteBootstrapPackage(ctx, teamID)
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting bootstrap package",
			"Could not delete bootstrap package: "+err.Error(),
		)
		return
	}
}

// ImportState imports an existing resource by ID.
func (r *bootstrapPackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: team_id
	teamID, ok := parseIDFromString(req.ID, "Bootstrap Package", &resp.Diagnostics)
	if !ok {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), teamID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), teamID)...)
	// We can't import package_content since it's not stored
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("package_content"), "")...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), "imported.pkg")...)
}
