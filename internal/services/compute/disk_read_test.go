package compute

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestSortDrivesForState(t *testing.T) {
	drives := []vergeos.VMDrive{
		{Key: vergeos.FlexInt(4), Name: "data", OrderID: 1},
		{Key: vergeos.FlexInt(2), Name: "os", OrderID: 0},
		{Key: vergeos.FlexInt(9), Name: "extra", OrderID: 1},
		{Key: vergeos.FlexInt(3), Name: "data", OrderID: 1},
	}
	sortDrivesForState(drives)
	want := []int{2, 3, 4, 9}
	for i, id := range want {
		if drives[i].Key.Int() != id {
			t.Fatalf("drive %d id = %d, want %d", i, drives[i].Key.Int(), id)
		}
	}
}

func TestPreserveDiskConfigFields(t *testing.T) {
	prior := []*diskResourceModel{{
		Key:         types.StringValue("11"),
		Media:       types.StringValue("cdrom"),
		MediaSource: types.Int32Value(33),
	}}
	current := []*diskResourceModel{{
		Key:         types.StringValue("11"),
		Name:        types.StringValue("os"),
		Media:       types.StringNull(),
		MediaSource: types.Int32Null(),
	}}
	preserveDiskConfigFields(prior, current)
	if current[0].Media.ValueString() != "cdrom" || current[0].MediaSource.ValueInt32() != 33 {
		t.Fatalf("configured media was dropped: %#v", current[0])
	}

	imported := []*diskResourceModel{{
		Key:  types.StringValue("11"),
		Name: types.StringValue("os"),
	}}
	preserveDiskConfigFields(nil, imported)
	if !imported[0].Media.IsNull() || !imported[0].MediaSource.IsNull() {
		t.Fatal("import with no prior state should leave media unset")
	}
}
