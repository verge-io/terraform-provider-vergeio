package vm

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestVMCreateBodyKeepsFalseZeroAndEmpty(t *testing.T) {
	data := &VMResourceModel{
		Name:         types.StringValue("vm"),
		Description:  types.StringValue(""),
		USBTablet:    types.BoolValue(false),
		AllowHotplug: types.BoolValue(false),
		BootDelay:    types.Int32Value(0),
		GuestAgent:   types.BoolNull(),
		PowerState:   types.BoolValue(false),
	}

	model := jsonObject(t, vmCreateModel(data))
	requireBool(t, model, "usb_tablet", false)
	requireBool(t, model, "allow_hotplug", false)
	requireNumber(t, model, "boot_delay", 0)
	requireString(t, model, "description", "")
	requireAbsent(t, model, "guest_agent")
	requireAbsent(t, model, "powerstate")

	req, err := vmCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	body := jsonObject(t, req)
	requireBool(t, body, "usb_tablet", false)
	requireBool(t, body, "allow_hotplug", false)
	requireNumber(t, body, "boot_delay", 0)
	requireAbsent(t, body, "guest_agent")
	requireAbsent(t, body, "powerstate")
}

func TestVMUpdateBodyKeepsFalseZeroAndEmpty(t *testing.T) {
	plan := &VMResourceModel{
		Id:                   types.StringValue("9"),
		Name:                 types.StringValue("vm"),
		Description:          types.StringValue(""),
		GuestAgent:           types.BoolValue(false),
		BootDelay:            types.Int32Value(0),
		USBTablet:            types.BoolValue(false),
		PowerState:           types.BoolValue(false),
		CPUCores:             types.Int32Value(2),
		NestedVirtualization: types.BoolValue(false),
	}
	state := &VMResourceModel{
		Id:                   types.StringValue("9"),
		Name:                 types.StringValue("vm"),
		Description:          types.StringValue("before"),
		GuestAgent:           types.BoolValue(true),
		BootDelay:            types.Int32Value(5),
		USBTablet:            types.BoolValue(true),
		PowerState:           types.BoolValue(true),
		CPUCores:             types.Int32Value(2),
		NestedVirtualization: types.BoolValue(true),
	}

	model := jsonObject(t, vmUpdateModel(plan, state))
	requireString(t, model, "description", "")
	requireBool(t, model, "guest_agent", false)
	requireNumber(t, model, "boot_delay", 0)
	requireBool(t, model, "usb_tablet", false)
	requireBool(t, model, "nested_virtualization", false)
	requireAbsent(t, model, "name")
	requireAbsent(t, model, "cpu_cores")
	requireAbsent(t, model, "powerstate")

	req, id, err := vmUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if id != 9 {
		t.Fatalf("vm id = %d, want 9", id)
	}
	body := jsonObject(t, req)
	requireString(t, body, "description", "")
	requireBool(t, body, "guest_agent", false)
	requireNumber(t, body, "boot_delay", 0)
	requireBool(t, body, "usb_tablet", false)
	requireBool(t, body, "nested_virtualization", false)
	requireAbsent(t, body, "name")
	requireAbsent(t, body, "cpu_cores")
	requireAbsent(t, body, "powerstate")
}

func TestVMCreateOmitsUnsetBool(t *testing.T) {
	data := &VMResourceModel{
		Name:      types.StringValue("vm"),
		USBTablet: types.BoolUnknown(),
	}
	model := jsonObject(t, vmCreateModel(data))
	requireAbsent(t, model, "usb_tablet")
	requireString(t, model, "name", "vm")
}
