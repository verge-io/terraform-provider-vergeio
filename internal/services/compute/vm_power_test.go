package compute

import (
	"context"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestPlannedPowerChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		planned types.Bool
		current bool
		change  bool
		powerOn bool
	}{
		{
			name:    "unknown leaves a running VM alone",
			planned: types.BoolUnknown(),
			current: true,
		},
		{
			name:    "unknown leaves a stopped VM alone",
			planned: types.BoolUnknown(),
			current: false,
		},
		{
			name:    "null leaves a running VM alone",
			planned: types.BoolNull(),
			current: true,
		},
		{
			name:    "null leaves a stopped VM alone",
			planned: types.BoolNull(),
			current: false,
		},
		{
			name:    "known on matches a running VM",
			planned: types.BoolValue(true),
			current: true,
		},
		{
			name:    "known off matches a stopped VM",
			planned: types.BoolValue(false),
			current: false,
		},
		{
			name:    "known on starts a stopped VM",
			planned: types.BoolValue(true),
			current: false,
			change:  true,
			powerOn: true,
		},
		{
			name:    "known off stops a running VM",
			planned: types.BoolValue(false),
			current: true,
			change:  true,
			powerOn: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			change, powerOn := plannedPowerChange(tt.planned, tt.current)
			if change != tt.change || powerOn != tt.powerOn {
				t.Fatalf("plannedPowerChange(%v, %v) = (%v, %v), want (%v, %v)",
					tt.planned, tt.current, change, powerOn, tt.change, tt.powerOn)
			}
		})
	}
}

func TestVMPowerStatePlanModifierKeepsPriorState(t *testing.T) {
	vmResource := NewVMResource()
	resp := &fwresource.SchemaResponse{}
	vmResource.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	powerAttr, ok := resp.Schema.Attributes["powerstate"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("powerstate should be a bool attribute")
	}
	if len(powerAttr.PlanModifiers) != 1 {
		t.Fatalf("powerstate should have one plan modifier, got %d", len(powerAttr.PlanModifiers))
	}

	// Configuration omits powerstate, so the framework would mark the plan
	// unknown. The modifier must copy the prior running state into the plan.
	req := planmodifier.BoolRequest{
		State: tfsdk.State{
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"powerstate": tftypes.Bool,
					},
				},
				map[string]tftypes.Value{
					"powerstate": tftypes.NewValue(tftypes.Bool, true),
				},
			),
		},
		StateValue:  types.BoolValue(true),
		PlanValue:   types.BoolUnknown(),
		ConfigValue: types.BoolNull(),
	}
	modifyResp := &planmodifier.BoolResponse{
		PlanValue: req.PlanValue,
	}
	powerAttr.PlanModifiers[0].PlanModifyBool(context.Background(), req, modifyResp)

	if !modifyResp.PlanValue.Equal(types.BoolValue(true)) {
		t.Fatalf("planned powerstate = %#v, want true from prior state", modifyResp.PlanValue)
	}
}
