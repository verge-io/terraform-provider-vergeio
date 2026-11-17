// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

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
	_ planmodifier.String = syncPeriodModifier{}
	_ planmodifier.Bool   = syncPeriodModifier{}
	_ planmodifier.Int64  = syncPeriodModifier{}
)

// syncPeriodModifier plans a nested site sync period attribute from the
// period with the same profile_period.
//
// UseStateForUnknown copies by list index. Reordering periods would copy
// another period's key, and a period added in an update has no prior index.
// A configured value is left alone. An omitted value is copied from the
// state period with the same profile_period. A period that is not in state
// is planned unknown, which agrees with whatever the following read stores.
type syncPeriodModifier struct{}

func (m syncPeriodModifier) Description(context.Context) string {
	return "Plans a site sync period attribute from the period with the same profile_period, or unknown when VergeOS will assign it."
}

func (m syncPeriodModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m syncPeriodModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if kept, ok := keepConfiguredString(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	prior, found, diags := lookupPriorSyncPeriodAttribute(ctx, req.State, req.Plan, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = refreshedString(prior, found)
}

func (m syncPeriodModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if kept, ok := keepConfiguredBool(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	prior, found, diags := lookupPriorSyncPeriodAttribute(ctx, req.State, req.Plan, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = refreshedBool(prior, found)
}

func (m syncPeriodModifier) PlanModifyInt64(ctx context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if kept, ok := keepConfiguredInt64(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	prior, found, diags := lookupPriorSyncPeriodAttribute(ctx, req.State, req.Plan, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = refreshedInt64(prior, found)
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

func keepConfiguredInt64(config types.Int64) (types.Int64, bool) {
	if config.IsNull() {
		return types.Int64{}, false
	}
	if config.IsUnknown() {
		return types.Int64Unknown(), true
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

func refreshedInt64(prior attr.Value, found bool) types.Int64 {
	value, ok := prior.(types.Int64)
	if !found || !ok || value.IsUnknown() {
		return types.Int64Unknown()
	}
	return value
}

func lookupPriorSyncPeriodAttribute(ctx context.Context, state tfsdk.State, plan tfsdk.Plan, attrPath path.Path) (attr.Value, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !stateReady(state) {
		return nil, false, diags
	}
	profilePeriod, nameDiags := plannedProfilePeriod(ctx, plan, attrPath)
	diags.Append(nameDiags...)
	if diags.HasError() || profilePeriod == "" {
		return nil, false, diags
	}
	var model outgoingModel
	diags.Append(state.Get(ctx, &model)...)
	if diags.HasError() {
		return nil, false, diags
	}
	for _, period := range model.Period {
		if !setString(period.ProfilePeriod) || period.ProfilePeriod.ValueString() != profilePeriod {
			continue
		}
		value, ok := syncPeriodAttribute(period, attributeName(attrPath))
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

func plannedProfilePeriod(ctx context.Context, plan tfsdk.Plan, attrPath path.Path) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	index, ok := periodIndex(attrPath)
	if !ok {
		diags.AddAttributeError(attrPath, "Unexpected period path", "The period plan modifier expected a list index.")
		return "", diags
	}
	if plan.Raw.Type() == nil || plan.Raw.IsNull() || !plan.Raw.IsKnown() {
		diags.AddAttributeError(attrPath, "Missing period plan", "The period plan modifier could not read the planned profile_period.")
		return "", diags
	}
	var model outgoingModel
	diags.Append(plan.Get(ctx, &model)...)
	if diags.HasError() {
		return "", diags
	}
	if index < 0 || index >= len(model.Period) {
		diags.AddAttributeError(attrPath, "Unexpected period index", "The period plan modifier index is outside the planned period list.")
		return "", diags
	}
	profilePeriod := model.Period[index].ProfilePeriod
	if !setString(profilePeriod) {
		return "", diags
	}
	return profilePeriod.ValueString(), diags
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

func syncPeriodAttribute(period syncPeriodModel, name string) (attr.Value, bool) {
	switch name {
	case "key":
		return period.Key, true
	case "schedule_task":
		return period.ScheduleTask, true
	case "task":
		return period.Task, true
	case "priority":
		return period.Priority, true
	case "do_not_expire":
		return period.DoNotExpire, true
	case "destination_prefix":
		return period.DestinationPrefix, true
	default:
		return nil, false
	}
}
