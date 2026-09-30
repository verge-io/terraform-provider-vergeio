// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ruleSchemaMode selects how omitted attributes are planned.
// The list resource owns every field. The single-rule resource leaves an
// omitted optional field unchanged, matching Ansible vnet_rule.
type ruleSchemaMode int

const (
	ruleSchemaList ruleSchemaMode = iota
	ruleSchemaSingle
)

var (
	ruleDirections = []string{"incoming", "outgoing"}
	ruleActions    = []string{"accept", "drop", "reject", "translate", "route"}
	ruleProtocols  = []string{"tcp", "udp", "tcpudp", "icmp", "any", "2", "47", "50", "51", "89"}
	ruleInterfaces = []string{"auto", "router", "dmz", "wireguard", "any"}
	rulePins       = []string{"no", "top", "bottom"}
	aliasScopes    = []string{"private", "global", "tenant", "none"}
)

// firewallRuleModel is one rule in state. The list resource nests it.
// The single-rule resource adds vnet and apply on the same struct.
type firewallRuleModel struct {
	ID               types.String `tfsdk:"id"`
	VNet             types.String `tfsdk:"vnet"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	Direction        types.String `tfsdk:"direction"`
	Action           types.String `tfsdk:"action"`
	Protocol         types.String `tfsdk:"protocol"`
	Interface        types.String `tfsdk:"interface"`
	CTState          types.String `tfsdk:"ct_state"`
	SourceIP         types.String `tfsdk:"source_ip"`
	SourcePorts      types.String `tfsdk:"source_ports"`
	DestinationIP    types.String `tfsdk:"destination_ip"`
	DestinationPorts types.String `tfsdk:"destination_ports"`
	TargetIP         types.String `tfsdk:"target_ip"`
	TargetPorts      types.String `tfsdk:"target_ports"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	Log              types.Bool   `tfsdk:"log"`
	Statistics       types.Bool   `tfsdk:"statistics"`
	Trace            types.Bool   `tfsdk:"trace"`
	Throttle         types.String `tfsdk:"throttle"`
	DropThrottle     types.Bool   `tfsdk:"drop_throttle"`
	Pin              types.String `tfsdk:"pin"`
	OrderID          types.Int32  `tfsdk:"orderid"`
	Apply            types.Bool   `tfsdk:"apply"`
}

func firewallRuleAttributes(mode ruleSchemaMode) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "Rule id, the vnet_rules key.",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Rule name, unique on the network. Rules are matched by name.",
			Required:            true,
		},
		"description":       ruleFreeString("Rule description.", mode),
		"direction":         ruleEnum("Traffic direction. Defaults to incoming.", "incoming", ruleDirections),
		"action":            ruleEnum("What matching traffic does. The API field action. Defaults to accept. translate and route use target_ip and target_ports.", "accept", ruleActions),
		"protocol":          ruleEnum("Protocol to match. Defaults to any. Numeric values are OSPF (89), IGMP (2), GRE (47), ESP (50), and AH (51).", "any", ruleProtocols),
		"interface":         ruleEnum("Interface the rule binds to. Defaults to auto.", "auto", ruleInterfaces),
		"ct_state":          ruleFreeString("Connection tracking state filter, for example new or established.", mode),
		"source_ip":         ruleFreeString("Source address filter. A literal, a special value such as vnetself, or alias:<name>.", mode),
		"source_ports":      ruleFreeString("Source port filter, for example 22 or 80,443. An alias is written alias:<name>.", mode),
		"destination_ip":    ruleFreeString("Destination address filter. A literal, a special value, or alias:<name>.", mode),
		"destination_ports": ruleFreeString("Destination port filter. An alias is written alias:<name>.", mode),
		"target_ip":         ruleFreeString("Target address for translate and route actions.", mode),
		"target_ports":      ruleFreeString("Target port for translate.", mode),
		"enabled":           ruleBool("Whether the rule is enabled. Defaults to true.", true),
		"log":               ruleBool("Log traffic that matches the rule. Defaults to false.", false),
		"statistics":        ruleBool("Track packet and byte counters for the rule. Defaults to false.", false),
		"trace":             ruleBool("Trace packets that match the rule. Defaults to false.", false),
		"throttle":          ruleFreeString("Rate limit, for example 1000 kbytes/second.", mode),
		"drop_throttle":     ruleBool("Add a drop rule when throttle is exceeded. Defaults to false.", false),
		"pin":               rulePin(mode),
		"orderid":           ruleOrder(mode),
	}
}

func ruleEnum(desc, def string, values []string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString(def),
		Validators: []validator.String{
			stringvalidator.OneOf(values...),
		},
	}
}

func ruleBool(desc string, def bool) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(def),
	}
}

func ruleFreeString(desc string, mode ruleSchemaMode) schema.StringAttribute {
	attr := schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
	}
	// Computed is required for a default. On the list resource the default is
	// empty, so an omitted value is cleared. On a single rule there is no
	// default: an omitted value stays at whatever the network already has.
	attr.Computed = true
	if mode == ruleSchemaList {
		attr.Default = stringdefault.StaticString("")
	}
	return attr
}

func rulePin(mode ruleSchemaMode) schema.StringAttribute {
	attr := schema.StringAttribute{
		MarkdownDescription: "Pin the rule to the top or bottom of the non-system rules. no leaves it in orderid order.",
		Optional:            true,
		Validators: []validator.String{
			stringvalidator.OneOf(rulePins...),
		},
	}
	if mode == ruleSchemaSingle {
		attr.Computed = true
		return attr
	}
	attr.Computed = true
	attr.Default = stringdefault.StaticString("no")
	return attr
}

func ruleOrder(mode ruleSchemaMode) schema.Attribute {
	if mode == ruleSchemaSingle {
		return schema.Int32Attribute{
			MarkdownDescription: "Explicit orderid. Omit to leave the current position unchanged. Lower is earlier.",
			Optional:            true,
			Computed:            true,
		}
	}
	return schema.Int32Attribute{
		MarkdownDescription: "Position assigned from the rule list, starting at 1. Lower is earlier.",
		Computed:            true,
		PlanModifiers: []planmodifier.Int32{
			int32planmodifier.UseStateForUnknown(),
		},
	}
}

func ruleAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":                types.StringType,
		"name":              types.StringType,
		"description":       types.StringType,
		"direction":         types.StringType,
		"action":            types.StringType,
		"protocol":          types.StringType,
		"interface":         types.StringType,
		"ct_state":          types.StringType,
		"source_ip":         types.StringType,
		"source_ports":      types.StringType,
		"destination_ip":    types.StringType,
		"destination_ports": types.StringType,
		"target_ip":         types.StringType,
		"target_ports":      types.StringType,
		"enabled":           types.BoolType,
		"log":               types.BoolType,
		"statistics":        types.BoolType,
		"trace":             types.BoolType,
		"throttle":          types.StringType,
		"drop_throttle":     types.BoolType,
		"pin":               types.StringType,
		"orderid":           types.Int32Type,
	}
}

func ruleObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: ruleAttrTypes()}
}

func (rule firewallRule) model() firewallRuleModel {
	return firewallRuleModel{
		ID:               idValue(rule.Key),
		Name:             types.StringValue(rule.Name),
		Description:      types.StringValue(rule.Description),
		Direction:        types.StringValue(rule.Direction),
		Action:           types.StringValue(rule.Action),
		Protocol:         types.StringValue(rule.Protocol),
		Interface:        types.StringValue(rule.Interface),
		CTState:          types.StringValue(rule.CTState),
		SourceIP:         types.StringValue(rule.SourceIP),
		SourcePorts:      types.StringValue(rule.SourcePorts),
		DestinationIP:    types.StringValue(rule.DestinationIP),
		DestinationPorts: types.StringValue(rule.DestinationPorts),
		TargetIP:         types.StringValue(rule.TargetIP),
		TargetPorts:      types.StringValue(rule.TargetPorts),
		Enabled:          types.BoolValue(rule.Enabled),
		Log:              types.BoolValue(rule.Log),
		Statistics:       types.BoolValue(rule.Statistics),
		Trace:            types.BoolValue(rule.Trace),
		Throttle:         types.StringValue(rule.Throttle),
		DropThrottle:     types.BoolValue(rule.DropThrottle),
		Pin:              types.StringValue(rule.Pin),
		OrderID:          types.Int32Value(int32(rule.OrderID)),
	}
}

func rulesToList(ctx context.Context, rules []firewallRule) (types.List, diag.Diagnostics) {
	models := make([]firewallRuleModel, 0, len(rules))
	for _, rule := range rules {
		models = append(models, rule.model())
	}
	return types.ListValueFrom(ctx, ruleObjectType(), models)
}

func rulesFromList(ctx context.Context, list types.List, single bool) ([]firewallRule, diag.Diagnostics) {
	if list.IsNull() || list.IsUnknown() {
		return nil, nil
	}
	var models []firewallRuleModel
	diags := list.ElementsAs(ctx, &models, false)
	if diags.HasError() {
		return nil, diags
	}
	rules := make([]firewallRule, 0, len(models))
	for _, model := range models {
		rules = append(rules, model.rule(single))
	}
	return rules, nil
}

// rule converts a planned rule. single uses cfg-style nulls already stored on
// the model: the caller passes the config model for a single rule and the
// plan model for a list, where defaults have filled omitted values.
func (model firewallRuleModel) rule(single bool) firewallRule {
	rule := firewallRule{
		Key:              ruleKey(model.ID),
		Name:             strings.TrimSpace(stringOrEmpty(model.Name)),
		Description:      stringOrEmpty(model.Description),
		Direction:        stringOr(model.Direction, "incoming"),
		Action:           stringOr(model.Action, "accept"),
		Protocol:         stringOr(model.Protocol, "any"),
		Interface:        stringOr(model.Interface, "auto"),
		CTState:          stringOrEmpty(model.CTState),
		SourceIP:         stringOrEmpty(model.SourceIP),
		SourcePorts:      stringOrEmpty(model.SourcePorts),
		DestinationIP:    stringOrEmpty(model.DestinationIP),
		DestinationPorts: stringOrEmpty(model.DestinationPorts),
		TargetIP:         stringOrEmpty(model.TargetIP),
		TargetPorts:      stringOrEmpty(model.TargetPorts),
		Throttle:         stringOrEmpty(model.Throttle),
		Enabled:          boolOr(model.Enabled, true),
		Log:              boolOr(model.Log, false),
		Statistics:       boolOr(model.Statistics, false),
		Trace:            boolOr(model.Trace, false),
		DropThrottle:     boolOr(model.DropThrottle, false),
		Pin:              stringOrEmpty(model.Pin),
	}
	if !single {
		return normalizeRuleDefaults(rule)
	}
	rule.Optional = &optionalRuleFields{
		Description:      knownString(model.Description),
		CTState:          knownString(model.CTState),
		SourceIP:         knownString(model.SourceIP),
		SourcePorts:      knownString(model.SourcePorts),
		DestinationIP:    knownString(model.DestinationIP),
		DestinationPorts: knownString(model.DestinationPorts),
		TargetIP:         knownString(model.TargetIP),
		TargetPorts:      knownString(model.TargetPorts),
		Throttle:         knownString(model.Throttle),
		Pin:              knownString(model.Pin),
		Order:            knownInt(model.OrderID),
	}
	if rule.Optional.Order {
		rule.OrderID = int(model.OrderID.ValueInt32())
	}
	return normalizeRuleDefaults(rule)
}

func knownString(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func knownInt(v types.Int32) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func stringOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

func stringOr(v types.String, fallback string) string {
	if v.IsNull() || v.IsUnknown() || strings.TrimSpace(v.ValueString()) == "" {
		return fallback
	}
	return v.ValueString()
}

func boolOr(v types.Bool, fallback bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return fallback
	}
	return v.ValueBool()
}

func idValue(key int) types.String {
	if key <= 0 {
		return types.StringNull()
	}
	return types.StringValue(strconv.Itoa(key))
}

func ruleKey(id types.String) int {
	if id.IsNull() || id.IsUnknown() {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(id.ValueString()))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func parsePositiveID(v string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("expected a positive integer id, got %q", v)
	}
	return id, nil
}

func applyEnabled(v types.Bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return true
	}
	return v.ValueBool()
}
