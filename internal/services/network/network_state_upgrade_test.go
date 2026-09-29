package network

import (
	"context"
	"math/big"
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

func TestUpgradeNetworkKeepsClientSettings(t *testing.T) {
	resp := upgradeNetworkState(t, `{"name":"lan","description":"lab","dnslist":"8.8.8.8,1.1.1.1","domain":"example.local","rate_limit":100,"powerstate":"true"}`)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade: %v", resp.Diagnostics)
	}
	requireUpgradedString(t, resp.State.Raw, "description", "lab")
	requireUpgradedString(t, resp.State.Raw, "dnslist", "8.8.8.8,1.1.1.1")
	requireUpgradedString(t, resp.State.Raw, "domain", "example.local")
	got, isNull := upgradedInt64Attr(t, resp.State.Raw, "rate_limit")
	if isNull || got != 100 {
		t.Fatalf("rate_limit null=%v value=%d, want 100", isNull, got)
	}
}

func TestUpgradeNetworkNullsOmittedClientSettings(t *testing.T) {
	resp := upgradeNetworkState(t, `{"name":"lan","powerstate":"false"}`)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade: %v", resp.Diagnostics)
	}
	for _, name := range []string{"description", "dnslist", "domain"} {
		if _, isNull := upgradedStringAttr(t, resp.State.Raw, name); !isNull {
			t.Fatalf("%s should be null when the prior state omitted it", name)
		}
	}
	if _, isNull := upgradedInt64Attr(t, resp.State.Raw, "rate_limit"); !isNull {
		t.Fatal("rate_limit should be null when the prior state omitted it")
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

func upgradedStringAttr(t *testing.T, raw tftypes.Value, name string) (string, bool) {
	t.Helper()
	value := upgradedAttr(t, raw, name)
	if value.IsNull() {
		return "", true
	}
	var got string
	if err := value.As(&got); err != nil {
		t.Fatal(err)
	}
	return got, false
}

func requireUpgradedString(t *testing.T, raw tftypes.Value, name, want string) {
	t.Helper()
	got, isNull := upgradedStringAttr(t, raw, name)
	if isNull || got != want {
		t.Fatalf("%s null=%v value=%q, want %q", name, isNull, got, want)
	}
}

func upgradedInt64Attr(t *testing.T, raw tftypes.Value, name string) (int64, bool) {
	t.Helper()
	value := upgradedAttr(t, raw, name)
	if value.IsNull() {
		return 0, true
	}
	var num big.Float
	if err := value.As(&num); err != nil {
		t.Fatal(err)
	}
	got, _ := num.Int64()
	return got, false
}

func upgradedAttr(t *testing.T, raw tftypes.Value, name string) tftypes.Value {
	t.Helper()
	step, err := raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName(name))
	if err != nil {
		t.Fatal(err)
	}
	return step.(tftypes.Value)
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
