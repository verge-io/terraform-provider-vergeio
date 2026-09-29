package network

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
}

func TestUpgradeNetworkPowerState(t *testing.T) {
	ctx := context.Background()
	r := &NetworkResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	upgrader, ok := r.UpgradeState(ctx)[0]
	if !ok {
		t.Fatal("missing upgrader for version 0")
	}

	cases := []struct {
		name   string
		raw    string
		isNull bool
		want   bool
	}{
		{name: "true string", raw: `"powerstate": "true"`, want: true},
		{name: "false string", raw: `"powerstate": "false"`},
		{name: "True mixed case", raw: `"powerstate": "True"`, want: true},
		{name: "FALSE mixed case", raw: `"powerstate": "FALSE"`},
		{name: "running", raw: `"powerstate": "running"`, want: true},
		{name: "stopped", raw: `"powerstate": "stopped"`},
		{name: "already bool", raw: `"powerstate": false`},
		{name: "null", raw: `"powerstate": null`, isNull: true},
		{name: "blank", raw: `"powerstate": ""`, isNull: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &resource.UpgradeStateResponse{}
			resp.State.Schema = schemaResp.Schema
			upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{
				RawState: &tfprotov6.RawState{JSON: []byte("{" + tc.raw + "}")},
			}, resp)
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
		})
	}
}

func TestNormalizeNetworkStateJSONRejectsUnknownPowerState(t *testing.T) {
	_, err := normalizeNetworkStateJSON([]byte(`{"powerstate":"yes"}`))
	if err == nil {
		t.Fatal("expected error converting an unknown powerstate string")
	}
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
