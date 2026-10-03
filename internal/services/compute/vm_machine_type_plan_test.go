// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestImportedMachineTypePlansClean plans a VM the way import leaves it.
// Import stores the API machine type (pc-q35-10.0). Configuration says q35.
// Acceptance ImportState steps never plan from that state; they continue
// from the created state, which already stores q35. With boot_disk adopted,
// the plan must be empty. A created VM that stored q35 must stay empty too.
func TestImportedMachineTypePlansClean(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		configured string
		stored     string
	}{
		{name: "imported q35", configured: "q35", stored: "pc-q35-10.0"},
		{name: "imported pc", configured: "pc", stored: "pc-i440fx-10.0"},
		{name: "created q35", configured: "q35", stored: "q35"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := adoptedVM(tc.stored, 1024)
			config := configuredVM(tc.configured, 1024)
			proposed := adoptedVM(tc.stored, 1024)
			proposed.MachineType = types.StringValue(tc.configured)

			planned := planVM(t, config, proposed, state)
			prior := vmRaw(t, state)
			if unknown := unknownPaths(planned); unknown != "" {
				t.Fatalf("plan has unknown values: %s", unknown)
			}
			if !plansMatch(planned, prior) {
				t.Fatalf("plan is not empty\nplan:  %s\nstate: %s", planned, prior)
			}
			model := decodeVM(t, planned)
			if model.MachineType.ValueString() != tc.stored {
				t.Fatalf("machine_type plan = %s, want stored %s", model.MachineType, tc.stored)
			}
			if !model.ConsolePass.IsNull() || model.ConsolePass.IsUnknown() {
				t.Fatalf("console_pass plan = %s, want null", model.ConsolePass)
			}
			if model.SecureBoot.IsUnknown() || model.SecureBoot.IsNull() || model.SecureBoot.ValueBool() {
				t.Fatalf("secure_boot plan = %s, want false", model.SecureBoot)
			}
			if model.CloudInitFiles != nil {
				t.Fatalf("cloudinit_files plan = %#v, want null", model.CloudInitFiles)
			}
			if !model.GuestAgentIPs.IsNull() {
				t.Fatalf("guest_agent_ips plan = %s, want null", model.GuestAgentIPs)
			}
		})
	}
}

// TestMachineTypePlanKeepsRealChanges is the other half of the import plan.
// An equivalent machine type must not hide a real edit, a boot disk that
// still has to be adopted, or a switch to a different machine type.
func TestMachineTypePlanKeepsRealChanges(t *testing.T) {
	t.Parallel()

	t.Run("ram change", func(t *testing.T) {
		t.Parallel()
		state := adoptedVM("pc-q35-10.0", 1024)
		config := configuredVM("q35", 2048)
		proposed := adoptedVM("pc-q35-10.0", 2048)
		proposed.MachineType = types.StringValue("q35")

		planned := planVM(t, config, proposed, state)
		if planned.Equal(vmRaw(t, state)) {
			t.Fatal("ram change was planned as a no-op")
		}
		model := decodeVM(t, planned)
		if model.RAM.IsUnknown() || model.RAM.ValueInt32() != 2048 {
			t.Fatalf("ram plan = %s, want 2048", model.RAM)
		}
		if !model.SecureBoot.IsUnknown() {
			t.Fatalf("secure_boot plan = %s, want unknown on a real update", model.SecureBoot)
		}
		if model.MachineType.ValueString() != "pc-q35-10.0" {
			t.Fatalf("machine_type plan = %s, want stored pc-q35-10.0", model.MachineType)
		}
	})

	t.Run("boot disk adoption", func(t *testing.T) {
		t.Parallel()
		state := adoptedVM("pc-q35-10.0", 1024)
		state.BootDisk = nil
		config := configuredVM("q35", 1024)
		proposed := adoptedVM("pc-q35-10.0", 1024)
		proposed.MachineType = types.StringValue("q35")
		proposed.BootDisk = config.BootDisk

		planned := planVM(t, config, proposed, state)
		if planned.Equal(vmRaw(t, state)) {
			t.Fatal("boot disk adoption was planned as a no-op")
		}
		model := decodeVM(t, planned)
		if model.BootDisk == nil {
			t.Fatal("boot disk adoption dropped the configured disk")
		}
	})

	t.Run("different machine type", func(t *testing.T) {
		t.Parallel()
		state := adoptedVM("pc-q35-10.0", 1024)
		config := configuredVM("pc-q35-9.0", 1024)
		proposed := adoptedVM("pc-q35-10.0", 1024)
		proposed.MachineType = types.StringValue("pc-q35-9.0")

		planned := planVM(t, config, proposed, state)
		if planned.Equal(vmRaw(t, state)) {
			t.Fatal("machine type change was planned as a no-op")
		}
		model := decodeVM(t, planned)
		if model.MachineType.ValueString() != "pc-q35-9.0" {
			t.Fatalf("machine_type plan = %s, want pc-q35-9.0", model.MachineType)
		}
	})
}

func adoptedVM(machineType string, ram int32) *VMResourceModel {
	return &VMResourceModel{
		Id:                 types.StringValue("45"),
		Machine:            types.Int32Value(70),
		Name:               types.StringValue("zzimp-vm"),
		Cluster:            types.Int32Value(1),
		Description:        types.StringValue(""),
		Enabled:            types.BoolValue(true),
		MachineType:        types.StringValue(machineType),
		CPUCores:           types.Int32Value(1),
		CPUType:            types.StringValue("host"),
		RAM:                types.Int32Value(ram),
		OSFamily:           types.StringValue("linux"),
		ConsolePassEnabled: types.BoolValue(false),
		ConsolePass:        types.StringNull(),
		UEFI:               types.BoolValue(true),
		SecureBoot:         types.BoolValue(false),
		PowerState:         types.BoolValue(false),
		ForcePowerOff:      types.BoolValue(false),
		ShutdownOnDestroy:  types.StringValue(shutdownOnDestroyGracefulThenKill),
		GuestAgentIPs:      types.ListNull(types.StringType),
		BootDisk: &bootDiskModel{
			Key:  types.StringValue("45"),
			Name: types.StringValue("os"),
			Size: types.Float64Value(2),
		},
	}
}

func configuredVM(machineType string, ram int32) *VMResourceModel {
	return &VMResourceModel{
		Name:          types.StringValue("zzimp-vm"),
		MachineType:   types.StringValue(machineType),
		CPUCores:      types.Int32Value(1),
		RAM:           types.Int32Value(ram),
		OSFamily:      types.StringValue("linux"),
		PowerState:    types.BoolValue(false),
		GuestAgentIPs: types.ListNull(types.StringType),
		BootDisk: &bootDiskModel{
			Name: types.StringValue("os"),
			Size: types.Float64Value(2),
		},
	}
}

func planVM(t *testing.T, config, proposed, prior *VMResourceModel) tftypes.Value {
	t.Helper()
	ctx := t.Context()
	server, err := providerserver.NewProtocol6WithError(&vmTestProvider{})()
	if err != nil {
		t.Fatal(err)
	}
	typ := vmSchema(t).Type().TerraformType(ctx)
	resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "vergeio_vm",
		Config:           mustDynamic(t, typ, vmRaw(t, config)),
		ProposedNewState: mustDynamic(t, typ, vmRaw(t, proposed)),
		PriorState:       mustDynamic(t, typ, vmRaw(t, prior)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if diags := formatPlanDiags(resp.Diagnostics); diags != "" {
		t.Fatal(diags)
	}
	if resp.PlannedState == nil {
		t.Fatal("plan response has no planned state")
	}
	planned, err := resp.PlannedState.Unmarshal(typ)
	if err != nil {
		t.Fatal(err)
	}
	return planned
}

func vmRaw(t *testing.T, model *VMResourceModel) tftypes.Value {
	t.Helper()
	state := tfsdk.State{Schema: vmSchema(t)}
	if diags := state.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	return state.Raw
}

func decodeVM(t *testing.T, raw tftypes.Value) VMResourceModel {
	t.Helper()
	state := tfsdk.State{Schema: vmSchema(t), Raw: raw}
	var model VMResourceModel
	if diags := state.Get(context.Background(), &model); diags.HasError() {
		t.Fatal(diags)
	}
	return model
}

func mustDynamic(t *testing.T, typ tftypes.Type, value tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()
	dv, err := tfprotov6.NewDynamicValue(typ, value)
	if err != nil {
		t.Fatal(err)
	}
	return &dv
}

func formatPlanDiags(diags []*tfprotov6.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		if d == nil || d.Severity != tfprotov6.DiagnosticSeverityError {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(d.Summary)
		if d.Detail != "" {
			b.WriteString(": ")
			b.WriteString(d.Detail)
		}
	}
	return b.String()
}

// plansMatch compares a plan response to prior state. The protocol reifies a
// null vergeio_device block to an empty list on the way out and nullifies it
// again on the way in, so those two shapes are the same plan.
func plansMatch(plan, state tftypes.Value) bool {
	return valuesMatch(plan, state, false)
}

func valuesMatch(plan, state tftypes.Value, deviceBlock bool) bool {
	if plan.Equal(state) {
		return true
	}
	if deviceBlock && nullOrEmptyList(plan) && nullOrEmptyList(state) {
		return true
	}
	if !plan.IsKnown() || !state.IsKnown() || plan.IsNull() || state.IsNull() {
		return false
	}
	switch {
	case plan.Type().Is(tftypes.Object{}) && state.Type().Is(tftypes.Object{}):
		var planAttrs, stateAttrs map[string]tftypes.Value
		if plan.As(&planAttrs) != nil || state.As(&stateAttrs) != nil || len(planAttrs) != len(stateAttrs) {
			return false
		}
		for name, planAttr := range planAttrs {
			stateAttr, ok := stateAttrs[name]
			if !ok || !valuesMatch(planAttr, stateAttr, name == "vergeio_device") {
				return false
			}
		}
		return true
	case (plan.Type().Is(tftypes.List{}) || plan.Type().Is(tftypes.Tuple{})) && plan.Type().Equal(state.Type()):
		var planElems, stateElems []tftypes.Value
		if plan.As(&planElems) != nil || state.As(&stateElems) != nil || len(planElems) != len(stateElems) {
			return false
		}
		for i := range planElems {
			if !valuesMatch(planElems[i], stateElems[i], false) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func nullOrEmptyList(v tftypes.Value) bool {
	if !v.IsKnown() || v.Type() == nil {
		return false
	}
	if !v.Type().Is(tftypes.List{}) && !v.Type().Is(tftypes.Set{}) && !v.Type().Is(tftypes.Tuple{}) {
		return false
	}
	if v.IsNull() {
		return true
	}
	var elems []tftypes.Value
	if v.As(&elems) != nil {
		return false
	}
	return len(elems) == 0
}

func unknownPaths(value tftypes.Value) string {
	var paths []string
	_ = tftypes.Walk(value, func(path *tftypes.AttributePath, v tftypes.Value) (bool, error) {
		if !v.IsKnown() {
			if path == nil || len(path.Steps()) == 0 {
				paths = append(paths, "<root>")
			} else {
				paths = append(paths, path.String())
			}
			return false, nil
		}
		return true, nil
	})
	return strings.Join(paths, ", ")
}
