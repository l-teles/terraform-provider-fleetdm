package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
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

var (
	_ resource.Resource                = &SoftwarePatchDeadlineSettingsResource{}
	_ resource.ResourceWithImportState = &SoftwarePatchDeadlineSettingsResource{}
)

// patchDeadlineTimePattern is stricter than Fleet, which also accepts "9:00"
// but stores it as "09:00"; the stored value would then drift from config.
var patchDeadlineTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// NewSoftwarePatchDeadlineSettingsResource creates a new patch deadline settings resource.
func NewSoftwarePatchDeadlineSettingsResource() resource.Resource {
	return &SoftwarePatchDeadlineSettingsResource{}
}

// SoftwarePatchDeadlineSettingsResource manages a fleet's patch deadline
// settings. Every fleet always has settings, so destroy resets them to Fleet's
// defaults.
type SoftwarePatchDeadlineSettingsResource struct {
	client *fleetdm.Client
}

// SoftwarePatchDeadlineSettingsResourceModel describes the resource data model.
type SoftwarePatchDeadlineSettingsResourceModel struct {
	ID      types.String `tfsdk:"id"`
	FleetID types.Int64  `tfsdk:"fleet_id"`
	Days    types.Set    `tfsdk:"days"`
	Time    types.String `tfsdk:"time"`
}

func (r *SoftwarePatchDeadlineSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_patch_deadline_settings"
}

func (r *SoftwarePatchDeadlineSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a fleet's software patch deadline settings: the weekdays and host-local time when patch deadlines fall.\n\n" +
			"Requires Fleet Premium and Fleet >= 4.95.\n\n" +
			"~> Every fleet always has patch deadline settings. Destroying this resource resets them to Fleet's default (`tuesday` at `17:00`) rather than removing them. If the fleet itself has already been deleted, destroy succeeds without doing anything.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The fleet ID as a string (`\"0\"` for Unassigned).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"fleet_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The ID of the fleet (team) the settings apply to. Use `0` for hosts that are not assigned to a fleet (Unassigned). Changing this forces a new resource to be created.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"days": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "The weekdays when patch deadlines fall, as lowercase names: `monday`, `tuesday`, `wednesday`, `thursday`, `friday`, `saturday` or `sunday`. At least one day is required.",
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf(fleetdm.PatchDeadlineDays...)),
				},
			},
			"time": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The host-local time when patch deadlines fall, in 24-hour `HH:MM` format with a two-digit hour (for example `\"09:00\"` or `\"17:30\"`).",
				Validators: []validator.String{
					stringvalidator.RegexMatches(patchDeadlineTimePattern, `must be in 24-hour HH:MM format with a two-digit hour, for example "09:00" or "17:30"`),
				},
			},
		},
	}
}

func (r *SoftwarePatchDeadlineSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

func (r *SoftwarePatchDeadlineSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SoftwarePatchDeadlineSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, "Creating")
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SoftwarePatchDeadlineSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SoftwarePatchDeadlineSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	settings, err := r.client.GetPatchDeadlineSettings(ctx, data.FleetID.ValueInt64())
	if err != nil {
		if patchDeadlineSettingsUnsupported(ctx, r.client, err) {
			resp.Diagnostics.AddError("Patch Deadline Settings Need Fleet 4.95", patchDeadlineSettingsUnsupportedDetail)
			return
		}
		// The fleet was deleted, taking its settings with it.
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading FleetDM Patch Deadline Settings", err.Error())
		return
	}

	resp.Diagnostics.Append(mapPatchDeadlineSettingsToModel(ctx, settings, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SoftwarePatchDeadlineSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SoftwarePatchDeadlineSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &data, &resp.Diagnostics, "Updating")
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete resets the settings to Fleet's defaults.
func (r *SoftwarePatchDeadlineSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SoftwarePatchDeadlineSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	fleetID := data.FleetID.ValueInt64()
	if _, err := r.client.UpdatePatchDeadlineSettings(ctx, fleetdm.DefaultPatchDeadlineSettings(fleetID)); err != nil {
		// Older Fleet has no such settings, so there is nothing to reset.
		if patchDeadlineSettingsUnsupported(ctx, r.client, err) {
			resp.Diagnostics.AddWarning("Patch Deadline Settings Need Fleet 4.95",
				"This Fleet server has no patch deadline settings, so there was nothing to reset. The resource was removed from state.")
			return
		}
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Patch Deadline Settings",
			fmt.Sprintf("Unable to reset patch deadline settings for fleet %d to the defaults: %s", fleetID, err),
		)
	}
}

// ImportState takes the fleet ID ("0" for Unassigned).
func (r *SoftwarePatchDeadlineSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fleetID, ok := parseIDFromString(req.ID, "Patch Deadline Settings Fleet", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fleet_id"), fleetID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(fleetID, 10))...)
}

// apply PATCHes the planned settings and reads them back into data.
func (r *SoftwarePatchDeadlineSettingsResource) apply(ctx context.Context, data *SoftwarePatchDeadlineSettingsResourceModel, diags *diag.Diagnostics, verb string) {
	fleetID := data.FleetID.ValueInt64()

	var days []string
	diags.Append(data.Days.ElementsAs(ctx, &days, false)...)
	if diags.HasError() {
		return
	}

	title := fmt.Sprintf("Error %s FleetDM Patch Deadline Settings", verb)
	if _, err := r.client.UpdatePatchDeadlineSettings(ctx, fleetdm.PatchDeadlineSettings{
		FleetID: fleetID,
		Days:    days,
		Time:    data.Time.ValueString(),
	}); err != nil {
		if patchDeadlineSettingsUnsupported(ctx, r.client, err) {
			diags.AddError("Patch Deadline Settings Need Fleet 4.95", patchDeadlineSettingsUnsupportedDetail)
			return
		}
		diags.AddError(title, fmt.Sprintf("Unable to update patch deadline settings for fleet %d: %s", fleetID, err))
		return
	}

	settings, err := r.client.GetPatchDeadlineSettings(ctx, fleetID)
	if err != nil {
		diags.AddError(title, fmt.Sprintf("Unable to read back patch deadline settings for fleet %d: %s", fleetID, err))
		return
	}
	diags.Append(mapPatchDeadlineSettingsToModel(ctx, settings, data)...)
}

const patchDeadlineSettingsUnsupportedDetail = "This Fleet server has no patch deadline settings endpoint. " +
	"fleetdm_software_patch_deadline_settings requires Fleet Premium 4.95.0 or later."

// patchDeadlineSettingsUnsupported reports whether a 404 came from a Fleet
// older than 4.95, which has no such endpoint, rather than from a deleted
// fleet. An unreadable version is treated as a deleted fleet.
func patchDeadlineSettingsUnsupported(ctx context.Context, client *fleetdm.Client, err error) bool {
	if !isNotFound(err) {
		return false
	}
	before, known := fleetServerVersionBefore(ctx, client, 4, 95)
	return known && before
}

// mapPatchDeadlineSettingsToModel copies the API settings onto the model.
func mapPatchDeadlineSettingsToModel(ctx context.Context, settings *fleetdm.PatchDeadlineSettings, data *SoftwarePatchDeadlineSettingsResourceModel) diag.Diagnostics {
	days, diags := types.SetValueFrom(ctx, types.StringType, settings.Days)
	if diags.HasError() {
		return diags
	}
	data.ID = types.StringValue(strconv.FormatInt(settings.FleetID, 10))
	data.FleetID = types.Int64Value(settings.FleetID)
	data.Days = days
	data.Time = types.StringValue(settings.Time)
	return diags
}
