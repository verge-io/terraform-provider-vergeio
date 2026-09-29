package compute

import (
	"context"
	"math/big"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestUpgradeVMStateNumericIDsAndDiskSize(t *testing.T) {
	ctx := context.Background()
	r := &VMResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Schema.Version != 3 {
		t.Fatalf("schema version = %d, want 3", schemaResp.Schema.Version)
	}
	driveBlock, ok := schemaResp.Schema.Blocks["vergeio_drive"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("vergeio_drive should be a list nested block")
	}
	if _, ok := driveBlock.NestedObject.Attributes["preferred_tier"].(schema.Int32Attribute); !ok {
		t.Fatal("preferred_tier should be an int32 attribute")
	}
	if _, ok := schemaResp.Schema.Attributes["powerstate"].(schema.BoolAttribute); !ok {
		t.Fatal("powerstate should be a bool attribute")
	}

	// Versions 0 and 1 stored ids as strings. Version 2 stored them as
	// numbers and preferred_tier as a string. All three upgrade to v3.
	prior := []byte(`{
		"cluster": "3",
		"preferred_node": "",
		"snapshot_profile": null,
		"vergeio_drive": [{"disksize": 8, "name": "os", "preferred_tier": "4"}]
	}`)

	upgraders := r.UpgradeState(ctx)
	for _, version := range []int64{0, 1, 2} {
		upgrader, ok := upgraders[version]
		if !ok {
			t.Fatalf("missing upgrader for version %d", version)
		}

		resp := &resource.UpgradeStateResponse{}
		resp.State.Schema = schemaResp.Schema
		upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{
			RawState: &tfprotov6.RawState{JSON: prior},
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("version %d upgrade: %v", version, resp.Diagnostics)
		}

		cluster := mustNumberAttr(t, resp.State.Raw, "cluster")
		if cluster.Cmp(big.NewFloat(3)) != 0 {
			t.Fatalf("version %d cluster = %s, want 3", version, cluster.String())
		}
		if !mustNullAttr(t, resp.State.Raw, "preferred_node") {
			t.Fatalf("version %d preferred_node should be null when the prior value was blank", version)
		}
		if !mustNullAttr(t, resp.State.Raw, "snapshot_profile") {
			t.Fatalf("version %d snapshot_profile should stay null", version)
		}

		drive, err := resp.State.Raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName("vergeio_drive"))
		if err != nil {
			t.Fatal(err)
		}
		driveVal := drive.(tftypes.Value)
		elem, err := driveVal.ApplyTerraform5AttributePathStep(tftypes.ElementKeyInt(0))
		if err != nil {
			t.Fatal(err)
		}
		sizeVal, err := elem.(tftypes.Value).ApplyTerraform5AttributePathStep(tftypes.AttributeName("disksize"))
		if err != nil {
			t.Fatal(err)
		}
		size := mustNumber(t, sizeVal.(tftypes.Value))
		if size.Cmp(big.NewFloat(8)) != 0 {
			t.Fatalf("version %d disksize = %s, want 8", version, size.String())
		}
		tierVal, err := elem.(tftypes.Value).ApplyTerraform5AttributePathStep(tftypes.AttributeName("preferred_tier"))
		if err != nil {
			t.Fatal(err)
		}
		tier := mustNumber(t, tierVal.(tftypes.Value))
		if tier.Cmp(big.NewFloat(4)) != 0 {
			t.Fatalf("version %d preferred_tier = %s, want 4", version, tier.String())
		}
	}
}

func TestUpgradeVMStatePreferredTierBlankAndNumber(t *testing.T) {
	ctx := context.Background()
	r := &VMResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	prior := []byte(`{
		"cluster": 3,
		"vergeio_drive": [
			{"name": "os", "preferred_tier": "4"},
			{"name": "data", "preferred_tier": ""},
			{"name": "extra", "preferred_tier": 2}
		]
	}`)

	upgrader, ok := r.UpgradeState(ctx)[2]
	if !ok {
		t.Fatal("missing upgrader for version 2")
	}
	resp := &resource.UpgradeStateResponse{}
	resp.State.Schema = schemaResp.Schema
	upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{
		RawState: &tfprotov6.RawState{JSON: prior},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade: %v", resp.Diagnostics)
	}

	tiers := driveTierValues(t, resp.State.Raw)
	if len(tiers) != 3 {
		t.Fatalf("drive count = %d, want 3", len(tiers))
	}
	if tiers[0] == nil || tiers[0].Cmp(big.NewFloat(4)) != 0 {
		t.Fatalf("os preferred_tier = %v, want 4", tiers[0])
	}
	if tiers[1] != nil {
		t.Fatalf("blank preferred_tier = %s, want null", tiers[1].String())
	}
	if tiers[2] == nil || tiers[2].Cmp(big.NewFloat(2)) != 0 {
		t.Fatalf("extra preferred_tier = %v, want 2", tiers[2])
	}
}

func driveTierValues(t *testing.T, raw tftypes.Value) []*big.Float {
	t.Helper()
	drive, err := raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName("vergeio_drive"))
	if err != nil {
		t.Fatal(err)
	}
	var elems []tftypes.Value
	if err := drive.(tftypes.Value).As(&elems); err != nil {
		t.Fatal(err)
	}
	tiers := make([]*big.Float, len(elems))
	for i, elem := range elems {
		tierVal, err := elem.ApplyTerraform5AttributePathStep(tftypes.AttributeName("preferred_tier"))
		if err != nil {
			t.Fatal(err)
		}
		value := tierVal.(tftypes.Value)
		if value.IsNull() {
			continue
		}
		tiers[i] = mustNumber(t, value)
	}
	return tiers
}

func TestNormalizeVMStateJSONRejectsNonNumericID(t *testing.T) {
	_, err := normalizeVMStateJSON([]byte(`{"cluster":"abc"}`))
	if err == nil {
		t.Fatal("expected error converting a non-numeric cluster id")
	}
}

func TestNormalizeVMStateJSONRejectsNonNumericTier(t *testing.T) {
	_, err := normalizeVMStateJSON([]byte(`{"vergeio_drive":[{"preferred_tier":"tier"}]}`))
	if err == nil {
		t.Fatal("expected error converting a non-numeric preferred_tier")
	}
}

func mustNumberAttr(t *testing.T, raw tftypes.Value, name string) *big.Float {
	t.Helper()
	step, err := raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName(name))
	if err != nil {
		t.Fatal(err)
	}
	return mustNumber(t, step.(tftypes.Value))
}

func mustNullAttr(t *testing.T, raw tftypes.Value, name string) bool {
	t.Helper()
	step, err := raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName(name))
	if err != nil {
		t.Fatal(err)
	}
	return step.(tftypes.Value).IsNull()
}

func mustNumber(t *testing.T, v tftypes.Value) *big.Float {
	t.Helper()
	var n big.Float
	if err := v.As(&n); err != nil {
		t.Fatal(err)
	}
	return &n
}
