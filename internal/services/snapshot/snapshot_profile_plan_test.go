// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

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

// TestPeriodPlanAgreesWithRead is the unit stand-in for the acceptance
// failure on an update that adds a period. The new period's omitted
// attributes were null in the plan, and Read stored the API key and
// defaults. Those attributes must be unknown instead. An existing period
// keeps its state, matched by name, so the following plan is empty.
func TestPeriodPlanAgreesWithRead(t *testing.T) {
	ctx := context.Background()
	schema := snapshotProfileSchema(t)
	nightlyState := refreshedPeriod("nightly", "11", "daily", 2, 0, "", 604800, true)
	state := modelValue(t, schema, &SnapshotProfileResourceModel{
		Id:     types.StringValue("4"),
		Name:   types.StringValue("nightly"),
		Period: []periodModel{nightlyState},
	})
	planned := &SnapshotProfileResourceModel{
		Id:   types.StringValue("4"),
		Name: types.StringValue("nightly"),
		Period: []periodModel{
			configuredPeriod("nightly", "daily", 2, 0, "", 604800, true),
			configuredPeriod("weekly", "weekly", 1, 0, "sun", 2419200, true),
		},
	}
	config := modelValue(t, schema, planned)
	plan := modelValue(t, schema, planned)

	t.Run("new period is unknown", func(t *testing.T) {
		assertPeriodUnknown(t, ctx, schema, config, plan, state, 1, "key")
		assertPeriodUnknown(t, ctx, schema, config, plan, state, 1, "day_of_month")
		assertPeriodUnknown(t, ctx, schema, config, plan, state, 1, "immutable")
		assertPeriodUnknown(t, ctx, schema, config, plan, state, 1, "month")
		assertPeriodUnknown(t, ctx, schema, config, plan, state, 1, "max_tier")
		assertPeriodUnknown(t, ctx, schema, config, plan, state, 1, "min_snapshots")
		assertPeriodInt32(t, ctx, schema, config, plan, state, 1, "hour", 1)
		assertPeriodInt32(t, ctx, schema, config, plan, state, 1, "minute", 0)
		assertPeriodString(t, ctx, schema, config, plan, state, 1, "day_of_week", "sun")
		assertPeriodBool(t, ctx, schema, config, plan, state, 1, "quiesce", true)
	})

	t.Run("existing period keeps state", func(t *testing.T) {
		assertPeriodString(t, ctx, schema, config, plan, state, 0, "key", "11")
		assertPeriodInt32(t, ctx, schema, config, plan, state, 0, "day_of_month", 0)
		assertPeriodBool(t, ctx, schema, config, plan, state, 0, "immutable", false)
		assertPeriodInt32(t, ctx, schema, config, plan, state, 0, "month", 0)
		assertPeriodString(t, ctx, schema, config, plan, state, 0, "max_tier", "1")
		assertPeriodInt32(t, ctx, schema, config, plan, state, 0, "min_snapshots", 1)
		assertPeriodInt32(t, ctx, schema, config, plan, state, 0, "hour", 2)
		assertPeriodNullString(t, ctx, schema, config, plan, state, 0, "day_of_week")
	})

	t.Run("second plan stays known", func(t *testing.T) {
		weeklyState := refreshedPeriod("weekly", "22", "weekly", 1, 0, "sun", 2419200, true)
		refreshed := modelValue(t, schema, &SnapshotProfileResourceModel{
			Id:     types.StringValue("4"),
			Name:   types.StringValue("nightly"),
			Period: []periodModel{nightlyState, weeklyState},
		})
		assertPeriodString(t, ctx, schema, config, plan, refreshed, 1, "key", "22")
		assertPeriodInt32(t, ctx, schema, config, plan, refreshed, 1, "day_of_month", 0)
		assertPeriodBool(t, ctx, schema, config, plan, refreshed, 1, "immutable", false)
		assertPeriodInt32(t, ctx, schema, config, plan, refreshed, 1, "month", 0)
		assertPeriodString(t, ctx, schema, config, plan, refreshed, 1, "max_tier", "1")
		assertPeriodInt32(t, ctx, schema, config, plan, refreshed, 1, "min_snapshots", 1)
		assertPeriodBool(t, ctx, schema, config, plan, refreshed, 1, "quiesce", true)
	})
}

func TestPeriodPlanMatchesByName(t *testing.T) {
	ctx := context.Background()
	schema := snapshotProfileSchema(t)
	nightly := refreshedPeriod("nightly", "11", "daily", 2, 0, "", 604800, true)
	weekly := refreshedPeriod("weekly", "22", "weekly", 1, 0, "sun", 2419200, true)
	state := modelValue(t, schema, &SnapshotProfileResourceModel{
		Id:     types.StringValue("4"),
		Name:   types.StringValue("nightly"),
		Period: []periodModel{nightly, weekly},
	})
	reordered := &SnapshotProfileResourceModel{
		Id:   types.StringValue("4"),
		Name: types.StringValue("nightly"),
		Period: []periodModel{
			configuredPeriod("weekly", "weekly", 1, 0, "sun", 2419200, true),
			configuredPeriod("nightly", "daily", 2, 0, "", 604800, true),
		},
	}
	config := modelValue(t, schema, reordered)
	plan := modelValue(t, schema, reordered)

	assertPeriodString(t, ctx, schema, config, plan, state, 0, "key", "22")
	assertPeriodString(t, ctx, schema, config, plan, state, 1, "key", "11")
	assertPeriodInt32(t, ctx, schema, config, plan, state, 0, "min_snapshots", 1)
	assertPeriodString(t, ctx, schema, config, plan, state, 0, "max_tier", "1")
}

func TestPeriodPlanUnknownWhenNameReplacesIndex(t *testing.T) {
	ctx := context.Background()
	schema := snapshotProfileSchema(t)
	state := modelValue(t, schema, &SnapshotProfileResourceModel{
		Id:     types.StringValue("4"),
		Name:   types.StringValue("nightly"),
		Period: []periodModel{refreshedPeriod("nightly", "11", "daily", 2, 0, "", 604800, true)},
	})
	replaced := &SnapshotProfileResourceModel{
		Id:   types.StringValue("4"),
		Name: types.StringValue("nightly"),
		Period: []periodModel{
			configuredPeriod("weekly", "weekly", 1, 0, "sun", 2419200, true),
		},
	}
	config := modelValue(t, schema, replaced)
	plan := modelValue(t, schema, replaced)

	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "key")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "day_of_month")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "month")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "max_tier")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "min_snapshots")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "immutable")
}

func TestPeriodPlanUnknownOnCreate(t *testing.T) {
	ctx := context.Background()
	schema := snapshotProfileSchema(t)
	planned := &SnapshotProfileResourceModel{
		Name: types.StringValue("nightly"),
		Period: []periodModel{
			configuredPeriod("nightly", "daily", 2, 0, "", 604800, true),
		},
	}
	config := modelValue(t, schema, planned)
	plan := modelValue(t, schema, planned)
	state := tfsdk.State{
		Schema: schema,
		Raw:    tftypes.NewValue(schema.Type().TerraformType(ctx), nil),
	}

	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "key")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "day_of_month")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "immutable")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "month")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "max_tier")
	assertPeriodUnknown(t, ctx, schema, config, plan, state, 0, "min_snapshots")
	assertPeriodInt32(t, ctx, schema, config, plan, state, 0, "hour", 2)
}

func snapshotProfileSchema(t *testing.T) resschema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	NewSnapshotProfileResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	return resp.Schema
}

func modelValue(t *testing.T, schema resschema.Schema, model *SnapshotProfileResourceModel) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: schema}
	diags := state.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return state
}

func configuredPeriod(name, frequency string, hour, minute int32, day string, retention int64, quiesce bool) periodModel {
	period := periodModel{
		Name:      types.StringValue(name),
		Frequency: types.StringValue(frequency),
		Hour:      types.Int32Value(hour),
		Minute:    types.Int32Value(minute),
		Retention: types.Int64Value(retention),
		Quiesce:   types.BoolValue(quiesce),
	}
	if day != "" {
		period.DayOfWeek = types.StringValue(day)
	}
	return period
}

func refreshedPeriod(name, key, frequency string, hour, minute int32, day string, retention int64, quiesce bool) periodModel {
	period := configuredPeriod(name, frequency, hour, minute, day, retention, quiesce)
	period.Key = types.StringValue(key)
	period.DayOfMonth = types.Int32Value(0)
	period.Month = types.Int32Value(0)
	period.Immutable = types.BoolValue(false)
	period.MaxTier = types.StringValue("1")
	period.MinSnapshots = types.Int32Value(1)
	if day == "" {
		period.DayOfWeek = types.StringNull()
	}
	return period
}

func assertPeriodUnknown(t *testing.T, ctx context.Context, schema resschema.Schema, config tfsdk.State, plan tfsdk.State, state tfsdk.State, index int, name string) {
	t.Helper()
	got := planPeriodAttribute(t, ctx, schema, config, plan, state, index, name)
	if got.IsNull() || !got.IsUnknown() {
		t.Fatalf("period[%d].%s plan = %s, want unknown", index, name, got)
	}
}

func assertPeriodString(t *testing.T, ctx context.Context, schema resschema.Schema, config tfsdk.State, plan tfsdk.State, state tfsdk.State, index int, name, want string) {
	t.Helper()
	got := planPeriodAttribute(t, ctx, schema, config, plan, state, index, name)
	value, ok := got.(types.String)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueString() != want {
		t.Fatalf("period[%d].%s plan = %s, want %q", index, name, got, want)
	}
}

func assertPeriodNullString(t *testing.T, ctx context.Context, schema resschema.Schema, config tfsdk.State, plan tfsdk.State, state tfsdk.State, index int, name string) {
	t.Helper()
	got := planPeriodAttribute(t, ctx, schema, config, plan, state, index, name)
	value, ok := got.(types.String)
	if !ok || !value.IsNull() || value.IsUnknown() {
		t.Fatalf("period[%d].%s plan = %s, want null", index, name, got)
	}
}

func assertPeriodBool(t *testing.T, ctx context.Context, schema resschema.Schema, config tfsdk.State, plan tfsdk.State, state tfsdk.State, index int, name string, want bool) {
	t.Helper()
	got := planPeriodAttribute(t, ctx, schema, config, plan, state, index, name)
	value, ok := got.(types.Bool)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueBool() != want {
		t.Fatalf("period[%d].%s plan = %s, want %t", index, name, got, want)
	}
}

func assertPeriodInt32(t *testing.T, ctx context.Context, schema resschema.Schema, config tfsdk.State, plan tfsdk.State, state tfsdk.State, index int, name string, want int32) {
	t.Helper()
	got := planPeriodAttribute(t, ctx, schema, config, plan, state, index, name)
	value, ok := got.(types.Int32)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueInt32() != want {
		t.Fatalf("period[%d].%s plan = %s, want %d", index, name, got, want)
	}
}

func planPeriodAttribute(t *testing.T, ctx context.Context, schema resschema.Schema, config tfsdk.State, plan tfsdk.State, state tfsdk.State, index int, name string) attr.Value {
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
	case resschema.Int32Attribute:
		var configValue types.Int32
		if diags := configData.GetAttribute(ctx, attrPath, &configValue); diags.HasError() {
			t.Fatal(diags)
		}
		got := configValue
		for _, mod := range typed.PlanModifiers {
			resp := &planmodifier.Int32Response{PlanValue: got}
			mod.PlanModifyInt32(ctx, planmodifier.Int32Request{
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
