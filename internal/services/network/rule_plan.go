// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ planmodifier.String = ruleListComputedModifier{}
	_ planmodifier.Int32  = ruleListComputedModifier{}
)

// ruleListComputedModifier plans id and orderid on a nested rule.
//
// UseStateForUnknown copies the value at the same list index, including
// null. Reordering, inserting, removing, or renaming a rule then plans
// another rule's id, and a new index plans null. Apply writes the real
// values and Terraform rejects the plan as inconsistent.
//
// Name is the identity. id is copied from the rule with that name, wherever
// it sits in state. orderid changes when the rule moves, so it is copied
// only when that name is still at this index. A new rule, and a rule that
// moved, stays unknown.
type ruleListComputedModifier struct {
	// sameIndex is set for orderid. A moved rule must not keep the orderid
	// of the rule that previously had its name.
	sameIndex bool
}

func (m ruleListComputedModifier) Description(context.Context) string {
	if m.sameIndex {
		return "Plans orderid from state when the rule at this index keeps its name. A new or moved rule stays unknown."
	}
	return "Plans the rule id from the rule with the same name. A new name stays unknown."
}

func (m ruleListComputedModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m ruleListComputedModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if kept, ok := keepKnownString(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	resp.PlanValue = types.StringUnknown()
	if !req.PlanValue.IsUnknown() && !req.PlanValue.IsNull() {
		resp.PlanValue = req.PlanValue
		return
	}
	if attributeName(req.Path) != "id" {
		resp.Diagnostics.AddAttributeError(req.Path, "Unexpected rule attribute", "The rule id plan modifier was applied to "+attributeName(req.Path)+".")
		return
	}
	prior, diags := m.lookup(ctx, req.Config, req.State, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || prior == nil {
		return
	}
	value, ok := prior.(types.String)
	if !ok || value.IsNull() || value.IsUnknown() {
		return
	}
	resp.PlanValue = value
}

func (m ruleListComputedModifier) PlanModifyInt32(ctx context.Context, req planmodifier.Int32Request, resp *planmodifier.Int32Response) {
	if kept, ok := keepKnownInt32(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	resp.PlanValue = types.Int32Unknown()
	if !req.PlanValue.IsUnknown() && !req.PlanValue.IsNull() {
		resp.PlanValue = req.PlanValue
		return
	}
	if attributeName(req.Path) != "orderid" {
		resp.Diagnostics.AddAttributeError(req.Path, "Unexpected rule attribute", "The rule orderid plan modifier was applied to "+attributeName(req.Path)+".")
		return
	}
	prior, diags := m.lookup(ctx, req.Config, req.State, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || prior == nil {
		return
	}
	value, ok := prior.(types.Int32)
	if !ok || value.IsNull() || value.IsUnknown() {
		return
	}
	resp.PlanValue = value
}

func keepKnownString(config types.String) (types.String, bool) {
	if config.IsNull() {
		return types.String{}, false
	}
	if config.IsUnknown() {
		return types.StringUnknown(), true
	}
	return config, true
}

func keepKnownInt32(config types.Int32) (types.Int32, bool) {
	if config.IsNull() {
		return types.Int32{}, false
	}
	if config.IsUnknown() {
		return types.Int32Unknown(), true
	}
	return config, true
}

func (m ruleListComputedModifier) lookup(ctx context.Context, config tfsdk.Config, state tfsdk.State, attrPath path.Path) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !rawReady(state.Raw) || !rawReady(config.Raw) {
		return nil, diags
	}
	index, ok := listIndex(attrPath)
	if !ok {
		diags.AddAttributeError(attrPath, "Unexpected rule path", "The rule plan modifier expected a list index.")
		return nil, diags
	}
	name, nameDiags := configuredRuleName(ctx, config, attrPath, index)
	diags.Append(nameDiags...)
	if diags.HasError() || name == "" {
		return nil, diags
	}
	rules, rulesDiags := rulesFromState(ctx, state)
	diags.Append(rulesDiags...)
	if diags.HasError() {
		return nil, diags
	}
	rule, found := findPlannedRule(rules, name, index, m.sameIndex)
	if !found {
		return nil, diags
	}
	switch attributeName(attrPath) {
	case "id":
		return rule.ID, diags
	case "orderid":
		return rule.OrderID, diags
	default:
		diags.AddAttributeError(attrPath, "Unexpected rule attribute", "The rule plan modifier does not know attribute "+attributeName(attrPath)+".")
		return nil, diags
	}
}

func rawReady(raw tftypes.Value) bool {
	if raw.Type() == nil || raw.IsNull() || !raw.IsKnown() {
		return false
	}
	return true
}

func configuredRuleName(ctx context.Context, config tfsdk.Config, attrPath path.Path, index int) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	var model networkRulesModel
	diags.Append(config.Get(ctx, &model)...)
	if diags.HasError() {
		return "", diags
	}
	rules, rulesDiags := ruleModels(ctx, model.Rule)
	diags.Append(rulesDiags...)
	if diags.HasError() {
		return "", diags
	}
	if index < 0 || index >= len(rules) {
		diags.AddAttributeError(attrPath, "Unexpected rule index", "The rule plan modifier index is outside the configured rule list.")
		return "", diags
	}
	name := rules[index].Name
	if name.IsNull() || name.IsUnknown() {
		return "", diags
	}
	return strings.TrimSpace(name.ValueString()), diags
}

func rulesFromState(ctx context.Context, state tfsdk.State) ([]nestedFirewallRuleModel, diag.Diagnostics) {
	var model networkRulesModel
	diags := state.Get(ctx, &model)
	if diags.HasError() {
		return nil, diags
	}
	return ruleModels(ctx, model.Rule)
}

func ruleModels(ctx context.Context, list types.List) ([]nestedFirewallRuleModel, diag.Diagnostics) {
	if list.IsNull() || list.IsUnknown() {
		return nil, nil
	}
	var rules []nestedFirewallRuleModel
	diags := list.ElementsAs(ctx, &rules, false)
	return rules, diags
}

// findPlannedRule returns the state rule whose name matches. sameIndex
// requires that rule to still occupy this index, which is true for orderid
// and false for id.
func findPlannedRule(rules []nestedFirewallRuleModel, name string, index int, sameIndex bool) (nestedFirewallRuleModel, bool) {
	if sameIndex {
		if index < 0 || index >= len(rules) {
			return nestedFirewallRuleModel{}, false
		}
		if !ruleNameIs(rules[index], name) {
			return nestedFirewallRuleModel{}, false
		}
		return rules[index], true
	}
	for _, rule := range rules {
		if ruleNameIs(rule, name) {
			return rule, true
		}
	}
	return nestedFirewallRuleModel{}, false
}

func ruleNameIs(rule nestedFirewallRuleModel, name string) bool {
	if rule.Name.IsNull() || rule.Name.IsUnknown() {
		return false
	}
	return strings.TrimSpace(rule.Name.ValueString()) == name
}

func listIndex(attrPath path.Path) (int, bool) {
	for _, step := range attrPath.Steps() {
		index, ok := step.(path.PathStepElementKeyInt)
		if ok {
			return int(index), true
		}
	}
	return 0, false
}

func attributeName(attrPath path.Path) string {
	steps := attrPath.Steps()
	if len(steps) == 0 {
		return ""
	}
	name, ok := steps[len(steps)-1].(path.PathStepAttributeName)
	if !ok {
		return ""
	}
	return string(name)
}

func ruleIDPlanModifiers(mode ruleSchemaMode) []planmodifier.String {
	if mode == ruleSchemaSingle {
		// A single rule is matched by key, then by name. Renaming it keeps
		// the same id, so the prior id is still the planned id.
		return []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	}
	return []planmodifier.String{ruleListComputedModifier{}}
}
