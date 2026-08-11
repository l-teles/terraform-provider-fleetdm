package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces. Note the
// deliberate absence of resource.ResourceWithImportState: an Android web app
// cannot be imported because Fleet exposes no way to read one back (see the
// comment on Read).
var (
	_ resource.Resource              = &softwareWebAppResource{}
	_ resource.ResourceWithConfigure = &softwareWebAppResource{}
)

// NewSoftwareWebAppResource is the constructor registered with the provider.
func NewSoftwareWebAppResource() resource.Resource {
	return &softwareWebAppResource{}
}

// softwareWebAppResource manages an Android web app (web clip).
//
// This resource is unusual, and the shape follows directly from what Fleet
// actually implements. POST /software/web_apps is a thin wrapper over Google's
// Android Management API: it registers a web app inside the Android enterprise
// and stores nothing in Fleet. Fleet 4.90 registers no other route on that
// path — GET answers 405, and there is no list and no DELETE.
//
// So the lifecycle is create-only:
//   - Create calls the endpoint and records the returned app_store_id.
//   - Read is a no-op; there is no endpoint to refresh from.
//   - Update never runs, because every configurable attribute RequiresReplace.
//   - Delete drops the resource from state and warns that the web app itself
//     remains registered in the Android enterprise.
//
// The point of the resource is app_store_id: feed it to
// fleetdm_software_app_store_app with platform = "android" to make the web app
// installable on a team. That companion resource is what has a real, readable,
// deletable lifecycle in Fleet.
type softwareWebAppResource struct {
	client *fleetdm.Client
}

// softwareWebAppResourceModel maps the resource schema data.
type softwareWebAppResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Title      types.String `tfsdk:"title"`
	URL        types.String `tfsdk:"url"`
	IconPath   types.String `tfsdk:"icon_path"`
	AppStoreID types.String `tfsdk:"app_store_id"`
}

// Metadata returns the resource type name.
func (r *softwareWebAppResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_web_app"
}

// Schema defines the schema for the resource. Every configurable attribute
// forces replacement: Fleet has no endpoint to modify an existing web app.
func (r *softwareWebAppResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Android web app (web clip). Creating one registers the web app in your Android " +
			"enterprise and yields an `app_store_id`; pass that to `fleetdm_software_app_store_app` with " +
			"`platform = \"android\"` to make it installable on a team.\n\n" +
			"Fleet exposes only a create endpoint for web apps — there is no read, update, or delete. " +
			"That has three consequences worth planning around:\n\n" +
			"* This resource cannot be imported, and drift is undetectable by design: because there is " +
			"nothing to read back, `terraform plan` will never report a web app that was changed or removed " +
			"outside Terraform. Use `terraform apply -replace=...` to force a fresh one.\n" +
			"* `icon_path` tracks the path, not the file contents. Editing the image in place produces no " +
			"diff; change the path or use `-replace` to push a new icon.\n" +
			"* Since nothing can be deleted, both destroying and replacing this resource leave the old web " +
			"app registered in your Android enterprise. Every change to `title`, `url`, or `icon_path` " +
			"registers a new web app and abandons the previous one, so repeated edits accumulate " +
			"registrations that must be cleaned up in the Fleet UI.\n\n" +
			"Requires Fleet Premium with Android MDM enabled and configured.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The web app's package name, identical to `app_store_id`.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				Description: "The web app name shown to the end user under the app icon. Required. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"url": schema.StringAttribute{
				Description: "The URL the web app opens. Must be an absolute URL. Required. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"icon_path": schema.StringAttribute{
				Description: "Path to a local icon file for the web app. Fleet requires a square PNG of at least 512x512px. " +
					"Optional. Changing this forces a new resource.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"app_store_id": schema.StringAttribute{
				Description: "The package name Fleet generated for the web app, e.g. " +
					"`com.google.enterprise.webapp.0123456789abcdef`. Use this as the `app_store_id` of a " +
					"`fleetdm_software_app_store_app` resource.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure injects the API client.
func (r *softwareWebAppResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create registers the web app in the Android enterprise.
func (r *softwareWebAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan softwareWebAppResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &fleetdm.CreateWebAppRequest{
		Title: plan.Title.ValueString(),
		URL:   plan.URL.ValueString(),
	}

	if !plan.IconPath.IsNull() && !plan.IconPath.IsUnknown() && plan.IconPath.ValueString() != "" {
		iconPath := plan.IconPath.ValueString()
		content, err := os.ReadFile(iconPath) // #nosec G304 -- path comes from Terraform config
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read icon file",
				fmt.Sprintf("Could not read %s: %s", iconPath, err.Error()),
			)
			return
		}
		createReq.Icon = content
		createReq.IconName = filepath.Base(iconPath)
	}

	appStoreID, err := r.client.CreateWebApp(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Android web app",
			"Could not create web app: "+err.Error()+
				". This endpoint requires Fleet Premium with Android MDM enabled and configured.",
		)
		return
	}

	plan.AppStoreID = types.StringValue(appStoreID)
	plan.ID = types.StringValue(appStoreID)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read is intentionally a no-op.
//
// Fleet 4.90 has no endpoint that returns an Android web app: GET
// /software/web_apps answers 405 and no listing includes web apps that haven't
// been added to a team. The web app record lives in the Android enterprise,
// reachable only through Google's API. Refreshing state is therefore
// impossible, so we keep what Create recorded rather than guess.
func (r *softwareWebAppResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

// Update never runs in practice: every configurable attribute in the schema
// carries RequiresReplace, because Fleet has no endpoint to modify a web app.
// It is implemented as a state pass-through so that an unexpected call cannot
// corrupt state. Same pattern as bootstrapPackageResource.Update.
func (r *softwareWebAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan softwareWebAppResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete removes the resource from state. Fleet exposes no DELETE for web
// apps, so we warn rather than silently implying the app was removed.
func (r *softwareWebAppResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Android web app not deleted",
		"Fleet provides no API to delete an Android web app, so it remains registered in your Android "+
			"enterprise. It has been removed from Terraform state only. Delete it from the Fleet UI if you "+
			"no longer need it.",
	)
}
