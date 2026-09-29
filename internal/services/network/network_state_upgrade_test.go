package network

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestNetworkSchemaPowerStateIsBool(t *testing.T) {
	ctx := context.Background()
	r := &NetworkResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Schema.Version != 1 {
		t.Fatalf("schema version = %d, want 1", schemaResp.Schema.Version)
	}
	if _, ok := schemaResp.Schema.Attributes["powerstate"].(schema.BoolAttribute); !ok {
		t.Fatal("powerstate should be a bool attribute")
	}

	upgrader, ok := r.UpgradeState(ctx)[0]
	if !ok || upgrader.PriorSchema == nil {
		t.Fatal("version 0 upgrader should declare a prior schema")
	}
	if _, ok := upgrader.PriorSchema.Attributes["powerstate"].(schema.StringAttribute); !ok {
		t.Fatal("version 0 powerstate should be a string attribute")
	}
}

func TestUpgradeNetworkPowerState(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		isNull bool
		want   bool
	}{
		{name: "true string", raw: `{"powerstate": "true", "name": "lan", "enabled": true}`, want: true},
		{name: "false string", raw: `{"powerstate": "false", "name": "lan"}`},
		{name: "True mixed case", raw: `{"powerstate": "True"}`, want: true},
		{name: "FALSE mixed case", raw: `{"powerstate": "FALSE"}`},
		{name: "running", raw: `{"powerstate": "running"}`, want: true},
		{name: "stopped", raw: `{"powerstate": "stopped"}`},
		{name: "null", raw: `{"powerstate": null}`, isNull: true},
		{name: "blank", raw: `{"powerstate": ""}`, isNull: true},
		{name: "omitted", raw: `{"name": "lan"}`, isNull: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := upgradeNetworkState(t, tc.raw)
			if resp.Diagnostics.HasError() {
				t.Fatalf("upgrade: %v", resp.Diagnostics)
			}
			got, isNull := networkPowerAttr(t, resp.State.Raw)
			if isNull != tc.isNull {
				t.Fatalf("powerstate null = %v, want %v", isNull, tc.isNull)
			}
			if !isNull && got != tc.want {
				t.Fatalf("powerstate = %v, want %v", got, tc.want)
			}
			if tc.name == "true string" {
				name, err := resp.State.Raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName("name"))
				if err != nil {
					t.Fatal(err)
				}
				var gotName string
				if err := name.(tftypes.Value).As(&gotName); err != nil {
					t.Fatal(err)
				}
				if gotName != "lan" {
					t.Fatalf("name = %q, want lan", gotName)
				}
			}
		})
	}
}

func TestUpgradeNetworkPowerStateRejectsUnknown(t *testing.T) {
	resp := upgradeNetworkState(t, `{"powerstate":"yes"}`)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error converting an unknown powerstate string")
	}
}

// upgradeNetworkState populates prior state the way the framework does when
// PriorSchema is set, then runs the version 0 upgrader.
func upgradeNetworkState(t *testing.T, raw string) *resource.UpgradeStateResponse {
	t.Helper()
	ctx := context.Background()
	r := &NetworkResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	upgrader, ok := r.UpgradeState(ctx)[0]
	if !ok || upgrader.PriorSchema == nil {
		t.Fatal("missing prior schema for version 0")
	}

	rawState := &tfprotov6.RawState{JSON: []byte(raw)}
	priorValue, err := rawState.UnmarshalWithOpts(upgrader.PriorSchema.Type().TerraformType(ctx), tfprotov6.UnmarshalOpts{
		ValueFromJSONOpts: tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true},
	})
	if err != nil {
		t.Fatalf("unmarshal prior state: %v", err)
	}

	resp := &resource.UpgradeStateResponse{}
	resp.State.Schema = schemaResp.Schema
	upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{
		RawState: rawState,
		State: &tfsdk.State{
			Schema: *upgrader.PriorSchema,
			Raw:    priorValue,
		},
	}, resp)
	return resp
}

func networkPowerAttr(t *testing.T, raw tftypes.Value) (bool, bool) {
	t.Helper()
	step, err := raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName("powerstate"))
	if err != nil {
		t.Fatal(err)
	}
	value := step.(tftypes.Value)
	if value.IsNull() {
		return false, true
	}
	var on bool
	if err := value.As(&on); err != nil {
		t.Fatal(err)
	}
	return on, false
}
