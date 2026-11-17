// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// ModifyPlan drops a plan whose only difference is an alias of machine_type.
//
// Import stores the API value (pc-q35-10.0). Configuration may say q35.
// MachineTypeSemanticEquality rewrites the planned machine type back to the
// stored value, so the diff does not show machine_type. The framework has
// already compared the raw values and marked every computed attribute that
// is null in configuration as unknown. That happens before plan modifiers,
// and it is what makes the next plan an update. Apply writes nothing, so the
// following plan is the same.
//
// When those unknowns, and an omitted list block normalized to empty, are
// the only differences, the stored machine type already matches configuration.
// Keep it.
func (r *VMResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || !req.Plan.Raw.IsKnown() || req.State.Raw.IsNull() || !req.State.Raw.IsKnown() {
		return
	}
	if req.Config.Raw.IsNull() || !req.Config.Raw.IsKnown() {
		return
	}
	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}
	if vmPlanChangeIsReal(req.Config.Raw, req.Plan.Raw, req.State.Raw) {
		return
	}
	tflog.Debug(ctx, "VM plan matches state aside from an equivalent machine_type; keeping prior state")
	resp.Plan.Raw = req.State.Raw.Copy()
}

// vmPlanChangeIsReal reports whether the plan differs from state for a reason
// other than a machine_type alias. machine_type is compared from configuration
// to state: the plan modifier may already have copied the stored value.
func vmPlanChangeIsReal(config, plan, prior tftypes.Value) bool {
	return objectChangeIsReal(config, plan, prior, true)
}

func objectChangeIsReal(config, plan, prior tftypes.Value, root bool) bool {
	planAttrs, ok := objectAttrs(plan)
	if !ok {
		return true
	}
	priorAttrs, ok := objectAttrs(prior)
	if !ok {
		return true
	}
	configAttrs, ok := objectAttrs(config)
	if !ok {
		return true
	}
	for name, planAttr := range planAttrs {
		priorAttr, ok := priorAttrs[name]
		if !ok {
			return true
		}
		configAttr, ok := configAttrs[name]
		if !ok {
			return true
		}
		if root && name == "machine_type" && machineTypeDriftIsSemantic(configAttr, priorAttr) {
			continue
		}
		if valueChangeIsReal(configAttr, planAttr, priorAttr) {
			return true
		}
	}
	for name := range priorAttrs {
		if _, ok := planAttrs[name]; !ok {
			return true
		}
	}
	return false
}

func valueChangeIsReal(config, plan, prior tftypes.Value) bool {
	if !plan.IsKnown() {
		// Null configuration is the framework marking a computed value unknown.
		// An unknown configuration is a value the user set that is not known yet.
		return !config.IsNull()
	}
	if plan.Equal(prior) {
		return false
	}
	// An omitted list block is null in state and configuration. Once any other
	// attribute differs, the plan normalizes that block to an empty list.
	if config.IsNull() && nullOrEmptyCollection(plan) && nullOrEmptyCollection(prior) {
		return false
	}
	if plan.IsNull() || prior.IsNull() {
		return true
	}
	switch {
	case plan.Type().Is(tftypes.Object{}):
		return objectChangeIsReal(config, plan, prior, false)
	case plan.Type().Is(tftypes.List{}) || plan.Type().Is(tftypes.Tuple{}):
		return sequenceChangeIsReal(config, plan, prior)
	default:
		return true
	}
}

func sequenceChangeIsReal(config, plan, prior tftypes.Value) bool {
	planElems, ok := sequenceElems(plan)
	if !ok {
		return true
	}
	priorElems, ok := sequenceElems(prior)
	if !ok {
		return true
	}
	configElems, ok := sequenceElems(config)
	if !ok {
		return true
	}
	if len(planElems) != len(priorElems) || len(planElems) != len(configElems) {
		return true
	}
	for i := range planElems {
		if valueChangeIsReal(configElems[i], planElems[i], priorElems[i]) {
			return true
		}
	}
	return false
}

func machineTypeDriftIsSemantic(config, prior tftypes.Value) bool {
	if !config.IsKnown() || config.IsNull() || !prior.IsKnown() || prior.IsNull() {
		return false
	}
	if !config.Type().Is(tftypes.String) || !prior.Type().Is(tftypes.String) {
		return false
	}
	var configured, stored string
	if config.As(&configured) != nil || prior.As(&stored) != nil {
		return false
	}
	return shared.MachineTypesAreEquivalent(configured, stored)
}

func objectAttrs(v tftypes.Value) (map[string]tftypes.Value, bool) {
	if !v.IsKnown() || v.IsNull() || !v.Type().Is(tftypes.Object{}) {
		return nil, false
	}
	var attrs map[string]tftypes.Value
	if v.As(&attrs) != nil {
		return nil, false
	}
	return attrs, true
}

func sequenceElems(v tftypes.Value) ([]tftypes.Value, bool) {
	if !v.IsKnown() || v.IsNull() {
		return nil, false
	}
	var elems []tftypes.Value
	if v.As(&elems) != nil {
		return nil, false
	}
	return elems, true
}

func nullOrEmptyCollection(v tftypes.Value) bool {
	if !v.IsKnown() || v.Type() == nil {
		return false
	}
	t := v.Type()
	switch {
	case t.Is(tftypes.List{}), t.Is(tftypes.Tuple{}), t.Is(tftypes.Set{}):
		if v.IsNull() {
			return true
		}
		elems, ok := sequenceElems(v)
		return ok && len(elems) == 0
	case t.Is(tftypes.Map{}):
		if v.IsNull() {
			return true
		}
		var entries map[string]tftypes.Value
		if v.As(&entries) != nil {
			return false
		}
		return len(entries) == 0
	default:
		return false
	}
}
