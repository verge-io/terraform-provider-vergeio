// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// RuleApi reads and writes firewall rules and rule aliases through govergeos.
type RuleApi struct {
	sdk *vergeos.Client
}

func NewRuleApi(c *vergeio.Client) *RuleApi {
	sdk, _ := vergeos.NewClient(c.SDKOptions()...)
	return &RuleApi{sdk: sdk}
}

type firewallNotice struct {
	Summary string
	Detail  string
}

func networkLabel(network *vergeos.Network, id int) string {
	if network != nil {
		name := strings.TrimSpace(network.Name)
		if name != "" {
			return fmt.Sprintf("%s (id %d)", name, id)
		}
	}
	return strconv.Itoa(id)
}

func stoppedFirewallNotice(network *vergeos.Network, id int) *firewallNotice {
	return &firewallNotice{
		Summary: "Firewall rules are not live yet",
		Detail: fmt.Sprintf(
			"Network %s is not running, so Terraform did not apply its firewall rules. A stopped network picks up staged rules when it starts.",
			networkLabel(network, id),
		),
	}
}

func stagedFirewallNotice(network *vergeos.Network, id int) *firewallNotice {
	return &firewallNotice{
		Summary: "Firewall rules were staged",
		Detail: fmt.Sprintf(
			"Network %s has staged firewall rule changes. apply is false, so Terraform did not apply them. need_fw_apply stays set until a later apply. VergeOS may act on a stale pending flag on its own.",
			networkLabel(network, id),
		),
	}
}

// syncRuleSet writes the authoritative non-system rule list and applies once.
func (a *RuleApi) syncRuleSet(ctx context.Context, vnetID int, desired []firewallRule, apply bool) ([]firewallRule, *firewallNotice, error) {
	if _, err := a.sdk.Networks.Get(ctx, vnetID); err != nil {
		if vergeos.IsNotFoundError(err) && len(desired) == 0 {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	current, err := a.sdk.VNetRules.ListByNetwork(ctx, vnetID)
	if err != nil {
		return nil, nil, err
	}
	changes, err := planRuleChanges(vnetID, current, desired, true)
	if err != nil {
		return nil, nil, err
	}
	if err := a.applyChanges(ctx, changes); err != nil {
		return nil, nil, err
	}
	notice, err := a.finishApply(ctx, vnetID, len(changes) > 0, apply)
	if err != nil {
		return nil, nil, err
	}
	rules, err := a.managedRules(ctx, vnetID)
	if err != nil {
		return nil, nil, err
	}
	return rules, notice, nil
}

// syncRule creates or updates one rule, then applies when apply is true.
func (a *RuleApi) syncRule(ctx context.Context, vnetID, existingKey int, desired firewallRule, apply bool) (firewallRule, *firewallNotice, error) {
	current, err := a.sdk.VNetRules.ListByNetwork(ctx, vnetID)
	if err != nil {
		return firewallRule{}, nil, err
	}
	changes, err := planSingleRule(vnetID, current, existingKey, desired)
	if err != nil {
		return firewallRule{}, nil, err
	}
	if err := a.applyChanges(ctx, changes); err != nil {
		return firewallRule{}, nil, err
	}
	notice, err := a.finishApply(ctx, vnetID, len(changes) > 0, apply)
	if err != nil {
		return firewallRule{}, nil, err
	}
	updated, err := a.sdk.VNetRules.ListByNetwork(ctx, vnetID)
	if err != nil {
		return firewallRule{}, nil, err
	}
	found, err := findRuleByName(updated, strings.TrimSpace(desired.Name))
	if err != nil {
		return firewallRule{}, nil, err
	}
	if found == nil {
		return firewallRule{}, nil, fmt.Errorf("firewall rule %q was not found after sync", desired.Name)
	}
	if found.SystemRule {
		return firewallRule{}, nil, &systemRuleError{Name: found.Name}
	}
	return ruleFromAPI(*found), notice, nil
}

// deleteRule removes one rule. A system rule is refused. A missing rule is
// already gone. The network is still refreshed when a previous delete left
// need_fw_apply set.
func (a *RuleApi) deleteRule(ctx context.Context, vnetID, key int, apply bool) (*firewallNotice, error) {
	if key <= 0 {
		return a.finishApply(ctx, vnetID, false, apply)
	}
	rule, err := a.sdk.VNetRules.Get(ctx, key)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return a.finishApply(ctx, vnetID, false, apply)
		}
		return nil, err
	}
	if rule.SystemRule {
		return nil, &systemRuleError{Name: rule.Name}
	}
	if err := a.sdk.VNetRules.Delete(ctx, key); err != nil {
		if vergeos.IsNotFoundError(err) {
			return a.finishApply(ctx, vnetID, false, apply)
		}
		return nil, err
	}
	return a.finishApply(ctx, vnetID, true, apply)
}

func (a *RuleApi) managedRules(ctx context.Context, vnetID int) ([]firewallRule, error) {
	current, err := a.sdk.VNetRules.ListByNetwork(ctx, vnetID)
	if err != nil {
		return nil, err
	}
	rules := make([]firewallRule, 0, len(current))
	for _, rule := range current {
		if rule.SystemRule {
			continue
		}
		rules = append(rules, ruleFromAPI(rule))
	}
	sortManagedRules(rules)
	return rules, nil
}

func (a *RuleApi) applyChanges(ctx context.Context, changes []ruleChange) error {
	for _, change := range changes {
		switch change.Kind {
		case ruleCreate:
			name := ""
			if change.Create != nil {
				name = change.Create.Name
			}
			if _, err := a.sdk.VNetRules.Create(ctx, change.Create); err != nil {
				return fmt.Errorf("create firewall rule %q: %w", name, err)
			}
		case ruleUpdate:
			if _, err := a.sdk.VNetRules.Update(ctx, change.Key, change.Update); err != nil {
				return fmt.Errorf("update firewall rule %d: %w", change.Key, err)
			}
		case ruleDelete:
			if err := a.sdk.VNetRules.Delete(ctx, change.Key); err != nil {
				if vergeos.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("delete firewall rule %d: %w", change.Key, err)
			}
		default:
			return fmt.Errorf("unknown firewall rule change %d", change.Kind)
		}
	}
	return nil
}

func (a *RuleApi) finishApply(ctx context.Context, vnetID int, wrote, apply bool) (*firewallNotice, error) {
	network, err := a.sdk.Networks.Get(ctx, vnetID)
	if err != nil {
		return nil, err
	}
	switch decideFirewallApply(wrote, apply, networkIsRunning(network), network.NeedFWApply) {
	case firewallApplyNow:
		tflog.Debug(ctx, fmt.Sprintf("Applying firewall rules on network %d", vnetID))
		if err := a.sdk.Networks.ApplyRules(ctx, vnetID); err != nil {
			return nil, err
		}
		return nil, nil
	case firewallApplySkippedStopped:
		tflog.Warn(ctx, fmt.Sprintf("Network %d is not running; skipping firewall apply", vnetID))
		return stoppedFirewallNotice(network, vnetID), nil
	case firewallApplySkippedDisabled:
		tflog.Warn(ctx, fmt.Sprintf("Network %d firewall apply is disabled; leaving rules staged", vnetID))
		return stagedFirewallNotice(network, vnetID), nil
	default:
		return nil, nil
	}
}

func (a *RuleApi) createAlias(ctx context.Context, data *networkRuleAliasModel) error {
	req := &vergeos.VNetRuleAliasCreateRequest{
		Name:  data.Name.ValueString(),
		Value: data.Value.ValueString(),
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	req.PublishingScope = vergeio.KnownString(data.PublishingScope)
	alias, err := a.sdk.VNetRuleAliases.Create(ctx, req)
	if err != nil {
		return err
	}
	data.ID = typesStringID(alias.Key.Int())
	return a.readAlias(ctx, data)
}

func (a *RuleApi) readAlias(ctx context.Context, data *networkRuleAliasModel) error {
	id, err := parsePositiveID(data.ID.ValueString())
	if err != nil {
		return err
	}
	alias, err := a.sdk.VNetRuleAliases.Get(ctx, id)
	if err != nil {
		return err
	}
	data.ID = typesStringID(alias.Key.Int())
	data.Name = types.StringValue(alias.Name)
	data.Description = types.StringValue(alias.Description)
	data.Value = types.StringValue(alias.Value)
	data.PublishingScope = types.StringValue(alias.PublishingScope)
	return nil
}

func (a *RuleApi) updateAlias(ctx context.Context, plan, state *networkRuleAliasModel) error {
	id, err := parsePositiveID(state.ID.ValueString())
	if err != nil {
		return err
	}
	req := &vergeos.VNetRuleAliasUpdateRequest{
		Name:            vergeio.ChangedString(plan.Name, state.Name),
		Description:     vergeio.ChangedString(plan.Description, state.Description),
		Value:           vergeio.ChangedString(plan.Value, state.Value),
		PublishingScope: vergeio.ChangedString(plan.PublishingScope, state.PublishingScope),
	}
	if req.Name == nil && req.Description == nil && req.Value == nil && req.PublishingScope == nil {
		plan.ID = state.ID
		return a.readAlias(ctx, plan)
	}
	if _, err := a.sdk.VNetRuleAliases.Update(ctx, id, req); err != nil {
		return err
	}
	plan.ID = state.ID
	return a.readAlias(ctx, plan)
}

func (a *RuleApi) deleteAlias(ctx context.Context, data *networkRuleAliasModel) error {
	id, err := parsePositiveID(data.ID.ValueString())
	if err != nil {
		return err
	}
	if err := a.sdk.VNetRuleAliases.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	return nil
}

func typesStringID(id int) types.String {
	return types.StringValue(strconv.Itoa(id))
}
