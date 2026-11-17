package compute

import (
	"context"
	"encoding/json"
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
	if schemaResp.Schema.Version != 4 {
		t.Fatalf("schema version = %d, want 4", schemaResp.Schema.Version)
	}
	if _, ok := schemaResp.Schema.Blocks["vergeio_drive"]; ok {
		t.Fatal("vergeio_drive should not be a block on vergeio_vm")
	}
	if _, ok := schemaResp.Schema.Blocks["vergeio_nic"]; ok {
		t.Fatal("vergeio_nic should not be a block on vergeio_vm")
	}
	boot, ok := schemaResp.Schema.Blocks["boot_disk"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("boot_disk should be a single nested block")
	}
	if _, ok := boot.Attributes["size"].(schema.Float64Attribute); !ok {
		t.Fatal("boot_disk.size should be a float64 attribute")
	}
	if _, ok := schemaResp.Schema.Attributes["powerstate"].(schema.BoolAttribute); !ok {
		t.Fatal("powerstate should be a bool attribute")
	}

	// Versions 0 and 1 stored ids as strings. Version 2 stored them as
	// numbers and preferred_tier as a string. Version 3 stored preferred_tier
	// as a number and still nested drives. All four upgrade to v4, which
	// drops the inline drive and NIC lists.
	prior := []byte(`{
		"cluster": "3",
		"preferred_node": "",
		"snapshot_profile": null,
		"vergeio_drive": [{"disksize": 8, "name": "os", "preferred_tier": "4"}],
		"vergeio_nic": [{"name": "lan", "vnet": "6"}]
	}`)

	upgraders := r.UpgradeState(ctx)
	for _, version := range []int64{0, 1, 2, 3} {
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

		if _, err := resp.State.Raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName("vergeio_drive")); err == nil {
			t.Fatalf("version %d still has vergeio_drive", version)
		}
		if _, err := resp.State.Raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName("vergeio_nic")); err == nil {
			t.Fatalf("version %d still has vergeio_nic", version)
		}
		if !mustNullAttr(t, resp.State.Raw, "boot_disk") {
			t.Fatalf("version %d boot_disk should be null so the upgrade does not claim a drive", version)
		}
	}
}

func TestUpgradeVMStatePreferredTierBlankAndNumber(t *testing.T) {
	prior := []byte(`{
		"cluster": 3,
		"vergeio_drive": [
			{"name": "os", "preferred_tier": "4"},
			{"name": "data", "preferred_tier": ""},
			{"name": "extra", "preferred_tier": 2}
		]
	}`)

	normalized, err := normalizeVMStateJSON(prior)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(normalized, &state); err != nil {
		t.Fatal(err)
	}
	var drives []map[string]json.RawMessage
	if err := json.Unmarshal(state["vergeio_drive"], &drives); err != nil {
		t.Fatal(err)
	}
	if len(drives) != 3 {
		t.Fatalf("drive count = %d, want 3", len(drives))
	}
	if string(drives[0]["preferred_tier"]) != "4" {
		t.Fatalf("os preferred_tier = %s, want 4", drives[0]["preferred_tier"])
	}
	if string(drives[1]["preferred_tier"]) != "null" {
		t.Fatalf("blank preferred_tier = %s, want null", drives[1]["preferred_tier"])
	}
	if string(drives[2]["preferred_tier"]) != "2" {
		t.Fatalf("extra preferred_tier = %s, want 2", drives[2]["preferred_tier"])
	}

	upgraded, err := upgradeVMStateJSON(prior)
	if err != nil {
		t.Fatal(err)
	}
	var dropped map[string]json.RawMessage
	if err := json.Unmarshal(upgraded, &dropped); err != nil {
		t.Fatal(err)
	}
	if _, ok := dropped["vergeio_drive"]; ok {
		t.Fatal("upgrade should drop vergeio_drive after the tier check")
	}
	if string(dropped["boot_disk"]) != "null" {
		t.Fatalf("boot_disk = %s, want null", dropped["boot_disk"])
	}
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
