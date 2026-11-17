// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSyncPeriodPlanAgreesWithRead(t *testing.T) {
	ctx := context.Background()
	schema := outgoingSchema(t)
	nightly := refreshedSyncPeriod("9", "11", 604800, 5, true, "dr-")
	state := outgoingValue(t, schema, &outgoingModel{
		ID:     types.StringValue("4"),
		SiteID: types.StringValue("2"),
		Name:   types.StringValue("to-remote"),
		Period: []syncPeriodModel{nightly},
	})
	planned := &outgoingModel{
		ID:     types.StringValue("4"),
		SiteID: types.StringValue("2"),
		Name:   types.StringValue("to-remote"),
		Period: []syncPeriodModel{
			configuredSyncPeriod("9", 604800, 5, true, "dr-"),
			configuredSyncPeriod("10", 2419200, 1, false, "wk-"),
		},
	}
	config := outgoingValue(t, schema, planned)
	plan := outgoingValue(t, schema, planned)

	assertSyncUnknown(t, ctx, schema, config, plan, state, 1, "key")
	assertSyncUnknown(t, ctx, schema, config, plan, state, 1, "schedule_task")
	assertSyncUnknown(t, ctx, schema, config, plan, state, 1, "task")
	assertSyncString(t, ctx, schema, config, plan, state, 0, "key", "11")
	assertSyncInt64(t, ctx, schema, config, plan, state, 0, "schedule_task", 40)
	assertSyncInt64(t, ctx, schema, config, plan, state, 0, "task", 41)
	assertSyncInt64(t, ctx, schema, config, plan, state, 1, "priority", 1)
	assertSyncString(t, ctx, schema, config, plan, state, 1, "destination_prefix", "wk-")
	assertSyncBool(t, ctx, schema, config, plan, state, 1, "do_not_expire", false)
}

func TestSyncPeriodPlanMatchesByProfilePeriod(t *testing.T) {
	ctx := context.Background()
	schema := outgoingSchema(t)
	nightly := refreshedSyncPeriod("9", "11", 604800, 5, true, "dr-")
	weekly := refreshedSyncPeriod("10", "22", 2419200, 1, false, "wk-")
	state := outgoingValue(t, schema, &outgoingModel{
		ID:     types.StringValue("4"),
		SiteID: types.StringValue("2"),
		Name:   types.StringValue("to-remote"),
		Period: []syncPeriodModel{nightly, weekly},
	})
	reordered := &outgoingModel{
		ID:     types.StringValue("4"),
		SiteID: types.StringValue("2"),
		Name:   types.StringValue("to-remote"),
		Period: []syncPeriodModel{
			configuredSyncPeriod("10", 2419200, 1, false, "wk-"),
			configuredSyncPeriod("9", 604800, 5, true, "dr-"),
		},
	}
	config := outgoingValue(t, schema, reordered)
	plan := outgoingValue(t, schema, reordered)
	assertSyncString(t, ctx, schema, config, plan, state, 0, "key", "22")
	assertSyncString(t, ctx, schema, config, plan, state, 1, "key", "11")
	assertSyncInt64(t, ctx, schema, config, plan, state, 0, "schedule_task", 40)
}

func TestSyncPeriodPlanUnknownOnCreate(t *testing.T) {
	ctx := context.Background()
	schema := outgoingSchema(t)
	planned := &outgoingModel{
		SiteID: types.StringValue("2"),
		Name:   types.StringValue("to-remote"),
		Period: []syncPeriodModel{configuredSyncPeriod("9", 604800, 5, true, "dr-")},
	}
	config := outgoingValue(t, schema, planned)
	plan := outgoingValue(t, schema, planned)
	state := tfsdk.State{
		Schema: schema,
		Raw:    tftypes.NewValue(schema.Type().TerraformType(ctx), nil),
	}
	assertSyncUnknown(t, ctx, schema, config, plan, state, 0, "key")
	assertSyncUnknown(t, ctx, schema, config, plan, state, 0, "schedule_task")
	assertSyncInt64(t, ctx, schema, config, plan, state, 0, "priority", 5)
}

func outgoingSchema(t *testing.T) resschema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	NewSyncOutgoingResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	return resp.Schema
}

func outgoingValue(t *testing.T, schema resschema.Schema, model *outgoingModel) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: schema}
	diags := state.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return state
}

func configuredSyncPeriod(profilePeriod string, retention int64, priority int64, doNotExpire bool, prefix string) syncPeriodModel {
	return syncPeriodModel{
		ProfilePeriod:     types.StringValue(profilePeriod),
		Retention:         types.Int64Value(retention),
		Priority:          types.Int64Value(priority),
		DoNotExpire:       types.BoolValue(doNotExpire),
		DestinationPrefix: types.StringValue(prefix),
	}
}

func refreshedSyncPeriod(profilePeriod, key string, retention, priority int64, doNotExpire bool, prefix string) syncPeriodModel {
	period := configuredSyncPeriod(profilePeriod, retention, priority, doNotExpire, prefix)
	period.Key = types.StringValue(key)
	period.ScheduleTask = types.Int64Value(40)
	period.Task = types.Int64Value(41)
	return period
}

func assertSyncUnknown(t *testing.T, ctx context.Context, schema resschema.Schema, config, plan, state tfsdk.State, index int, name string) {
	t.Helper()
	got := planSyncAttribute(t, ctx, schema, config, plan, state, index, name)
	if got.IsNull() || !got.IsUnknown() {
		t.Fatalf("period[%d].%s plan = %s, want unknown", index, name, got)
	}
}

func assertSyncString(t *testing.T, ctx context.Context, schema resschema.Schema, config, plan, state tfsdk.State, index int, name, want string) {
	t.Helper()
	got := planSyncAttribute(t, ctx, schema, config, plan, state, index, name)
	value, ok := got.(types.String)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueString() != want {
		t.Fatalf("period[%d].%s plan = %s, want %q", index, name, got, want)
	}
}

func assertSyncBool(t *testing.T, ctx context.Context, schema resschema.Schema, config, plan, state tfsdk.State, index int, name string, want bool) {
	t.Helper()
	got := planSyncAttribute(t, ctx, schema, config, plan, state, index, name)
	value, ok := got.(types.Bool)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueBool() != want {
		t.Fatalf("period[%d].%s plan = %s, want %t", index, name, got, want)
	}
}

func assertSyncInt64(t *testing.T, ctx context.Context, schema resschema.Schema, config, plan, state tfsdk.State, index int, name string, want int64) {
	t.Helper()
	got := planSyncAttribute(t, ctx, schema, config, plan, state, index, name)
	value, ok := got.(types.Int64)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueInt64() != want {
		t.Fatalf("period[%d].%s plan = %s, want %d", index, name, got, want)
	}
}

func planSyncAttribute(t *testing.T, ctx context.Context, schema resschema.Schema, config, plan, state tfsdk.State, index int, name string) attr.Value {
	t.Helper()
	block := schema.Blocks["period"].(resschema.ListNestedBlock)
	attrPath := path.Root("period").AtListIndex(index).AtName(name)
	configData := tfsdk.Config{Schema: schema, Raw: config.Raw}
	planData := tfsdk.Plan{Schema: schema, Raw: plan.Raw}
	switch typed := block.NestedObject.Attributes[name].(type) {
	case resschema.StringAttribute:
		var configValue types.String
		if diags := configData.GetAttribute(ctx, attrPath, &configValue); diags.HasError() {
			t.Fatal(diags)
		}
		got := configValue
		for _, mod := range typed.PlanModifiers {
			resp := &planmodifier.StringResponse{PlanValue: got}
			mod.PlanModifyString(ctx, planmodifier.StringRequest{
				Config:      configData,
				ConfigValue: configValue,
				Plan:        planData,
				PlanValue:   got,
				State:       state,
				Path:        attrPath,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			got = resp.PlanValue
		}
		return got
	case resschema.BoolAttribute:
		var configValue types.Bool
		if diags := configData.GetAttribute(ctx, attrPath, &configValue); diags.HasError() {
			t.Fatal(diags)
		}
		got := configValue
		for _, mod := range typed.PlanModifiers {
			resp := &planmodifier.BoolResponse{PlanValue: got}
			mod.PlanModifyBool(ctx, planmodifier.BoolRequest{
				Config:      configData,
				ConfigValue: configValue,
				Plan:        planData,
				PlanValue:   got,
				State:       state,
				Path:        attrPath,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			got = resp.PlanValue
		}
		return got
	case resschema.Int64Attribute:
		var configValue types.Int64
		if diags := configData.GetAttribute(ctx, attrPath, &configValue); diags.HasError() {
			t.Fatal(diags)
		}
		got := configValue
		for _, mod := range typed.PlanModifiers {
			resp := &planmodifier.Int64Response{PlanValue: got}
			mod.PlanModifyInt64(ctx, planmodifier.Int64Request{
				Config:      configData,
				ConfigValue: configValue,
				Plan:        planData,
				PlanValue:   got,
				State:       state,
				Path:        attrPath,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			got = resp.PlanValue
		}
		return got
	default:
		t.Fatalf("period attribute %s has unexpected type %T", name, block.NestedObject.Attributes[name])
		return types.StringNull()
	}
}
