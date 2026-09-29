package compute

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
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

func TestVMConsolePassSentOnlyWhenSetOrChanged(t *testing.T) {
	withPass := &VMResourceModel{
		Name:               types.StringValue("vm"),
		ConsolePassEnabled: types.BoolValue(true),
		ConsolePass:        types.StringValue("console-secret"),
	}
	created := jsonObject(t, vmCreateModel(withPass))
	requireString(t, created, "console_pass", "console-secret")
	requireBool(t, created, "console_pass_enabled", true)

	unset := &VMResourceModel{
		Name:        types.StringValue("vm"),
		ConsolePass: types.StringUnknown(),
	}
	requireAbsent(t, jsonObject(t, vmCreateModel(unset)), "console_pass")

	same := &VMResourceModel{
		Id:          types.StringValue("9"),
		Name:        types.StringValue("vm"),
		ConsolePass: types.StringValue("console-secret"),
	}
	requireAbsent(t, jsonObject(t, vmUpdateModel(same, same)), "console_pass")

	rotated := &VMResourceModel{
		Id:          types.StringValue("9"),
		Name:        types.StringValue("vm"),
		ConsolePass: types.StringValue("console-secret-rotated"),
	}
	updated := jsonObject(t, vmUpdateModel(rotated, same))
	requireString(t, updated, "console_pass", "console-secret-rotated")
	requireAbsent(t, updated, "name")
}

func TestVMCreateFillsPlatformCPUAndRAMDefaults(t *testing.T) {
	cases := []struct {
		name     string
		cores    types.Int32
		ram      types.Int32
		wantCore float64
		wantRAM  float64
	}{
		{
			name:     "both omitted",
			cores:    types.Int32Null(),
			ram:      types.Int32Null(),
			wantCore: 1,
			wantRAM:  1024,
		},
		{
			name:     "both unknown",
			cores:    types.Int32Unknown(),
			ram:      types.Int32Unknown(),
			wantCore: 1,
			wantRAM:  1024,
		},
		{
			name:     "ram omitted",
			cores:    types.Int32Value(4),
			ram:      types.Int32Null(),
			wantCore: 4,
			wantRAM:  1024,
		},
		{
			name:     "cpu omitted",
			cores:    types.Int32Unknown(),
			ram:      types.Int32Value(8192),
			wantCore: 1,
			wantRAM:  8192,
		},
		{
			name:     "explicit values",
			cores:    types.Int32Value(2),
			ram:      types.Int32Value(2048),
			wantCore: 2,
			wantRAM:  2048,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := &VMResourceModel{
				Name:     types.StringValue("vm"),
				CPUCores: tc.cores,
				RAM:      tc.ram,
			}
			model := jsonObject(t, vmCreateModel(data))
			requireNumber(t, model, "cpu_cores", tc.wantCore)
			requireNumber(t, model, "ram", tc.wantRAM)

			req, err := vmCreateRequest(data)
			if err != nil {
				t.Fatal(err)
			}
			if req.Name != "vm" {
				t.Fatalf("name = %q, want vm", req.Name)
			}
			if req.CPUCores != int(tc.wantCore) {
				t.Fatalf("cpu_cores = %d, want %v", req.CPUCores, tc.wantCore)
			}
			if req.RAM != int(tc.wantRAM) {
				t.Fatalf("ram = %d, want %v", req.RAM, tc.wantRAM)
			}
		})
	}
}

func TestVMCreateRejectsNonPositiveCPUAndRAM(t *testing.T) {
	cases := []struct {
		name  string
		cores types.Int32
		ram   types.Int32
		field string
	}{
		{name: "zero cores", cores: types.Int32Value(0), ram: types.Int32Value(1024), field: "cpu_cores"},
		{name: "negative cores", cores: types.Int32Value(-2), ram: types.Int32Value(1024), field: "cpu_cores"},
		{name: "zero ram", cores: types.Int32Value(1), ram: types.Int32Value(0), field: "ram"},
		{name: "negative ram", cores: types.Int32Value(1), ram: types.Int32Value(-1), field: "ram"},
	}

	// Validation runs before the client is used.
	svc := &vergeos.VMService{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := vmCreateRequest(&VMResourceModel{
				Name:     types.StringValue("vm"),
				CPUCores: tc.cores,
				RAM:      tc.ram,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = svc.Create(context.Background(), req)
			var validation *vergeos.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("Create error = %v, want validation error", err)
			}
			if validation.Field != tc.field {
				t.Fatalf("validation field = %q, want %q", validation.Field, tc.field)
			}
			if !strings.Contains(validation.Message, "must be positive") {
				t.Fatalf("validation message = %q, want must be positive", validation.Message)
			}
		})
	}
}

func TestVMUpdateOmitsUnsetCPUAndRAM(t *testing.T) {
	plan := &VMResourceModel{
		Id:       types.StringValue("9"),
		Name:     types.StringValue("vm"),
		CPUCores: types.Int32Null(),
		RAM:      types.Int32Unknown(),
	}
	state := &VMResourceModel{
		Id:       types.StringValue("9"),
		Name:     types.StringValue("vm"),
		CPUCores: types.Int32Value(4),
		RAM:      types.Int32Value(4096),
	}

	model := jsonObject(t, vmUpdateModel(plan, state))
	requireAbsent(t, model, "cpu_cores")
	requireAbsent(t, model, "ram")

	req, _, err := vmUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	body := jsonObject(t, req)
	requireAbsent(t, body, "cpu_cores")
	requireAbsent(t, body, "ram")
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
