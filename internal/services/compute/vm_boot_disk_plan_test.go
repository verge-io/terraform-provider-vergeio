// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestBootDiskMediaRequiresReplace(t *testing.T) {
	mod := bootDiskMediaRequiresReplace()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	state := tfsdk.State{Raw: raw}
	plan := tfsdk.Plan{Raw: raw}

	adopt := &planmodifier.StringResponse{PlanValue: types.StringValue("import")}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringValue("import"),
		PlanValue:   types.StringValue("import"),
		StateValue:  types.StringNull(),
		State:       state,
		Plan:        plan,
	}, adopt)
	if adopt.RequiresReplace {
		t.Fatal("null prior media (import/upgrade adopt) should not replace the VM")
	}

	changed := &planmodifier.StringResponse{PlanValue: types.StringValue("cdrom")}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringValue("cdrom"),
		PlanValue:   types.StringValue("cdrom"),
		StateValue:  types.StringValue("import"),
		State:       state,
		Plan:        plan,
	}, changed)
	if !changed.RequiresReplace {
		t.Fatal("changing media on an owned boot disk should replace the VM")
	}

	same := &planmodifier.StringResponse{PlanValue: types.StringValue("import")}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringValue("import"),
		PlanValue:   types.StringValue("import"),
		StateValue:  types.StringValue("import"),
		State:       state,
		Plan:        plan,
	}, same)
	if same.RequiresReplace {
		t.Fatal("unchanged media should not replace the VM")
	}

	cleared := &planmodifier.StringResponse{PlanValue: types.StringNull()}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		PlanValue:   types.StringNull(),
		StateValue:  types.StringValue("import"),
		State:       state,
		Plan:        plan,
	}, cleared)
	if !cleared.RequiresReplace {
		t.Fatal("clearing media on an owned boot disk should replace the VM")
	}
}

func TestBootDiskSourceRequiresReplace(t *testing.T) {
	mod := bootDiskSourceRequiresReplace()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	state := tfsdk.State{Raw: raw}
	plan := tfsdk.Plan{Raw: raw}

	adopt := &planmodifier.Int32Response{PlanValue: types.Int32Value(94)}
	mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
		ConfigValue: types.Int32Value(94),
		PlanValue:   types.Int32Value(94),
		StateValue:  types.Int32Null(),
		State:       state,
		Plan:        plan,
	}, adopt)
	if adopt.RequiresReplace {
		t.Fatal("null prior source (import/upgrade adopt) should not replace the VM")
	}

	changed := &planmodifier.Int32Response{PlanValue: types.Int32Value(18)}
	mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
		ConfigValue: types.Int32Value(18),
		PlanValue:   types.Int32Value(18),
		StateValue:  types.Int32Value(94),
		State:       state,
		Plan:        plan,
	}, changed)
	if !changed.RequiresReplace {
		t.Fatal("changing source on an owned boot disk should replace the VM")
	}

	same := &planmodifier.Int32Response{PlanValue: types.Int32Value(94)}
	mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
		ConfigValue: types.Int32Value(94),
		PlanValue:   types.Int32Value(94),
		StateValue:  types.Int32Value(94),
		State:       state,
		Plan:        plan,
	}, same)
	if same.RequiresReplace {
		t.Fatal("unchanged source should not replace the VM")
	}

	cleared := &planmodifier.Int32Response{PlanValue: types.Int32Null()}
	mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
		ConfigValue: types.Int32Null(),
		PlanValue:   types.Int32Null(),
		StateValue:  types.Int32Value(94),
		State:       state,
		Plan:        plan,
	}, cleared)
	if !cleared.RequiresReplace {
		t.Fatal("clearing source on an owned boot disk should replace the VM")
	}
}
