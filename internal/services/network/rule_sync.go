// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"fmt"
	"sort"
	"strings"

	"github.com/verge-io/govergeos"
)

// firewallRule is one non-system vnet_rules row the provider can write.
// Optional == nil means every field is authoritative. A non-nil Optional
// tells a single-rule update which omitted attributes to leave alone.
type firewallRule struct {
	Key              int
	VNet             int
	Name             string
	Description      string
	Direction        string
	Action           string
	Protocol         string
	Interface        string
	CTState          string
	SourceIP         string
	SourcePorts      string
	DestinationIP    string
	DestinationPorts string
	TargetIP         string
	TargetPorts      string
	Throttle         string
	Pin              string
	Enabled          bool
	Log              bool
	Statistics       bool
	Trace            bool
	DropThrottle     bool
	OrderID          int
	SystemRule       bool
	Optional         *optionalRuleFields
}

// optionalRuleFields is the set of single-rule attributes present in config.
// Defaults such as direction and enabled are always written and are not listed.
type optionalRuleFields struct {
	Description      bool
	CTState          bool
	SourceIP         bool
	SourcePorts      bool
	DestinationIP    bool
	DestinationPorts bool
	TargetIP         bool
	TargetPorts      bool
	Throttle         bool
	Pin              bool
	Order            bool
}

type ruleChangeKind int

const (
	ruleCreate ruleChangeKind = iota
	ruleUpdate
	ruleDelete
)

type ruleChange struct {
	Kind   ruleChangeKind
	Key    int
	Create *vergeos.VNetRuleCreateRequest
	Update *vergeos.VNetRuleUpdateRequest
}

type systemRuleError struct {
	Name string
}

func (e *systemRuleError) Error() string {
	return fmt.Sprintf("%q is a system rule and cannot be managed", e.Name)
}

type duplicateRuleError struct {
	Name string
}

func (e *duplicateRuleError) Error() string {
	return fmt.Sprintf("more than one firewall rule is named %q; refusing to guess", e.Name)
}

// decideFirewallApply chooses whether to refresh a network after rule writes.
// A stopped network is checked first: the API rejects the refresh, and the
// network loads staged rules when it starts. That matches Ansible vnet_apply.
type firewallApplyAction int

const (
	firewallApplyNone firewallApplyAction = iota
	firewallApplyNow
	firewallApplySkippedStopped
	firewallApplySkippedDisabled
)

func decideFirewallApply(wrote, apply, running, needFWApply bool) firewallApplyAction {
	if !running {
		if wrote || needFWApply {
			return firewallApplySkippedStopped
		}
		return firewallApplyNone
	}
	if !apply {
		if wrote || needFWApply {
			return firewallApplySkippedDisabled
		}
		return firewallApplyNone
	}
	if wrote || needFWApply {
		return firewallApplyNow
	}
	return firewallApplyNone
}

func normalizeRuleDefaults(rule firewallRule) firewallRule {
	if strings.TrimSpace(rule.Direction) == "" {
		rule.Direction = "incoming"
	}
	if strings.TrimSpace(rule.Action) == "" {
		rule.Action = "accept"
	}
	if strings.TrimSpace(rule.Protocol) == "" {
		rule.Protocol = "any"
	}
	if strings.TrimSpace(rule.Interface) == "" {
		rule.Interface = "auto"
	}
	if strings.TrimSpace(rule.Pin) == "" {
		rule.Pin = "no"
	}
	return rule
}

func ruleFromAPI(rule vergeos.VNetRule) firewallRule {
	return normalizeRuleDefaults(firewallRule{
		Key:              rule.Key.Int(),
		VNet:             rule.VNet.Int(),
		Name:             rule.Name,
		Description:      rule.Description,
		Direction:        rule.Direction,
		Action:           rule.Action,
		Protocol:         rule.Protocol,
		Interface:        rule.Interface,
		CTState:          rule.CTState,
		SourceIP:         rule.SourceIP,
		SourcePorts:      rule.SourcePorts,
		DestinationIP:    rule.DestinationIP,
		DestinationPorts: rule.DestinationPorts,
		TargetIP:         rule.TargetIP,
		TargetPorts:      rule.TargetPorts,
		Throttle:         rule.Throttle,
		Pin:              rule.Pin,
		Enabled:          rule.Enabled,
		Log:              rule.Log,
		Statistics:       rule.Statistics,
		Trace:            rule.Trace,
		DropThrottle:     rule.DropThrottle,
		OrderID:          rule.OrderID,
		SystemRule:       rule.SystemRule,
	})
}

func sortManagedRules(rules []firewallRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].OrderID != rules[j].OrderID {
			return rules[i].OrderID < rules[j].OrderID
		}
		if rules[i].Name != rules[j].Name {
			return rules[i].Name < rules[j].Name
		}
		return rules[i].Key < rules[j].Key
	})
}

func validateDesiredNames(desired []firewallRule) error {
	seen := make(map[string]struct{}, len(desired))
	for _, rule := range desired {
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return fmt.Errorf("firewall rule name is required")
		}
		if _, ok := seen[name]; ok {
			return &duplicateRuleError{Name: name}
		}
		seen[name] = struct{}{}
	}
	return nil
}

func findRuleByName(rules []vergeos.VNetRule, name string) (*vergeos.VNetRule, error) {
	var found *vergeos.VNetRule
	for i := range rules {
		if rules[i].Name != name {
			continue
		}
		if found != nil {
			return nil, &duplicateRuleError{Name: name}
		}
		copy := rules[i]
		found = &copy
	}
	return found, nil
}

// planRuleChanges diffs non-system rules on one network.
// The desired slice order is the rule order: orderid is assigned 1..n.
// System rules are never updated or deleted. A desired name that matches a
// system rule is an error, matching Ansible vnet_rule.
func planRuleChanges(vnetID int, current []vergeos.VNetRule, desired []firewallRule, authoritative bool) ([]ruleChange, error) {
	if err := validateDesiredNames(desired); err != nil {
		return nil, err
	}

	system := make(map[string]bool)
	managed := make(map[string]vergeos.VNetRule)
	var managedNames []string
	for _, rule := range current {
		if rule.SystemRule {
			system[rule.Name] = true
			continue
		}
		if _, exists := managed[rule.Name]; exists {
			return nil, &duplicateRuleError{Name: rule.Name}
		}
		managed[rule.Name] = rule
		managedNames = append(managedNames, rule.Name)
	}
	for _, rule := range desired {
		if system[strings.TrimSpace(rule.Name)] {
			return nil, &systemRuleError{Name: strings.TrimSpace(rule.Name)}
		}
	}

	planned := append([]firewallRule(nil), desired...)
	if authoritative {
		for i := range planned {
			planned[i].Name = strings.TrimSpace(planned[i].Name)
			planned[i].OrderID = i + 1
			planned[i].Optional = nil
			planned[i] = normalizeRuleDefaults(planned[i])
		}
	}

	var changes []ruleChange
	seen := make(map[string]bool, len(planned))
	for _, rule := range planned {
		seen[rule.Name] = true
		cur, ok := managed[rule.Name]
		if !ok {
			changes = append(changes, ruleChange{
				Kind:   ruleCreate,
				Create: createRequest(vnetID, rule),
			})
			continue
		}
		if req := updateRequest(ruleFromAPI(cur), rule); req != nil {
			changes = append(changes, ruleChange{
				Kind:   ruleUpdate,
				Key:    cur.Key.Int(),
				Update: req,
			})
		}
	}
	if !authoritative {
		return changes, nil
	}

	var deletes []ruleChange
	for _, name := range managedNames {
		if seen[name] {
			continue
		}
		deletes = append(deletes, ruleChange{
			Kind: ruleDelete,
			Key:  managed[name].Key.Int(),
		})
	}
	sort.Slice(deletes, func(i, j int) bool {
		return deletes[i].Key < deletes[j].Key
	})
	return append(changes, deletes...), nil
}

// planSingleRule creates or updates one rule matched by key, then by name.
// It does not delete any other rule.
func planSingleRule(vnetID int, current []vergeos.VNetRule, existingKey int, desired firewallRule) ([]ruleChange, error) {
	desired.Name = strings.TrimSpace(desired.Name)
	if desired.Name == "" {
		return nil, fmt.Errorf("firewall rule name is required")
	}
	desired = normalizeRuleDefaults(desired)

	var currentRule *vergeos.VNetRule
	if existingKey > 0 {
		for i := range current {
			if current[i].Key.Int() == existingKey {
				copy := current[i]
				currentRule = &copy
				break
			}
		}
	}
	if currentRule == nil {
		found, err := findRuleByName(current, desired.Name)
		if err != nil {
			return nil, err
		}
		currentRule = found
	} else if currentRule.Name != desired.Name {
		other, err := findRuleByName(current, desired.Name)
		if err != nil {
			return nil, err
		}
		if other != nil && other.Key.Int() != currentRule.Key.Int() {
			return nil, &duplicateRuleError{Name: desired.Name}
		}
	}
	if currentRule != nil && currentRule.SystemRule {
		return nil, &systemRuleError{Name: currentRule.Name}
	}
	for _, rule := range current {
		if rule.SystemRule && rule.Name == desired.Name {
			return nil, &systemRuleError{Name: desired.Name}
		}
	}
	if currentRule == nil {
		return []ruleChange{{
			Kind:   ruleCreate,
			Create: createRequest(vnetID, desired),
		}}, nil
	}
	if req := updateRequest(ruleFromAPI(*currentRule), desired); req != nil {
		return []ruleChange{{
			Kind:   ruleUpdate,
			Key:    currentRule.Key.Int(),
			Update: req,
		}}, nil
	}
	return nil, nil
}

func createRequest(vnetID int, rule firewallRule) *vergeos.VNetRuleCreateRequest {
	rule = normalizeRuleDefaults(rule)
	req := &vergeos.VNetRuleCreateRequest{
		VNet:         vnetID,
		Name:         rule.Name,
		Direction:    strPtr(rule.Direction),
		Action:       strPtr(rule.Action),
		Protocol:     strPtr(rule.Protocol),
		Interface:    strPtr(rule.Interface),
		Enabled:      boolPtr(rule.Enabled),
		Log:          boolPtr(rule.Log),
		Statistics:   boolPtr(rule.Statistics),
		Trace:        boolPtr(rule.Trace),
		DropThrottle: boolPtr(rule.DropThrottle),
	}
	all := rule.Optional == nil
	opt := rule.Optional
	include := func(field bool) bool {
		return all || (opt != nil && field)
	}
	if include(opt != nil && opt.Description) && rule.Description != "" {
		req.Description = rule.Description
	}
	put := func(on bool, value string, dst **string) {
		if !include(on) || value == "" {
			return
		}
		*dst = strPtr(value)
	}
	put(opt != nil && opt.CTState, rule.CTState, &req.CTState)
	put(opt != nil && opt.SourceIP, rule.SourceIP, &req.SourceIP)
	put(opt != nil && opt.SourcePorts, rule.SourcePorts, &req.SourcePorts)
	put(opt != nil && opt.DestinationIP, rule.DestinationIP, &req.DestinationIP)
	put(opt != nil && opt.DestinationPorts, rule.DestinationPorts, &req.DestinationPorts)
	put(opt != nil && opt.TargetIP, rule.TargetIP, &req.TargetIP)
	put(opt != nil && opt.TargetPorts, rule.TargetPorts, &req.TargetPorts)
	put(opt != nil && opt.Throttle, rule.Throttle, &req.Throttle)
	if all || (opt != nil && opt.Order) {
		order := rule.OrderID
		req.OrderID = &order
	}
	if (all || (opt != nil && opt.Pin)) && rule.Pin != "" && rule.Pin != "no" {
		req.Pin = strPtr(rule.Pin)
	}
	return req
}

func updateRequest(current, desired firewallRule) *vergeos.VNetRuleUpdateRequest {
	current = normalizeRuleDefaults(current)
	desired = normalizeRuleDefaults(desired)
	all := desired.Optional == nil
	opt := desired.Optional
	manage := func(on bool) bool {
		return all || on
	}
	on := func(field bool) bool {
		return opt != nil && field
	}

	req := &vergeos.VNetRuleUpdateRequest{}
	changed := false
	assignString(true, current.Name, desired.Name, &req.Name, &changed)
	assignString(true, current.Direction, desired.Direction, &req.Direction, &changed)
	assignString(true, current.Action, desired.Action, &req.Action, &changed)
	assignString(true, current.Protocol, desired.Protocol, &req.Protocol, &changed)
	assignString(true, current.Interface, desired.Interface, &req.Interface, &changed)
	assignBool(current.Enabled, desired.Enabled, &req.Enabled, &changed)
	assignBool(current.Log, desired.Log, &req.Log, &changed)
	assignBool(current.Statistics, desired.Statistics, &req.Statistics, &changed)
	assignBool(current.Trace, desired.Trace, &req.Trace, &changed)
	assignBool(current.DropThrottle, desired.DropThrottle, &req.DropThrottle, &changed)
	assignString(manage(on(opt != nil && opt.Description)), current.Description, desired.Description, &req.Description, &changed)
	assignString(manage(on(opt != nil && opt.CTState)), current.CTState, desired.CTState, &req.CTState, &changed)
	assignString(manage(on(opt != nil && opt.SourceIP)), current.SourceIP, desired.SourceIP, &req.SourceIP, &changed)
	assignString(manage(on(opt != nil && opt.SourcePorts)), current.SourcePorts, desired.SourcePorts, &req.SourcePorts, &changed)
	assignString(manage(on(opt != nil && opt.DestinationIP)), current.DestinationIP, desired.DestinationIP, &req.DestinationIP, &changed)
	assignString(manage(on(opt != nil && opt.DestinationPorts)), current.DestinationPorts, desired.DestinationPorts, &req.DestinationPorts, &changed)
	assignString(manage(on(opt != nil && opt.TargetIP)), current.TargetIP, desired.TargetIP, &req.TargetIP, &changed)
	assignString(manage(on(opt != nil && opt.TargetPorts)), current.TargetPorts, desired.TargetPorts, &req.TargetPorts, &changed)
	assignString(manage(on(opt != nil && opt.Throttle)), current.Throttle, desired.Throttle, &req.Throttle, &changed)
	assignString(manage(on(opt != nil && opt.Pin)), current.Pin, desired.Pin, &req.Pin, &changed)
	assignInt(manage(on(opt != nil && opt.Order)), current.OrderID, desired.OrderID, &req.OrderID, &changed)
	if !changed {
		return nil
	}
	return req
}

func assignString(manage bool, current, want string, dst **string, changed *bool) {
	if !manage || current == want {
		return
	}
	*dst = strPtr(want)
	*changed = true
}

func assignBool(current, want bool, dst **bool, changed *bool) {
	if current == want {
		return
	}
	*dst = boolPtr(want)
	*changed = true
}

func assignInt(manage bool, current, want int, dst **int, changed *bool) {
	if !manage || current == want {
		return
	}
	*dst = intPtr(want)
	*changed = true
}

func strPtr(v string) *string { return &v }

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }
