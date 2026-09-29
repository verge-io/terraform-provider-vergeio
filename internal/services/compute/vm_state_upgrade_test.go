package compute

import (
	"context"
	"math/big"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestUpgradeVMStateNumericIDsAndDiskSize(t *testing.T) {
	ctx := context.Background()
	r := &VMResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Schema.Version != 2 {
		t.Fatalf("schema version = %d, want 2", schemaResp.Schema.Version)
	}

	// Version 0 / 1 state: string ids, integer disksize, no on_power_loss.
	prior := []byte(`{
		"cluster": "3",
		"preferred_node": "",
		"snapshot_profile": null,
		"vergeio_drive": [{"disksize": 8, "name": "os"}]
	}`)

	upgraders := r.UpgradeState(ctx)
	for _, version := range []int64{0, 1} {
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
	}
}

func TestNormalizeVMStateJSONRejectsNonNumericID(t *testing.T) {
	_, err := normalizeVMStateJSON([]byte(`{"cluster":"abc"}`))
	if err == nil {
		t.Fatal("expected error converting a non-numeric cluster id")
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
