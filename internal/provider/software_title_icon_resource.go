package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &softwareTitleIconResource{}
	_ resource.ResourceWithConfigure   = &softwareTitleIconResource{}
	_ resource.ResourceWithImportState = &softwareTitleIconResource{}
	_ resource.ResourceWithModifyPlan  = &softwareTitleIconResource{}
)

// pngMagic is the 8-byte PNG signature. Fleet rejects non-PNG uploads with
// "icon must be a PNG image"; checking locally turns that round-trip into a
// plan-time diagnostic naming the offending file.
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// NewSoftwareTitleIconResource is the constructor registered with the provider.
func NewSoftwareTitleIconResource() resource.Resource {
	return &softwareTitleIconResource{}
}

// softwareTitleIconResource manages the custom icon Fleet 4.90+ shows for a
// software title in Fleet Desktop and the self-service catalog.
//
// The icon is a child of the (title, fleet) pair rather than of the title
// alone: the same title installed on two fleets carries two independent
// icons. Both scoping attributes are therefore Required and ForceNew — there
// is no "move this icon to another title" operation in Fleet, only
// upload-here / delete-there.
//
// Changing the image itself is an in-place update: Fleet's PUT overwrites
// whatever icon is stored, so there's no delete-then-upload window to worry
// about. That mirrors how fleetdm_software_custom_package treats a changed
// package_path.
type softwareTitleIconResource struct {
	client *fleetdm.Client
}

// softwareTitleIconResourceModel maps the resource schema data.
type softwareTitleIconResourceModel struct {
	ID         types.String `tfsdk:"id"`
	TitleID    types.Int64  `tfsdk:"title_id"`
	FleetID    types.Int64  `tfsdk:"fleet_id"`
	IconPath   types.String `tfsdk:"icon_path"`
	Filename   types.String `tfsdk:"filename"`
	HashSHA256 types.String `tfsdk:"hash_sha256"`
	IconURL    types.String `tfsdk:"icon_url"`
}

// Metadata returns the resource type name.
func (r *softwareTitleIconResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_title_icon"
}

// Schema defines the schema for the resource.
func (r *softwareTitleIconResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the custom icon for a software title in a fleet (Fleet 4.90+). The icon replaces the auto-derived one " +
			"in Fleet Desktop and the self-service catalog. The image must be a PNG between 120x120 and 1024x1024 pixels and " +
			"under 100KB, and the title must be installable (a custom package, Fleet-maintained app, VPP app, or in-house app) " +
			"in the referenced fleet. Fleet Premium only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The identifier for this resource, in the form '{title_id}:{fleet_id}'.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title_id": schema.Int64Attribute{
				Description: "The ID of the software title the icon belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"fleet_id": schema.Int64Attribute{
				Description: "The ID of the fleet (team) the icon applies to. Use 0 for the \"No team\" fleet. " +
					"Icons are scoped per fleet, so a title available on several fleets needs one resource per fleet.",
				Required: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"icon_path": schema.StringAttribute{
				Description: "Filesystem path to the PNG icon. The file is uploaded whenever its SHA256 differs from the icon " +
					"Fleet currently stores, which also means an icon replaced through the Fleet UI is detected as drift and " +
					"restored on the next apply.",
				Required: true,
			},
			"filename": schema.StringAttribute{
				Description: "The filename sent with the upload. Cosmetic — Fleet validates the image by content, not by " +
					"extension. When omitted it is computed once from the base name of `icon_path` at creation; a later " +
					"`icon_path` change keeps the original filename, so set `filename` explicitly if you want to control it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					// Reject "" outright. An empty string can't round-trip:
					// the upload needs a non-empty part filename, so the
					// provider would substitute the icon_path base name and
					// return a value the plan didn't contain ("Provider
					// produced inconsistent result after apply"). Omit the
					// attribute to get the derived default.
					stringvalidator.LengthAtLeast(1),
				},
			},
			"hash_sha256": schema.StringAttribute{
				Description: "The SHA256 hash of the icon. Computed at plan time from `icon_path`, and refreshed on read by " +
					"hashing the bytes Fleet serves back. Fleet exposes no icon hash of its own, so this is the provider's " +
					"drift signal.",
				Computed: true,
			},
			"icon_url": schema.StringAttribute{
				Description: "The Fleet-relative URL the icon is served from.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure injects the API client.
func (r *softwareTitleIconResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// ModifyPlan computes hash_sha256 and filename at plan time from the local
// file, so `terraform plan` shows the icon change before apply rather than
// surfacing it as a bare "known after apply".
//
// A local read failure during plan is a warning, not an error: the file may
// legitimately not exist yet on the machine running plan (generated by a
// sibling resource, fetched by a wrapper script) even though it will by the
// time apply runs. Create/Update read it again and do error there. This is
// the same trade-off fleetdm_software_custom_package makes for package_path.
func (r *softwareTitleIconResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Destroy plans carry a null plan; nothing to compute.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan softwareTitleIconResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.IconPath.IsNull() || plan.IconPath.IsUnknown() || plan.IconPath.ValueString() == "" {
		return
	}

	content, err := readIconFile(plan.IconPath.ValueString())
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Could not read icon at plan time",
			err.Error()+". The icon will be read again during apply; if the file is generated later in the run this warning is expected.",
		)
		return
	}

	plan.HashSHA256 = types.StringValue(sumHex(content))
	if plan.Filename.IsUnknown() {
		plan.Filename = types.StringValue(iconFilename(plan.IconPath, plan.Filename))
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// Create uploads the icon for the (title, fleet) pair.
func (r *softwareTitleIconResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan softwareTitleIconResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.upload(ctx, &plan, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state by hashing the icon Fleet serves back.
//
// Fleet publishes no icon hash, but GET on the icon route returns the
// uploaded bytes verbatim, so hashing the response gives genuine drift
// detection: an icon swapped out in the Fleet UI shows up as a hash_sha256
// diff and gets restored on the next apply. A missing icon (deleted out of
// band) drops the resource from state so the next plan re-creates it.
func (r *softwareTitleIconResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state softwareTitleIconResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	fleetID := int(state.FleetID.ValueInt64())

	content, err := r.client.GetTitleIcon(ctx, titleID, fleetID)
	if err != nil {
		if fleetdm.IsTitleIconAbsent(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading software title icon",
			fmt.Sprintf("Could not read the icon for software title %d in fleet %d: %s", titleID, fleetID, err.Error()),
		)
		return
	}
	if len(content) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(titleIconID(titleID, fleetID))
	state.HashSHA256 = types.StringValue(sumHex(content))

	// icon_url comes from the title, not the icon route. Read it best-effort:
	// a transient failure here shouldn't fail the refresh of an icon we just
	// successfully fetched, and the value is cosmetic.
	if title, titleErr := r.client.GetSoftwareTitle(ctx, titleID, &fleetID); titleErr == nil && title != nil {
		state.IconURL = types.StringValue(title.IconURL)
	} else if state.IconURL.IsNull() || state.IconURL.IsUnknown() {
		state.IconURL = types.StringValue("")
	}

	// filename is not recoverable from Fleet — it only ever appears in the
	// Content-Disposition of a download. Derive it from icon_path when there's
	// no prior value, and leave it empty on import, where icon_path is unset
	// too (filepath.Base("") would otherwise store a bare ".").
	if state.Filename.IsNull() || state.Filename.IsUnknown() {
		state.Filename = types.StringValue(iconFilename(state.IconPath, state.Filename))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update re-uploads the icon when the image content or the upload filename
// changed.
//
// Fleet's PUT overwrites the stored icon, so no DELETE is needed and there is
// never a moment where the title has no icon. When neither the bytes nor the
// filename moved, the upload is skipped so a no-op apply doesn't push an image
// at Fleet. The filename is included in that comparison because Fleet echoes
// it back in the icon download's Content-Disposition — it is observable state,
// not a provider-local label, so a change to it has to reach the server.
func (r *softwareTitleIconResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan softwareTitleIconResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state softwareTitleIconResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	content, err := readIconFile(plan.IconPath.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading icon", err.Error())
		return
	}
	sha := sumHex(content)
	filename := iconFilename(plan.IconPath, plan.Filename)

	if sha == state.HashSHA256.ValueString() && filename == state.Filename.ValueString() {
		// Nothing Fleet cares about moved: keep the stored icon and carry the
		// Computed attributes forward so the framework sees no unknowns in
		// the new state.
		plan.HashSHA256 = types.StringValue(sha)
		plan.Filename = types.StringValue(filename)
		plan.ID = state.ID
		if plan.IconURL.IsUnknown() {
			plan.IconURL = state.IconURL
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	if !r.uploadContent(ctx, &plan, content, sha, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the icon, restoring Fleet's auto-derived one.
//
// Fleet's DELETE is not idempotent — with no icon stored it answers 500
// "sql: no rows in result set" instead of a 404 — so an already-absent icon
// is treated as success. Without that, clearing an icon in the Fleet UI would
// wedge `terraform destroy`.
func (r *softwareTitleIconResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state softwareTitleIconResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	fleetID := int(state.FleetID.ValueInt64())

	if err := r.client.DeleteTitleIcon(ctx, titleID, fleetID); err != nil {
		// Covers both "no icon stored" shapes, and the 404 Fleet returns once
		// the whole title is gone — a deleted title takes its icon with it,
		// which is a converged destroy either way.
		if fleetdm.IsTitleIconAbsent(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting software title icon",
			fmt.Sprintf("Could not delete the icon for software title %d in fleet %d: %s", titleID, fleetID, err.Error()),
		)
	}
}

// ImportState imports an existing icon. Format: `title_id:fleet_id`, matching
// the ordering the sibling software resources use for their optional team
// suffix. Both halves are mandatory here because an icon has no meaning
// without its fleet scope — fleet_id 0 ("No team") must be spelled out.
//
// `icon_path` cannot be recovered from Fleet, so the user must add it to HCL
// after import; the following plan compares its hash against the imported
// icon's and only re-uploads on a genuine mismatch.
func (r *softwareTitleIconResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Import ID must be in format: title_id:fleet_id (got %q). Use fleet_id 0 for the \"No team\" fleet.", req.ID),
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
	fleetID, err := strconv.Atoi(parts[1])
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid fleet ID",
			fmt.Sprintf("Could not parse fleet ID %q: %s", parts[1], err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), titleIconID(titleID, fleetID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("title_id"), int64(titleID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fleet_id"), int64(fleetID))...)
}

// upload reads the icon from disk and pushes it to Fleet.
func (r *softwareTitleIconResource) upload(ctx context.Context, plan *softwareTitleIconResourceModel, diags *diag.Diagnostics) bool {
	content, err := readIconFile(plan.IconPath.ValueString())
	if err != nil {
		diags.AddError("Error reading icon", err.Error())
		return false
	}
	return r.uploadContent(ctx, plan, content, sumHex(content), diags)
}

// uploadContent pushes already-read bytes to Fleet and hydrates the plan's
// Computed attributes from the response.
func (r *softwareTitleIconResource) uploadContent(ctx context.Context, plan *softwareTitleIconResourceModel, content []byte, sha string, diags *diag.Diagnostics) bool {
	titleID := int(plan.TitleID.ValueInt64())
	fleetID := int(plan.FleetID.ValueInt64())

	filename := iconFilename(plan.IconPath, plan.Filename)

	iconURL, err := r.client.UploadTitleIcon(ctx, &fleetdm.UploadTitleIconRequest{
		TitleID:  titleID,
		FleetID:  fleetID,
		Icon:     content,
		Filename: filename,
	})
	if err != nil {
		diags.AddError(
			"Error uploading software title icon",
			fmt.Sprintf("Could not upload the icon for software title %d in fleet %d: %s", titleID, fleetID, err.Error()),
		)
		return false
	}

	plan.ID = types.StringValue(titleIconID(titleID, fleetID))
	plan.Filename = types.StringValue(filename)
	plan.HashSHA256 = types.StringValue(sha)
	plan.IconURL = types.StringValue(iconURL)
	return true
}

// titleIconID builds the resource's synthetic ID, which doubles as the import
// ID.
func titleIconID(titleID, fleetID int) string {
	return fmt.Sprintf("%d:%d", titleID, fleetID)
}

// iconFilename resolves the upload filename: an explicit `filename` wins,
// otherwise the base name of `icon_path`. Returns "" when neither is available
// (the import case), rather than the "." that filepath.Base("") would produce.
func iconFilename(iconPath, filename types.String) string {
	if !filename.IsNull() && !filename.IsUnknown() && filename.ValueString() != "" {
		return filename.ValueString()
	}
	if iconPath.IsNull() || iconPath.IsUnknown() || iconPath.ValueString() == "" {
		return ""
	}
	return filepath.Base(iconPath.ValueString())
}

// maxIconFileSize bounds what the provider will read from icon_path.
//
// Fleet caps icons at 100KB, so anything approaching this limit is already
// destined to be rejected server-side. The bound exists to stop a typo'd path
// — pointing at a disk image or a log file instead of an icon — from being
// buffered into memory in full before the PNG check runs. 1MiB leaves ample
// headroom over Fleet's cap while keeping a mistake cheap.
const maxIconFileSize = 1 << 20

// readIconFile reads a local icon, refusing implausibly large files and
// rejecting non-PNG content up front. Fleet's own error ("icon must be a PNG
// image") doesn't say which file was wrong, which is unhelpful when several
// icons apply in one run.
func readIconFile(iconPath string) ([]byte, error) {
	if iconPath == "" {
		return nil, fmt.Errorf("icon_path is empty")
	}

	// Size-check before reading: os.ReadFile would otherwise allocate the
	// whole file, so a mistyped path at a multi-gigabyte file takes the
	// provider down at plan time.
	info, err := os.Stat(iconPath)
	if err != nil {
		return nil, fmt.Errorf("could not read icon at %s: %w", iconPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("icon_path %s is a directory, not a PNG file", iconPath)
	}
	if info.Size() > maxIconFileSize {
		return nil, fmt.Errorf(
			"icon at %s is %d bytes, which exceeds the %d byte limit this provider will read "+
				"(Fleet itself rejects icons over 100KB) — check that icon_path points at the intended file",
			iconPath, info.Size(), maxIconFileSize)
	}

	content, err := os.ReadFile(iconPath) // #nosec G304 -- path comes from Terraform config
	if err != nil {
		return nil, fmt.Errorf("could not read icon at %s: %w", iconPath, err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("icon at %s is empty", iconPath)
	}
	if !bytes.HasPrefix(content, pngMagic) {
		return nil, fmt.Errorf("icon at %s is not a PNG file (Fleet only accepts PNG icons)", iconPath)
	}
	return content, nil
}

// sumHex returns the lowercase hex SHA256 of b.
func sumHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
