package vm

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPairDisksRenameByKey(t *testing.T) {
	plan := []*diskResourceModel{{
		Key:  types.StringValue("46"),
		Name: types.StringValue("data2"),
	}}
	state := []*diskResourceModel{{
		Key:  types.StringValue("46"),
		Name: types.StringValue("data"),
	}}

	updates, creates, deletes := pairBlocks(plan, state, diskBlockID, diskBlockName)
	if len(creates) != 0 || len(deletes) != 0 || len(updates) != 1 {
		t.Fatalf("updates=%d creates=%d deletes=%d, want 1 update", len(updates), len(creates), len(deletes))
	}
	if updates[0].state.Key.ValueString() != "46" || updates[0].plan.Name.ValueString() != "data2" {
		t.Fatalf("rename was not paired to key 46: %#v", updates[0])
	}
	if !diskNeedsUpdate(updates[0].plan, updates[0].state) {
		t.Fatal("rename was not treated as an in-place update")
	}
	if diskNeedsRecreate(updates[0].plan, updates[0].state) {
		t.Fatal("rename was treated as a recreate")
	}
}

func TestPairDisksNameFallbackWhenKeyMissing(t *testing.T) {
	plan := []*diskResourceModel{{
		Name:        types.StringValue("os"),
		Description: types.StringValue("next"),
	}}
	state := []*diskResourceModel{{
		Key:         types.StringValue("11"),
		Name:        types.StringValue("os"),
		Description: types.StringValue("prev"),
	}}

	updates, creates, deletes := pairBlocks(plan, state, diskBlockID, diskBlockName)
	if len(creates) != 0 || len(deletes) != 0 || len(updates) != 1 {
		t.Fatalf("updates=%d creates=%d deletes=%d, want name fallback to the keyed drive", len(updates), len(creates), len(deletes))
	}
	if updates[0].state.Key.ValueString() != "11" {
		t.Fatalf("name fallback lost key %q", updates[0].state.Key.ValueString())
	}
}

func TestPairDisksKeyWinsOverName(t *testing.T) {
	// Two drives swap names. Key pairing must update each drive in place.
	plan := []*diskResourceModel{
		{Key: types.StringValue("1"), Name: types.StringValue("data2")},
		{Key: types.StringValue("2"), Name: types.StringValue("data")},
	}
	state := []*diskResourceModel{
		{Key: types.StringValue("1"), Name: types.StringValue("data")},
		{Key: types.StringValue("2"), Name: types.StringValue("data2")},
	}

	updates, creates, deletes := pairBlocks(plan, state, diskBlockID, diskBlockName)
	if len(creates) != 0 || len(deletes) != 0 || len(updates) != 2 {
		t.Fatalf("updates=%d creates=%d deletes=%d, want two in-place updates", len(updates), len(creates), len(deletes))
	}
	if updates[0].state.Key.ValueString() != "1" || updates[1].state.Key.ValueString() != "2" {
		t.Fatalf("keys were not preserved: %#v", updates)
	}
}

func TestPairDisksConfiguredKeyDoesNotFallBackToName(t *testing.T) {
	// A plan block that already has a key is a different drive, even when
	// the name matches. The schema marks a configured key change as replace
	// so this delete+create is not applied as a silent update.
	plan := []*diskResourceModel{{
		Key:  types.StringValue("99"),
		Name: types.StringValue("data"),
	}}
	state := []*diskResourceModel{{
		Key:  types.StringValue("46"),
		Name: types.StringValue("data"),
	}}

	updates, creates, deletes := pairBlocks(plan, state, diskBlockID, diskBlockName)
	if len(updates) != 0 || len(creates) != 1 || len(deletes) != 1 {
		t.Fatalf("updates=%d creates=%d deletes=%d, want a different key to be a new drive", len(updates), len(creates), len(deletes))
	}
}

func TestPairDisksAddAndRemove(t *testing.T) {
	plan := []*diskResourceModel{
		{Key: types.StringValue("1"), Name: types.StringValue("os")},
		{Name: types.StringValue("data")},
	}
	state := []*diskResourceModel{
		{Key: types.StringValue("1"), Name: types.StringValue("os")},
		{Key: types.StringValue("9"), Name: types.StringValue("old")},
	}

	updates, creates, deletes := pairBlocks(plan, state, diskBlockID, diskBlockName)
	if len(updates) != 1 || len(creates) != 1 || len(deletes) != 1 {
		t.Fatalf("updates=%d creates=%d deletes=%d", len(updates), len(creates), len(deletes))
	}
	if creates[0].Name.ValueString() != "data" || deletes[0].Key.ValueString() != "9" {
		t.Fatalf("create=%q delete key=%q", creates[0].Name.ValueString(), deletes[0].Key.ValueString())
	}
}

func TestPairDisksDuplicateNamesStayWithKeys(t *testing.T) {
	plan := []*diskResourceModel{
		{Key: types.StringValue("1"), Name: types.StringValue("data2")},
		{Key: types.StringValue("2"), Name: types.StringValue("data")},
	}
	state := []*diskResourceModel{
		{Key: types.StringValue("1"), Name: types.StringValue("data")},
		{Key: types.StringValue("2"), Name: types.StringValue("data")},
	}

	updates, creates, deletes := pairBlocks(plan, state, diskBlockID, diskBlockName)
	if len(creates) != 0 || len(deletes) != 0 || len(updates) != 2 {
		t.Fatalf("updates=%d creates=%d deletes=%d", len(updates), len(creates), len(deletes))
	}
	if updates[0].plan.Name.ValueString() != "data2" || updates[0].state.Key.ValueString() != "1" {
		t.Fatalf("first drive paired wrong: plan %q state key %q", updates[0].plan.Name.ValueString(), updates[0].state.Key.ValueString())
	}
}

func TestPairNICsRenameByID(t *testing.T) {
	plan := []*nicResourceModel{{
		Id:   types.StringValue("98"),
		Name: types.StringValue("lan"),
		MAC:  types.StringValue("52:54:00:11:22:33"),
	}}
	state := []*nicResourceModel{{
		Id:   types.StringValue("98"),
		Name: types.StringValue("nic0"),
		MAC:  types.StringValue("52:54:00:11:22:33"),
	}}

	updates, creates, deletes := pairBlocks(plan, state, nicBlockID, nicBlockName)
	if len(creates) != 0 || len(deletes) != 0 || len(updates) != 1 {
		t.Fatalf("updates=%d creates=%d deletes=%d, want 1 update", len(updates), len(creates), len(deletes))
	}
	if updates[0].state.Id.ValueString() != "98" {
		t.Fatalf("NIC id = %q, want 98", updates[0].state.Id.ValueString())
	}
	if !nicNeedsUpdate(updates[0].plan, updates[0].state) {
		t.Fatal("NIC rename was not treated as an in-place update")
	}
	body := jsonObject(t, nicUpdatePayload(updates[0].plan, updates[0].state))
	requireString(t, body, "name", "lan")
	requireAbsent(t, body, "macaddress")
}

func TestPairNICsNameFallbackWhenIDMissing(t *testing.T) {
	plan := []*nicResourceModel{{
		Name: types.StringValue("nic0"),
	}}
	state := []*nicResourceModel{{
		Id:   types.StringValue("98"),
		Name: types.StringValue("nic0"),
		MAC:  types.StringValue("52:54:00:11:22:33"),
	}}

	updates, creates, deletes := pairBlocks(plan, state, nicBlockID, nicBlockName)
	if len(creates) != 0 || len(deletes) != 0 || len(updates) != 1 {
		t.Fatalf("updates=%d creates=%d deletes=%d, want name fallback", len(updates), len(creates), len(deletes))
	}
	if updates[0].state.MAC.ValueString() != "52:54:00:11:22:33" {
		t.Fatal("name fallback dropped the MAC")
	}
}

func TestDiskUpdatePayloadRename(t *testing.T) {
	plan := &diskResourceModel{Name: types.StringValue("data2")}
	state := &diskResourceModel{Name: types.StringValue("data"), Key: types.StringValue("46")}
	body := jsonObject(t, diskUpdatePayload(plan, state))
	requireString(t, body, "name", "data2")
	requireAbsent(t, body, "$key")
	requireAbsent(t, body, "key")
}
