package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

var (
	_ resource.Resource                   = &mdmEULAResource{}
	_ resource.ResourceWithConfigure      = &mdmEULAResource{}
	_ resource.ResourceWithImportState    = &mdmEULAResource{}
	_ resource.ResourceWithModifyPlan     = &mdmEULAResource{}
	_ resource.ResourceWithValidateConfig = &mdmEULAResource{}
)

// NewMDMEULAResource returns the fleetdm_mdm_eula resource.
func NewMDMEULAResource() resource.Resource {
	return &mdmEULAResource{}
}

type mdmEULAResource struct {
	client *fleetdm.Client
}

type mdmEULAResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Platform  types.String `tfsdk:"platform"`
	Source    types.String `tfsdk:"source"`
	Name      types.String `tfsdk:"name"`
	Sha256    types.String `tfsdk:"sha256"`
	Token     types.String `tfsdk:"token"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (r *mdmEULAResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mdm_eula"
}

func (r *mdmEULAResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the end user license agreement (EULA) Fleet shows during MDM enrollment, one per platform. " +
			"macOS (`darwin`) takes a PDF; Windows (`windows`, Fleet 4.94+) takes a Markdown (.md) file of up to 512 KB. " +
			"Requires Fleet Premium and the platform's MDM turned on. Fleet has no in-place update, so a change to the " +
			"file content or name replaces the EULA (delete, then upload). `create_before_destroy` is not supported: " +
			"Fleet allows only one EULA per platform.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The platform, which identifies the EULA.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform": schema.StringAttribute{
				Description: "The platform the EULA is for: `darwin` (a PDF) or `windows` (a Markdown file, Fleet 4.94+).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(fleetdm.EULAPlatformDarwin, fleetdm.EULAPlatformWindows),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source": schema.StringAttribute{
				Description: "Path to the local EULA file. The content is hashed at plan time and is not stored in state; " +
					"a content change (a different `sha256`) replaces the EULA. Moving the file without changing it does not.",
				Required: true,
			},
			"name": schema.StringAttribute{
				Description: "The file name Fleet stores and shows. Defaults to the base name of `source`. " +
					"Windows names must end in `.md`. It must be a bare file name of at most 255 characters " +
					"(no path separators or control characters), since Fleet strips those. Changing it replaces the EULA.",
				Optional: true,
				Computed: true,
			},
			"sha256": schema.StringAttribute{
				Description: "Lowercase hex SHA-256 of the EULA. Planned from the local file and refreshed from Fleet, so a change on either side replaces the EULA.",
				Computed:    true,
			},
			"token": schema.StringAttribute{
				Description: "The token Fleet uses in the EULA download URL. The download needs no other credential.",
				Computed:    true,
				Sensitive:   true,
			},
			"created_at": schema.StringAttribute{
				Description: "When the EULA was uploaded.",
				Computed:    true,
			},
		},
	}
}

func (r *mdmEULAResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// ValidateConfig checks a configured name; a defaulted one is checked in ModifyPlan.
func (r *mdmEULAResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config mdmEULAResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Name.IsNull() || config.Name.IsUnknown() || config.Platform.IsUnknown() {
		return
	}
	if msg := validateEULAName(config.Platform.ValueString(), config.Name.ValueString()); msg != "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid EULA file name", msg)
	}
}

// ModifyPlan hashes and validates the local file, defaults name, and plans a
// replacement when the content or name differs from Fleet's. Replacement, not
// an in-place update: Fleet keeps one EULA per platform, answers 409 to a
// second upload, and has no update route, so a change must delete then upload.
func (r *mdmEULAResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan, config mdmEULAResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state *mdmEULAResourceModel
	if !req.State.Raw.IsNull() {
		state = &mdmEULAResourceModel{}
		resp.Diagnostics.Append(req.State.Get(ctx, state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	platform := plan.Platform.ValueString()
	replace := state != nil && !plan.Platform.Equal(state.Platform)
	plan.ID = plan.Platform

	// name: the configured value, else the source's base name.
	switch {
	case !config.Name.IsNull():
		plan.Name = config.Name
	case plan.Source.IsUnknown():
		plan.Name = types.StringUnknown()
	default:
		plan.Name = types.StringValue(filepath.Base(plan.Source.ValueString()))
	}
	if !plan.Name.IsUnknown() && !plan.Platform.IsUnknown() && config.Name.IsNull() {
		if msg := validateEULAName(platform, plan.Name.ValueString()); msg != "" {
			resp.Diagnostics.AddAttributeError(path.Root("source"), "Invalid EULA file name", msg+" Set `name` to override the source's file name.")
			return
		}
	}
	if state != nil && !plan.Name.Equal(state.Name) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("name"))
		replace = true
	}

	// sha256: hash the local file.
	plan.Sha256 = types.StringUnknown()
	if !plan.Source.IsUnknown() && !plan.Platform.IsUnknown() {
		content, err := readEULAFile(plan.Source.ValueString())
		if err != nil {
			if state == nil {
				// The file may be produced during apply; Create reads it again.
				resp.Diagnostics.AddAttributeWarning(path.Root("source"), "Could not read EULA file", err.Error())
			} else {
				resp.Diagnostics.AddAttributeError(path.Root("source"), "Could not read EULA file", err.Error())
				return
			}
		} else {
			if msg := validateEULAContent(platform, content); msg != "" {
				resp.Diagnostics.AddAttributeError(path.Root("source"), "Invalid EULA file", msg)
				return
			}
			sum := sha256.Sum256(content)
			plan.Sha256 = types.StringValue(hex.EncodeToString(sum[:]))
		}
	}
	if state != nil && !plan.Sha256.Equal(state.Sha256) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("sha256"))
		replace = true
	}

	if replace || state == nil {
		plan.Token = types.StringUnknown()
		plan.CreatedAt = types.StringUnknown()
	} else {
		plan.Token = state.Token
		plan.CreatedAt = state.CreatedAt
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *mdmEULAResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mdmEULAResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	platform := plan.Platform.ValueString()

	content, err := readEULAFile(plan.Source.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("source"), "Could not read EULA file", err.Error())
		return
	}
	if msg := validateEULAName(platform, plan.Name.ValueString()); msg != "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid EULA file name", msg)
		return
	}
	if msg := validateEULAContent(platform, content); msg != "" {
		resp.Diagnostics.AddAttributeError(path.Root("source"), "Invalid EULA file", msg)
		return
	}
	sum := sha256.Sum256(content)
	localSha := hex.EncodeToString(sum[:])
	if !plan.Sha256.IsUnknown() && plan.Sha256.ValueString() != localSha {
		resp.Diagnostics.AddAttributeError(path.Root("source"), "EULA file changed during apply",
			"The file's SHA-256 differs from the one planned. Run terraform apply again.")
		return
	}

	if err := r.client.UploadEULA(ctx, platform, plan.Name.ValueString(), content); err != nil {
		detail := err.Error()
		switch {
		case isConflict(err):
			detail = fmt.Sprintf("Fleet already has a %s EULA, and it allows only one per platform. "+
				"Import it with `terraform import <address> %s`, or delete it in Fleet first.\n\n%s", platform, platform, detail)
		case platform == fleetdm.EULAPlatformWindows && isBadRequest(err) && r.serverBefore494(ctx):
			// Older Fleet ignores the platform and rejects the file as "not a PDF".
			detail += "\n\nFleet before 4.94 does not support Windows EULAs."
		}
		resp.Diagnostics.AddError("Error uploading EULA", detail)
		return
	}

	eula, err := r.client.GetEULAMetadata(ctx, platform)
	if err != nil {
		resp.Diagnostics.AddError("Error reading EULA after upload", err.Error())
		return
	}
	if eula.Sha256Hex() != localSha {
		resp.Diagnostics.AddError("EULA hash mismatch",
			fmt.Sprintf("Fleet reports SHA-256 %s for the uploaded EULA, but the file hashes to %s.", eula.Sha256Hex(), localSha))
		return
	}
	setEULAState(&plan, eula)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *mdmEULAResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mdmEULAResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	eula, err := r.client.GetEULAMetadata(ctx, state.Platform.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading EULA", err.Error())
		return
	}
	setEULAState(&state, eula)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update only records a new source path: every change Fleet would see
// (platform, name, content) is planned as a replacement.
func (r *mdmEULAResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan mdmEULAResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *mdmEULAResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mdmEULAResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteEULA(ctx, state.Token.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Error deleting EULA", err.Error())
	}
}

// ImportState takes the platform: terraform import fleetdm_mdm_eula.x windows.
func (r *mdmEULAResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID != fleetdm.EULAPlatformDarwin && req.ID != fleetdm.EULAPlatformWindows {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected the platform, %q or %q; got %q.", fleetdm.EULAPlatformDarwin, fleetdm.EULAPlatformWindows, req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("platform"), req.ID)...)
}

// serverBefore494 reports whether the server is known to run Fleet before 4.94.
func (r *mdmEULAResource) serverBefore494(ctx context.Context) bool {
	return fleetServerBefore(ctx, r.client, 4, 94)
}

func setEULAState(m *mdmEULAResourceModel, eula *fleetdm.EULA) {
	m.ID = m.Platform
	m.Name = types.StringValue(eula.Name)
	m.Sha256 = types.StringValue(eula.Sha256Hex())
	m.Token = types.StringValue(eula.Token)
	m.CreatedAt = types.StringValue(eula.CreatedAt)
}

// readEULAFile reads the file, refusing anything over Fleet's upload limit
// before buffering it.
func readEULAFile(p string) ([]byte, error) {
	f, err := os.Open(p) // #nosec G304 -- the path is the practitioner's own configuration
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	content, err := io.ReadAll(io.LimitReader(f, fleetdm.MaxEULASize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > fleetdm.MaxEULASize {
		return nil, fmt.Errorf("%s is larger than Fleet's %d MiB EULA limit", p, fleetdm.MaxEULASize>>20)
	}
	return content, nil
}

// validateEULAName mirrors Fleet's checks: it stores SanitizeEULAFileName's
// result, so a name that sanitizing changes would drift on every refresh.
func validateEULAName(platform, name string) string {
	if name == "" || name == "." || name == ".." {
		return "The file name must not be empty."
	}
	if utf8.RuneCountInString(name) > fleetdm.MaxNameLength {
		return fmt.Sprintf("The file name must be %d characters or fewer.", fleetdm.MaxNameLength)
	}
	if strings.ContainsAny(name, `/\`) {
		return "The file name must not contain a path separator."
	}
	if strings.IndexFunc(name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 {
		return "The file name must not contain control or formatting characters."
	}
	if strings.TrimSpace(name) != name {
		return "The file name must not start or end with whitespace."
	}
	if platform == fleetdm.EULAPlatformWindows && !strings.EqualFold(filepath.Ext(name), ".md") {
		return "A Windows EULA must be a Markdown (.md) file."
	}
	return ""
}

// validateEULAContent mirrors the cheap checks Fleet makes; Fleet's Markdown
// rendering limits are left to Fleet.
func validateEULAContent(platform string, content []byte) string {
	if platform == fleetdm.EULAPlatformWindows {
		if int64(len(content)) > fleetdm.MaxWindowsEULASize {
			return fmt.Sprintf("A Windows EULA must be %d KB or smaller.", fleetdm.MaxWindowsEULASize>>10)
		}
		if len(bytes.TrimSpace(content)) == 0 {
			return "The file is empty."
		}
		if !utf8.Valid(content) {
			return "A Windows EULA must be UTF-8 text."
		}
		return ""
	}
	if !bytes.HasPrefix(content, []byte("%PDF")) {
		return "A macOS EULA must be a PDF."
	}
	return ""
}
