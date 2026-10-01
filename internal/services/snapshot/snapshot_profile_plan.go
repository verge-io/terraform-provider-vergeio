// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ planmodifier.String = periodRefreshModifier{}
	_ planmodifier.Bool   = periodRefreshModifier{}
	_ planmodifier.Int32  = periodRefreshModifier{}
)

// periodRefreshModifier plans a nested period attribute so apply cannot
// disagree with the following Read.
//
// UseStateForUnknown only replaces an unknown plan value, and it uses the
// list index. A period added in an update is null in the plan, not unknown,
// and its index has no prior state. Read then stores the API key and the
// platform defaults (day_of_month 0, and the rest), which
// Terraform rejects as an inconsistent result.
//
// A configured value is left alone. An omitted value is copied from the
// state period with the same name, including a null that Read stored for a
// blank string, so the next plan stays empty. A period that is not in state
// is planned unknown. Unknown agrees with whatever Read writes, and an
// unknown value is not sent to VergeOS.
type periodRefreshModifier struct{}

func (m periodRefreshModifier) Description(context.Context) string {
	return "Plans a period attribute from the period with the same name, or unknown when VergeOS will assign it."
}

func (m periodRefreshModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m periodRefreshModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if kept, ok := keepConfiguredString(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	prior, found, diags := lookupPriorPeriodAttribute(ctx, req.State, req.Plan, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = refreshedString(prior, found)
}

func (m periodRefreshModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if kept, ok := keepConfiguredBool(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	prior, found, diags := lookupPriorPeriodAttribute(ctx, req.State, req.Plan, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = refreshedBool(prior, found)
}

func (m periodRefreshModifier) PlanModifyInt32(ctx context.Context, req planmodifier.Int32Request, resp *planmodifier.Int32Response) {
	if kept, ok := keepConfiguredInt32(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	prior, found, diags := lookupPriorPeriodAttribute(ctx, req.State, req.Plan, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = refreshedInt32(prior, found)
}

func keepConfiguredString(config types.String) (types.String, bool) {
	if config.IsNull() {
		return types.String{}, false
	}
	if config.IsUnknown() {
		return types.StringUnknown(), true
	}
	return config, true
}

func keepConfiguredBool(config types.Bool) (types.Bool, bool) {
	if config.IsNull() {
		return types.Bool{}, false
	}
	if config.IsUnknown() {
		return types.BoolUnknown(), true
	}
	return config, true
}

func keepConfiguredInt32(config types.Int32) (types.Int32, bool) {
	if config.IsNull() {
		return types.Int32{}, false
	}
	if config.IsUnknown() {
		return types.Int32Unknown(), true
	}
	return config, true
}

func refreshedString(prior attr.Value, found bool) types.String {
	value, ok := prior.(types.String)
	if !found || !ok || value.IsUnknown() {
		return types.StringUnknown()
	}
	return value
}

func refreshedBool(prior attr.Value, found bool) types.Bool {
	value, ok := prior.(types.Bool)
	if !found || !ok || value.IsUnknown() {
		return types.BoolUnknown()
	}
	return value
}

func refreshedInt32(prior attr.Value, found bool) types.Int32 {
	value, ok := prior.(types.Int32)
	if !found || !ok || value.IsUnknown() {
		return types.Int32Unknown()
	}
	return value
}

// lookupPriorPeriodAttribute returns the state value of this attribute on
// the period whose name matches the planned period. List index is not
// identity: reordering periods must not copy another period's key.
func lookupPriorPeriodAttribute(ctx context.Context, state tfsdk.State, plan tfsdk.Plan, attrPath path.Path) (attr.Value, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !stateReady(state) {
		return nil, false, diags
	}
	name, nameDiags := plannedPeriodName(ctx, plan, attrPath)
	diags.Append(nameDiags...)
	if diags.HasError() || name == "" {
		return nil, false, diags
	}
	var model SnapshotProfileResourceModel
	diags.Append(state.Get(ctx, &model)...)
	if diags.HasError() {
		return nil, false, diags
	}
	for _, period := range model.Period {
		if period.Name.IsNull() || period.Name.IsUnknown() || period.Name.ValueString() != name {
			continue
		}
		value, ok := periodAttribute(period, attributeName(attrPath))
		if !ok {
			diags.AddAttributeError(attrPath, "Unexpected period attribute", "The period plan modifier does not know attribute "+attributeName(attrPath)+".")
			return nil, false, diags
		}
		return value, true, diags
	}
	return nil, false, diags
}

func stateReady(state tfsdk.State) bool {
	if state.Raw.Type() == nil || state.Raw.IsNull() || !state.Raw.IsKnown() {
		return false
	}
	return true
}

func plannedPeriodName(ctx context.Context, plan tfsdk.Plan, attrPath path.Path) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	index, ok := periodIndex(attrPath)
	if !ok {
		diags.AddAttributeError(attrPath, "Unexpected period path", "The period plan modifier expected a list index.")
		return "", diags
	}
	if plan.Raw.Type() == nil || plan.Raw.IsNull() || !plan.Raw.IsKnown() {
		diags.AddAttributeError(attrPath, "Missing period plan", "The period plan modifier could not read the planned period name.")
		return "", diags
	}
	var model SnapshotProfileResourceModel
	diags.Append(plan.Get(ctx, &model)...)
	if diags.HasError() {
		return "", diags
	}
	if index < 0 || index >= len(model.Period) {
		diags.AddAttributeError(attrPath, "Unexpected period index", "The period plan modifier index is outside the planned period list.")
		return "", diags
	}
	name := model.Period[index].Name
	if name.IsNull() || name.IsUnknown() || name.ValueString() == "" {
		return "", diags
	}
	return name.ValueString(), diags
}

func periodIndex(attrPath path.Path) (int, bool) {
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

func periodAttribute(period periodModel, name string) (attr.Value, bool) {
	switch name {
	case "key":
		return period.Key, true
	case "day_of_week":
		return period.DayOfWeek, true
	case "max_tier":
		return period.MaxTier, true
	case "hour":
		return period.Hour, true
	case "minute":
		return period.Minute, true
	case "day_of_month":
		return period.DayOfMonth, true
	case "month":
		return period.Month, true
	case "min_snapshots":
		return period.MinSnapshots, true
	case "quiesce":
		return period.Quiesce, true
	case "immutable":
		return period.Immutable, true
	default:
		return nil, false
	}
}
