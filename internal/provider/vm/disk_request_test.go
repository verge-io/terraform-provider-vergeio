package vm

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDiskCreatePayloadKeepsFalseZeroAndEmpty(t *testing.T) {
	data := &diskResourceModel{
		Name:        types.StringValue("disk0"),
		Description: types.StringValue(""),
		Enabled:     types.BoolValue(false),
		ReadOnly:    types.BoolValue(false),
		OrderId:     types.Int32Value(0),
		DiskSize:    types.Float64Value(0),
		Serial:      types.StringNull(),
	}

	body := jsonObject(t, diskCreatePayload(data))
	requireBool(t, body, "enabled", false)
	requireBool(t, body, "readonly", false)
	requireNumber(t, body, "orderid", 0)
	requireNumber(t, body, "disksize", 0)
	requireString(t, body, "description", "")
	requireAbsent(t, body, "serial")
}

func TestDiskUpdatePayloadKeepsFalseZeroAndEmpty(t *testing.T) {
	plan := &diskResourceModel{
		Name:        types.StringValue("disk0"),
		Description: types.StringValue(""),
		Enabled:     types.BoolValue(false),
		OrderId:     types.Int32Value(0),
		DiskSize:    types.Float64Value(0),
		Interface:   types.StringValue("virtio"),
	}
	state := &diskResourceModel{
		Name:        types.StringValue("disk0"),
		Description: types.StringValue("before"),
		Enabled:     types.BoolValue(true),
		OrderId:     types.Int32Value(1),
		DiskSize:    types.Float64Value(10),
		Interface:   types.StringValue("virtio"),
	}

	body := jsonObject(t, diskUpdatePayload(plan, state))
	requireBool(t, body, "enabled", false)
	requireNumber(t, body, "orderid", 0)
	requireNumber(t, body, "disksize", 0)
	requireString(t, body, "description", "")
	requireAbsent(t, body, "name")
	requireAbsent(t, body, "interface")
}

func TestDiskUpdatePayloadOmitsDiskSizeUnlessSizeChanged(t *testing.T) {
	state := &diskResourceModel{
		Name:        types.StringValue("data2"),
		Interface:   types.StringValue("ide"),
		Description: types.StringValue("data"),
		DiskSize:    types.Float64Value(1),
		Enabled:     types.BoolValue(true),
		OrderId:     types.Int32Value(1),
	}

	equal := jsonObject(t, diskUpdatePayload(&diskResourceModel{
		Name:        state.Name,
		Interface:   state.Interface,
		Description: state.Description,
		DiskSize:    types.Float64Value(1),
		Enabled:     state.Enabled,
		OrderId:     state.OrderId,
	}, state))
	if len(equal) != 0 {
		t.Fatalf("unchanged drive payload = %#v, want empty", equal)
	}

	unknownSize := jsonObject(t, diskUpdatePayload(&diskResourceModel{
		Description: types.StringValue("note"),
		DiskSize:    types.Float64Unknown(),
		Interface:   types.StringUnknown(),
		Enabled:     types.BoolUnknown(),
	}, state))
	requireString(t, unknownSize, "description", "note")
	requireAbsent(t, unknownSize, "disksize")
	requireAbsent(t, unknownSize, "interface")
	requireAbsent(t, unknownSize, "enabled")

	nullSize := jsonObject(t, diskUpdatePayload(&diskResourceModel{
		Description: types.StringValue("note"),
		DiskSize:    types.Float64Null(),
	}, state))
	requireString(t, nullSize, "description", "note")
	requireAbsent(t, nullSize, "disksize")

	// 1.0005 GB is under the resize threshold. It must not be sent.
	noise := jsonObject(t, diskUpdatePayload(&diskResourceModel{
		DiskSize: types.Float64Value(1.0005),
	}, state))
	requireAbsent(t, noise, "disksize")

	resized := jsonObject(t, diskUpdatePayload(&diskResourceModel{
		Name:      state.Name,
		Interface: types.StringUnknown(),
		DiskSize:  types.Float64Value(2),
		Enabled:   types.BoolUnknown(),
	}, state))
	requireNumber(t, resized, "disksize", 2*1024*1024*1024)
	requireAbsent(t, resized, "name")
	requireAbsent(t, resized, "interface")
	requireAbsent(t, resized, "enabled")
}

func TestDiskUpdatePayloadOmitsUnsetEnabled(t *testing.T) {
	plan := &diskResourceModel{
		Description: types.StringValue("note"),
		Enabled:     types.BoolNull(),
	}
	state := &diskResourceModel{
		Description: types.StringValue(""),
		Enabled:     types.BoolValue(true),
	}
	body := jsonObject(t, diskUpdatePayload(plan, state))
	requireString(t, body, "description", "note")
	requireAbsent(t, body, "enabled")
}
