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
