// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestRuleListPlanFollowsName is the unit stand-in for the acceptance
// failure. UseStateForUnknown planned the id of whichever rule sat at the
// same index, and null for a new index. Apply then stored a different id
// and Terraform rejected the result.
func TestRuleListPlanFollowsName(t *testing.T) {
	ctx := context.Background()
	rulesSchema := networkRulesSchema(t)
	ssh := storedRule("ssh", "14", 1)
	web := storedRule("web", "15", 2)
	dns := storedRule("dns", "17", 3)
	state := rulesValue(t, rulesSchema, ssh, web, dns)

	t.Run("reorder keeps each id and drops orderid", func(t *testing.T) {
		config := rulesValue(t, rulesSchema, configuredRule("dns"), configuredRule("ssh"), configuredRule("web"))
		assertRuleString(t, ctx, rulesSchema, config, state, 0, "id", "17")
		assertRuleString(t, ctx, rulesSchema, config, state, 1, "id", "14")
		assertRuleString(t, ctx, rulesSchema, config, state, 2, "id", "15")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 0, "orderid")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 1, "orderid")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 2, "orderid")
	})

	t.Run("insert leaves the new rule unknown", func(t *testing.T) {
		config := rulesValue(t, rulesSchema, configuredRule("ssh"), configuredRule("icmp"), configuredRule("web"), configuredRule("dns"))
		assertRuleString(t, ctx, rulesSchema, config, state, 0, "id", "14")
		assertRuleInt32(t, ctx, rulesSchema, config, state, 0, "orderid", 1)
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 1, "id")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 1, "orderid")
		assertRuleString(t, ctx, rulesSchema, config, state, 2, "id", "15")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 2, "orderid")
		assertRuleString(t, ctx, rulesSchema, config, state, 3, "id", "17")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 3, "orderid")
	})

	t.Run("remove shifts the kept id", func(t *testing.T) {
		config := rulesValue(t, rulesSchema, configuredRule("ssh"), configuredRule("dns"))
		assertRuleString(t, ctx, rulesSchema, config, state, 0, "id", "14")
		assertRuleInt32(t, ctx, rulesSchema, config, state, 0, "orderid", 1)
		assertRuleString(t, ctx, rulesSchema, config, state, 1, "id", "17")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 1, "orderid")
	})

	t.Run("in place edit keeps id and orderid", func(t *testing.T) {
		edited := configuredRule("web")
		edited.DestinationPorts = types.StringValue("443")
		config := rulesValue(t, rulesSchema, configuredRule("ssh"), edited, configuredRule("dns"))
		assertRuleString(t, ctx, rulesSchema, config, state, 1, "id", "15")
		assertRuleInt32(t, ctx, rulesSchema, config, state, 1, "orderid", 2)
	})

	t.Run("rename stays unknown", func(t *testing.T) {
		config := rulesValue(t, rulesSchema, configuredRule("ssh"), configuredRule("web"), configuredRule("dns2"))
		assertRuleString(t, ctx, rulesSchema, config, state, 0, "id", "14")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 2, "id")
		assertRuleUnknown(t, ctx, rulesSchema, config, state, 2, "orderid")
	})
}

func TestRuleListPlanUnknownFromEmpty(t *testing.T) {
	ctx := context.Background()
	rulesSchema := networkRulesSchema(t)
	empty, diags := rulesToList(ctx, nil)
	if diags.HasError() {
		t.Fatal(diags)
	}
	state := tfsdk.State{Schema: rulesSchema}
	diags = state.Set(ctx, &networkRulesModel{
		ID:   types.StringValue("19"),
		VNet: types.StringValue("19"),
		Rule: empty,
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	config := rulesValue(t, rulesSchema, configuredRule("ssh"), configuredRule("web"))
	assertRuleUnknown(t, ctx, rulesSchema, config, state, 0, "id")
	assertRuleUnknown(t, ctx, rulesSchema, config, state, 0, "orderid")
	assertRuleUnknown(t, ctx, rulesSchema, config, state, 1, "id")
	assertRuleUnknown(t, ctx, rulesSchema, config, state, 1, "orderid")
}

func TestRuleListPlanUnknownOnCreate(t *testing.T) {
	ctx := context.Background()
	rulesSchema := networkRulesSchema(t)
	config := rulesValue(t, rulesSchema, configuredRule("ssh"))
	state := tfsdk.State{
		Schema: rulesSchema,
		Raw:    tftypes.NewValue(rulesSchema.Type().TerraformType(ctx), nil),
	}
	assertRuleUnknown(t, ctx, rulesSchema, config, state, 0, "id")
	assertRuleUnknown(t, ctx, rulesSchema, config, state, 0, "orderid")
}

func TestSingleRuleIDKeepsPriorState(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	NewNetworkRuleResource().Schema(ctx, resource.SchemaRequest{}, &resp)
	id, ok := resp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("id should be a string attribute")
	}
	want := stringplanmodifier.UseStateForUnknown().Description(ctx)
	if len(id.PlanModifiers) != 1 || id.PlanModifiers[0].Description(ctx) != want {
		t.Fatalf("single rule id modifier = %q, want UseStateForUnknown", describeModifiers(id.PlanModifiers))
	}
	prior := tfsdk.State{Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})}
	plan := types.StringUnknown()
	modResp := &planmodifier.StringResponse{PlanValue: plan}
	id.PlanModifiers[0].PlanModifyString(ctx, planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		PlanValue:   plan,
		StateValue:  types.StringValue("14"),
		State:       prior,
	}, modResp)
	if modResp.Diagnostics.HasError() {
		t.Fatal(modResp.Diagnostics)
	}
	if modResp.PlanValue.IsUnknown() || modResp.PlanValue.ValueString() != "14" {
		t.Fatalf("single rule id plan = %s, want 14", modResp.PlanValue)
	}
}

func networkRulesSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	NewNetworkRulesResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema
}

func rulesValue(t *testing.T, rulesSchema schema.Schema, rules ...nestedFirewallRuleModel) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	list, diags := types.ListValueFrom(ctx, ruleObjectType(), rules)
	if diags.HasError() {
		t.Fatal(diags)
	}
	state := tfsdk.State{Schema: rulesSchema}
	diags = state.Set(ctx, &networkRulesModel{
		ID:   types.StringValue("19"),
		VNet: types.StringValue("19"),
		Rule: list,
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	return state
}

func configuredRule(name string) nestedFirewallRuleModel {
	return nestedFirewallRuleModel{Name: types.StringValue(name)}
}

func storedRule(name, id string, order int32) nestedFirewallRuleModel {
	rule := configuredRule(name)
	rule.ID = types.StringValue(id)
	rule.OrderID = types.Int32Value(order)
	rule.Direction = types.StringValue("incoming")
	rule.Action = types.StringValue("accept")
	rule.Protocol = types.StringValue("tcp")
	rule.Interface = types.StringValue("auto")
	rule.Enabled = types.BoolValue(true)
	rule.Pin = types.StringValue("no")
	return rule
}

func assertRuleUnknown(t *testing.T, ctx context.Context, rulesSchema schema.Schema, config, state tfsdk.State, index int, name string) {
	t.Helper()
	got := planRuleAttribute(t, ctx, rulesSchema, config, state, index, name)
	if got == nil || got.IsNull() || !got.IsUnknown() {
		t.Fatalf("rule[%d].%s plan = %s, want unknown", index, name, got)
	}
}

func assertRuleString(t *testing.T, ctx context.Context, rulesSchema schema.Schema, config, state tfsdk.State, index int, name, want string) {
	t.Helper()
	got := planRuleAttribute(t, ctx, rulesSchema, config, state, index, name)
	value, ok := got.(types.String)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueString() != want {
		t.Fatalf("rule[%d].%s plan = %s, want %q", index, name, got, want)
	}
}

func assertRuleInt32(t *testing.T, ctx context.Context, rulesSchema schema.Schema, config, state tfsdk.State, index int, name string, want int32) {
	t.Helper()
	got := planRuleAttribute(t, ctx, rulesSchema, config, state, index, name)
	value, ok := got.(types.Int32)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueInt32() != want {
		t.Fatalf("rule[%d].%s plan = %s, want %d", index, name, got, want)
	}
}

func planRuleAttribute(t *testing.T, ctx context.Context, rulesSchema schema.Schema, config, state tfsdk.State, index int, name string) attr.Value {
	t.Helper()
	nested, ok := rulesSchema.Attributes["rule"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("rule should be a list nested attribute")
	}
	attrPath := path.Root("rule").AtListIndex(index).AtName(name)
	configData := tfsdk.Config{Schema: rulesSchema, Raw: config.Raw}
	planData := tfsdk.Plan{Schema: rulesSchema, Raw: config.Raw}
	switch typed := nested.NestedObject.Attributes[name].(type) {
	case schema.StringAttribute:
		var configValue types.String
		if diags := configData.GetAttribute(ctx, attrPath, &configValue); diags.HasError() {
			t.Fatal(diags)
		}
		got := configValue
		if configValue.IsNull() {
			got = types.StringUnknown()
		}
		for _, mod := range typed.PlanModifiers {
			resp := &planmodifier.StringResponse{PlanValue: got}
			mod.PlanModifyString(ctx, planmodifier.StringRequest{
				Config:      configData,
				ConfigValue: configValue,
				Plan:        planData,
				PlanValue:   got,
				State:       state,
				StateValue:  types.StringNull(),
				Path:        attrPath,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			got = resp.PlanValue
		}
		return got
	case schema.Int32Attribute:
		var configValue types.Int32
		if diags := configData.GetAttribute(ctx, attrPath, &configValue); diags.HasError() {
			t.Fatal(diags)
		}
		got := configValue
		if configValue.IsNull() {
			got = types.Int32Unknown()
		}
		for _, mod := range typed.PlanModifiers {
			resp := &planmodifier.Int32Response{PlanValue: got}
			mod.PlanModifyInt32(ctx, planmodifier.Int32Request{
				Config:      configData,
				ConfigValue: configValue,
				Plan:        planData,
				PlanValue:   got,
				State:       state,
				StateValue:  types.Int32Null(),
				Path:        attrPath,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			got = resp.PlanValue
		}
		return got
	default:
		t.Fatalf("rule attribute %s has unexpected type %T", name, nested.NestedObject.Attributes[name])
		return nil
	}
}

func describeModifiers(mods []planmodifier.String) string {
	if len(mods) == 0 {
		return ""
	}
	return mods[0].Description(context.Background())
}
