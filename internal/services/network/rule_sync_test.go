// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"errors"
	"strings"
	"testing"

	"github.com/verge-io/govergeos"
)

func TestDecideFirewallApply(t *testing.T) {
	tests := []struct {
		name        string
		wrote       bool
		apply       bool
		running     bool
		needFWApply bool
		want        firewallApplyAction
	}{
		{name: "nothing to do", apply: true, running: true, want: firewallApplyNone},
		{name: "write on a running network", wrote: true, apply: true, running: true, want: firewallApplyNow},
		{name: "pending flag on a running network", apply: true, running: true, needFWApply: true, want: firewallApplyNow},
		{name: "stopped after a write", wrote: true, apply: true, want: firewallApplySkippedStopped},
		{name: "stopped with a pending flag", apply: true, needFWApply: true, want: firewallApplySkippedStopped},
		{name: "stopped and idle", apply: true, want: firewallApplyNone},
		{name: "apply disabled after a write", wrote: true, running: true, want: firewallApplySkippedDisabled},
		{name: "apply disabled while pending", running: true, needFWApply: true, want: firewallApplySkippedDisabled},
		{name: "apply disabled and idle", running: true, want: firewallApplyNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideFirewallApply(tt.wrote, tt.apply, tt.running, tt.needFWApply)
			if got != tt.want {
				t.Fatalf("decision = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStoppedFirewallNotice(t *testing.T) {
	notice := stoppedFirewallNotice(&vergeos.Network{Name: "lan"}, 3)
	if notice.Summary == "" || !strings.Contains(notice.Detail, "not running") || !strings.Contains(notice.Detail, "starts") {
		t.Fatalf("notice = %#v", notice)
	}
	if !strings.Contains(notice.Detail, "lan (id 3)") {
		t.Fatalf("detail = %q", notice.Detail)
	}
}

func TestPlanRuleChangesNoWriteWhenEqual(t *testing.T) {
	current := []vergeos.VNetRule{
		apiRule(12, "allow-ssh", 1),
		apiRule(13, "allow-https", 2),
		systemRule(1, "builtin"),
	}
	desired := []firewallRule{baseRule("allow-ssh"), baseRule("allow-https")}
	changes, err := planRuleChanges(3, current, desired, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %#v, want none", changes)
	}
}

func TestPlanRuleChangesCreatesUpdatesAndDeletesOnce(t *testing.T) {
	current := []vergeos.VNetRule{
		apiRule(12, "allow-ssh", 5),
		apiRule(13, "stale", 9),
		systemRule(1, "builtin"),
	}
	current[0].DestinationPorts = "22"
	desired := []firewallRule{
		ruleWith(baseRule("allow-ssh"), func(rule *firewallRule) { rule.DestinationPorts = "2222" }),
		ruleWith(baseRule("allow-https"), func(rule *firewallRule) { rule.DestinationPorts = "443" }),
	}
	changes, err := planRuleChanges(3, current, desired, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("got %d changes, want update, create, delete", len(changes))
	}
	if changes[0].Kind != ruleUpdate || changes[0].Key != 12 || changes[0].Update.DestinationPorts == nil || *changes[0].Update.DestinationPorts != "2222" {
		t.Fatalf("update = %#v", changes[0])
	}
	if changes[0].Update.OrderID == nil || *changes[0].Update.OrderID != 1 {
		t.Fatalf("orderid = %#v, want 1", changes[0].Update.OrderID)
	}
	if changes[1].Kind != ruleCreate || changes[1].Create.Name != "allow-https" || changes[1].Create.OrderID == nil || *changes[1].Create.OrderID != 2 {
		t.Fatalf("create = %#v", changes[1].Create)
	}
	if changes[2].Kind != ruleDelete || changes[2].Key != 13 {
		t.Fatalf("delete = %#v", changes[2])
	}
}

func TestPlanRuleChangesRefusesSystemRuleName(t *testing.T) {
	current := []vergeos.VNetRule{systemRule(1, "builtin"), apiRule(12, "extra", 1)}
	_, err := planRuleChanges(3, current, []firewallRule{baseRule("builtin")}, true)
	var systemErr *systemRuleError
	if !errors.As(err, &systemErr) {
		t.Fatalf("err = %v, want system rule", err)
	}
}

func TestPlanRuleChangesDeletesNonSystemOnly(t *testing.T) {
	current := []vergeos.VNetRule{systemRule(1, "builtin"), apiRule(12, "extra", 1)}
	changes, err := planRuleChanges(3, current, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != ruleDelete || changes[0].Key != 12 {
		t.Fatalf("changes = %#v", changes)
	}
}

func TestPlanRuleChangesRejectsDuplicateNames(t *testing.T) {
	_, err := planRuleChanges(3, nil, []firewallRule{baseRule("allow-ssh"), baseRule("allow-ssh")}, true)
	var dup *duplicateRuleError
	if !errors.As(err, &dup) {
		t.Fatalf("err = %v, want duplicate", err)
	}
}

func TestPlanSingleRuleLeavesOmittedSource(t *testing.T) {
	current := []vergeos.VNetRule{apiRule(12, "allow-ssh", 4)}
	current[0].SourceIP = "192.0.2.0/24"
	desired := baseRule("allow-ssh")
	desired.Optional = &optionalRuleFields{}
	changes, err := planSingleRule(3, current, 12, desired)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %#v, want none", changes)
	}
}

func TestPlanSingleRuleUpdatesConfiguredSource(t *testing.T) {
	current := []vergeos.VNetRule{apiRule(12, "allow-ssh", 4)}
	current[0].SourceIP = "192.0.2.0/24"
	desired := baseRule("allow-ssh")
	desired.SourceIP = "192.0.2.10"
	desired.Optional = &optionalRuleFields{SourceIP: true}
	changes, err := planSingleRule(3, current, 12, desired)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Update == nil || changes[0].Update.SourceIP == nil || *changes[0].Update.SourceIP != "192.0.2.10" {
		t.Fatalf("changes = %#v", changes)
	}
	if changes[0].Update.OrderID != nil {
		t.Fatal("omitted orderid was sent")
	}
}

func TestPlanSingleRuleRefusesSystemRule(t *testing.T) {
	current := []vergeos.VNetRule{systemRule(9, "builtin")}
	_, err := planSingleRule(3, current, 9, baseRule("builtin"))
	var systemErr *systemRuleError
	if !errors.As(err, &systemErr) {
		t.Fatalf("err = %v, want system rule", err)
	}
}

func TestPlanSingleRuleRenamesInPlace(t *testing.T) {
	current := []vergeos.VNetRule{apiRule(12, "allow-ssh", 1)}
	desired := baseRule("allow-ssh-mgmt")
	desired.Optional = &optionalRuleFields{}
	changes, err := planSingleRule(3, current, 12, desired)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != ruleUpdate || changes[0].Key != 12 || changes[0].Update.Name == nil || *changes[0].Update.Name != "allow-ssh-mgmt" {
		t.Fatalf("changes = %#v", changes)
	}
}

func TestUpdateRequestClearsAuthoritativeDescription(t *testing.T) {
	current := baseRule("allow-ssh")
	current.Description = "old"
	desired := baseRule("allow-ssh")
	req := updateRequest(current, desired)
	if req == nil || req.Description == nil || *req.Description != "" {
		t.Fatalf("update = %#v", req)
	}
}

func TestCreateRequestSendsDisabledRule(t *testing.T) {
	rule := baseRule("allow-ssh")
	rule.Enabled = false
	rule.DestinationPorts = "22"
	req := createRequest(3, rule)
	if req.Enabled == nil || *req.Enabled {
		t.Fatalf("enabled = %#v", req.Enabled)
	}
	if req.DestinationPorts == nil || *req.DestinationPorts != "22" {
		t.Fatalf("ports = %#v", req.DestinationPorts)
	}
	if req.VNet != 3 {
		t.Fatalf("vnet = %d", req.VNet)
	}
}

func apiRule(key int, name string, order int) vergeos.VNetRule {
	return vergeos.VNetRule{
		Key:       vergeos.FlexInt(key),
		VNet:      vergeos.FlexInt(3),
		Name:      name,
		Direction: "incoming",
		Action:    "accept",
		Protocol:  "any",
		Interface: "auto",
		Enabled:   true,
		Pin:       "no",
		OrderID:   order,
	}
}

func systemRule(key int, name string) vergeos.VNetRule {
	rule := apiRule(key, name, 0)
	rule.SystemRule = true
	return rule
}

func baseRule(name string) firewallRule {
	return normalizeRuleDefaults(firewallRule{Name: name, Enabled: true})
}

func ruleWith(rule firewallRule, edit func(*firewallRule)) firewallRule {
	edit(&rule)
	return rule
}
