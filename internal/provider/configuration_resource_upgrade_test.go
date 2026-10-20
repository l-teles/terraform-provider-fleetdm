package provider

import (
	"context"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// upgradeConfigurationV0 runs the version 0 upgrader on a raw JSON state and
// returns the upgraded agent_options and org_name.
func upgradeConfigurationV0(t *testing.T, rawJSON string) (jsontypes.Normalized, types.String) {
	t.Helper()
	ctx := context.Background()

	r := &ConfigurationResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	upgrader, ok := r.UpgradeState(ctx)[0]
	if !ok {
		t.Fatal("expected a state upgrader for version 0")
	}

	resp := &resource.UpgradeStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{
		RawState: &tfprotov6.RawState{JSON: []byte(rawJSON)},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade failed: %v", resp.Diagnostics)
	}

	var agentOptions jsontypes.Normalized
	var orgName types.String
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("agent_options"), &agentOptions)...)
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("org_name"), &orgName)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("reading upgraded state failed: %v", resp.Diagnostics)
	}
	return agentOptions, orgName
}

func TestConfigurationUpgradeStateV0_emptyAgentOptionsBecomesNull(t *testing.T) {
	agentOptions, orgName := upgradeConfigurationV0(t, `{"id":"configuration","org_name":"Example","agent_options":""}`)

	if !agentOptions.IsNull() {
		t.Errorf(`expected "" agent_options to become null, got %q`, agentOptions.ValueString())
	}
	if orgName.ValueString() != "Example" {
		t.Errorf("expected org_name to be carried over, got %q", orgName.ValueString())
	}
}

func TestConfigurationUpgradeStateV0_keepsAgentOptions(t *testing.T) {
	const options = `{"config":{"options":{"distributed_interval":10}}}`
	agentOptions, _ := upgradeConfigurationV0(t, `{"id":"configuration","agent_options":`+strconv.Quote(options)+`}`)

	if agentOptions.ValueString() != options {
		t.Errorf("expected agent_options to be unchanged, got %q", agentOptions.ValueString())
	}
}
