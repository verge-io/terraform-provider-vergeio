// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNonEmptyRuleListConverts(t *testing.T) {
	ctx := t.Context()
	in := ruleWith(baseRule("allow-ssh"), func(rule *firewallRule) {
		rule.Key = 12
		rule.Protocol = "tcp"
		rule.Direction = "incoming"
		rule.Action = "accept"
		rule.DestinationPorts = "22"
		rule.OrderID = 1
	})

	list, diags := rulesToList(ctx, []firewallRule{in})
	if diags.HasError() {
		t.Fatalf("rulesToList: %v", diags)
	}
	if list.IsNull() || len(list.Elements()) != 1 {
		t.Fatalf("list = %#v", list)
	}
	obj, ok := list.Elements()[0].(types.Object)
	if !ok {
		t.Fatalf("element type = %T", list.Elements()[0])
	}
	attrs := obj.Attributes()
	if _, ok := attrs["vnet"]; ok {
		t.Fatal("nested rule object includes vnet")
	}
	if _, ok := attrs["apply"]; ok {
		t.Fatal("nested rule object includes apply")
	}

	got, diags := rulesFromList(ctx, list, false)
	if diags.HasError() {
		t.Fatalf("rulesFromList: %v", diags)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	rule := got[0]
	if rule.Key != 12 || rule.Name != "allow-ssh" || rule.Protocol != "tcp" || rule.Direction != "incoming" || rule.Action != "accept" || rule.DestinationPorts != "22" {
		t.Fatalf("rule = %#v", rule)
	}
}

func TestEmptyRuleListConverts(t *testing.T) {
	ctx := t.Context()
	list, diags := rulesToList(ctx, nil)
	if diags.HasError() {
		t.Fatalf("rulesToList: %v", diags)
	}
	if list.IsNull() || len(list.Elements()) != 0 {
		t.Fatalf("list = %#v", list)
	}
	got, diags := rulesFromList(ctx, list, false)
	if diags.HasError() {
		t.Fatalf("rulesFromList: %v", diags)
	}
	if len(got) != 0 {
		t.Fatalf("rules = %#v", got)
	}
}

func TestSingleRuleModelMatchesSchema(t *testing.T) {
	ctx := t.Context()
	var schemaResp resource.SchemaResponse
	NewNetworkRuleResource().Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	model := fillRuleModel(ruleWith(baseRule("allow-ssh"), func(rule *firewallRule) {
		rule.Key = 9
		rule.Protocol = "tcp"
		rule.DestinationPorts = "22"
		rule.OrderID = 1
	}), types.StringValue("19"), types.BoolValue(true))

	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, model)
	if diags.HasError() {
		t.Fatalf("state.Set: %v", diags)
	}
	var got firewallRuleModel
	diags = state.Get(ctx, &got)
	if diags.HasError() {
		t.Fatalf("state.Get: %v", diags)
	}
	if got.Name.ValueString() != "allow-ssh" || got.VNet.ValueString() != "19" || !got.Apply.ValueBool() || got.DestinationPorts.ValueString() != "22" {
		t.Fatalf("model = %#v", got)
	}
	if got.rule(true).DestinationPorts != "22" || got.rule(true).Name != "allow-ssh" {
		t.Fatalf("rule = %#v", got.rule(true))
	}
}

func TestRuleModelsMatchSchemas(t *testing.T) {
	ctx := t.Context()
	var listResp, singleResp resource.SchemaResponse
	NewNetworkRulesResource().Schema(ctx, resource.SchemaRequest{}, &listResp)
	NewNetworkRuleResource().Schema(ctx, resource.SchemaRequest{}, &singleResp)

	nested, ok := listResp.Schema.Attributes["rule"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("rule attribute type = %T", listResp.Schema.Attributes["rule"])
	}
	assertSameNames(t, "nested rule", schemaAttributeNames(nested.NestedObject.Attributes), tfsdkFieldNames(reflect.TypeOf(nestedFirewallRuleModel{})))
	assertSameNames(t, "single rule", schemaAttributeNames(singleResp.Schema.Attributes), tfsdkFieldNames(reflect.TypeOf(firewallRuleModel{})))
}

func schemaAttributeNames(attributes map[string]schema.Attribute) map[string]struct{} {
	names := make(map[string]struct{}, len(attributes))
	for name := range attributes {
		names[name] = struct{}{}
	}
	return names
}

func tfsdkFieldNames(typ reflect.Type) map[string]struct{} {
	names := make(map[string]struct{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Anonymous {
			for name := range tfsdkFieldNames(field.Type) {
				names[name] = struct{}{}
			}
			continue
		}
		tag := field.Tag.Get("tfsdk")
		if tag == "" || tag == "-" {
			continue
		}
		names[tag] = struct{}{}
	}
	return names
}

func assertSameNames(t *testing.T, label string, want, got map[string]struct{}) {
	t.Helper()
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("%s schema attribute %s is missing from the model", label, name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s model field %s is missing from the schema", label, name)
		}
	}
}
