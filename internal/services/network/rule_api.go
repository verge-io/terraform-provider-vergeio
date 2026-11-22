// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// RuleApi reads and writes firewall rules and rule aliases through govergeos.
type RuleApi struct {
	sdk *vergeos.Client
}

func NewRuleApi(c *vergeio.Client) (*RuleApi, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &RuleApi{sdk: sdk}, nil
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
	gate := networkRuleOrderGate(vnetID)
	worker := gate.joinRemoving()
	wrote, skip, err := a.writeRuleSet(ctx, gate, vnetID, desired)
	gate.leave(worker)
	if err != nil || skip {
		return nil, nil, err
	}
	notice, err := a.finishApply(ctx, vnetID, wrote, apply)
	if err != nil {
		return nil, nil, err
	}
	rules, err := a.managedRules(ctx, vnetID)
	if err != nil {
		return nil, nil, err
	}
	return rules, notice, nil
}

func (a *RuleApi) writeRuleSet(ctx context.Context, gate *orderGate, vnetID int, desired []firewallRule) (bool, bool, error) {
	gate.place.Lock()
	defer gate.place.Unlock()
	if _, err := a.sdk.Networks.Get(ctx, vnetID); err != nil {
		if vergeos.IsNotFoundError(err) && len(desired) == 0 {
			return false, true, nil
		}
		return false, false, err
	}
	current, err := a.sdk.VNetRules.ListByNetwork(ctx, vnetID)
	if err != nil {
		return false, false, err
	}
	changes, err := planRuleChanges(vnetID, current, desired, true)
	if err != nil {
		return false, false, err
	}
	if err := a.applyChanges(ctx, changes); err != nil {
		return false, false, err
	}
	return len(changes) > 0, false, nil
}

// syncRule creates or updates one rule, then applies when apply is true.
func (a *RuleApi) syncRule(ctx context.Context, vnetID, existingKey int, desired firewallRule, apply bool) (firewallRule, *firewallNotice, error) {
	ordered := desired.Optional != nil && desired.Optional.Order
	gate := networkRuleOrderGate(vnetID)
	want := 0
	if ordered {
		want = desired.OrderID
	}
	worker := gate.join(ordered, want)
	defer gate.leave(worker)
	if existingKey > 0 {
		gate.markCreated(worker)
	}
	if !ordered {
		rule, wrote, err := a.writeUnorderedRule(ctx, gate, worker, vnetID, existingKey, desired)
		if err != nil {
			return firewallRule{}, nil, err
		}
		notice, err := a.finishApply(ctx, vnetID, wrote, apply)
		if err != nil {
			return firewallRule{}, nil, err
		}
		return rule, notice, nil
	}
	return a.syncOrderedRule(ctx, worker, vnetID, existingKey, desired, apply)
}

func (a *RuleApi) writeUnorderedRule(ctx context.Context, gate *orderGate, worker *orderWorker, vnetID, existingKey int, desired firewallRule) (firewallRule, bool, error) {
	gate.place.Lock()
	defer gate.place.Unlock()
	rule, wrote, err := a.writeSingleRule(ctx, vnetID, existingKey, desired)
	if err != nil {
		return firewallRule{}, false, err
	}
	gate.markCreated(worker)
	return rule, wrote, nil
}

func (a *RuleApi) writeSingleRule(ctx context.Context, vnetID, existingKey int, desired firewallRule) (firewallRule, bool, error) {
	current, err := a.sdk.VNetRules.ListByNetwork(ctx, vnetID)
	if err != nil {
		return firewallRule{}, false, err
	}
	changes, err := planSingleRule(vnetID, current, existingKey, desired)
	if err != nil {
		return firewallRule{}, false, err
	}
	if err := a.applyChanges(ctx, changes); err != nil {
		return firewallRule{}, false, err
	}
	updated, err := a.sdk.VNetRules.ListByNetwork(ctx, vnetID)
	if err != nil {
		return firewallRule{}, false, err
	}
	found, err := findRuleByName(updated, strings.TrimSpace(desired.Name))
	if err != nil {
		return firewallRule{}, false, err
	}
	if found == nil {
		return firewallRule{}, false, fmt.Errorf("firewall rule %q was not found after sync", desired.Name)
	}
	if found.SystemRule {
		return firewallRule{}, false, &systemRuleError{Name: found.Name}
	}
	return ruleFromAPI(*found), len(changes) > 0, nil
}

func (a *RuleApi) syncOrderedRule(ctx context.Context, worker *orderWorker, vnetID, existingKey int, desired firewallRule, apply bool) (firewallRule, *firewallNotice, error) {
	gate := networkRuleOrderGate(vnetID)
	deadline := time.Now().Add(ruleOrderWait)
	var wrote bool
	var saved firewallRule
	for {
		if err := ctx.Err(); err != nil {
			return firewallRule{}, nil, err
		}
		if time.Now().After(deadline) {
			return firewallRule{}, nil, fmt.Errorf(
				"firewall rule %q did not stay at orderid %d; another rule on this network is using that position; leave orderid unset, or use vergeio_network_rules to own the order",
				strings.TrimSpace(desired.Name), desired.OrderID,
			)
		}
		gate.place.Lock()
		rule, settled, stepWrote, err := a.placeRuleOrder(ctx, gate, worker, vnetID, existingKey, desired)
		if err != nil {
			gate.place.Unlock()
			return firewallRule{}, nil, err
		}
		if settled {
			got, getErr := a.sdk.VNetRules.Get(ctx, rule.Key)
			if getErr != nil {
				gate.place.Unlock()
				return firewallRule{}, nil, getErr
			}
			if got.OrderID != desired.OrderID || gate.hasRemoving(worker) {
				gate.setRest(worker, false)
				settled = false
			} else {
				saved = ruleFromAPI(*got)
			}
		}
		gate.place.Unlock()
		if stepWrote {
			wrote = true
		}
		if !settled {
			if err := waitRuleOrder(ctx, ruleOrderPoll); err != nil {
				return firewallRule{}, nil, err
			}
			continue
		}
		break
	}
	notice, err := a.finishApply(ctx, vnetID, wrote, apply)
	if err != nil {
		return firewallRule{}, nil, err
	}
	return saved, notice, nil
}

func (a *RuleApi) placeRuleOrder(ctx context.Context, gate *orderGate, worker *orderWorker, vnetID, existingKey int, desired firewallRule) (firewallRule, bool, bool, error) {
	current, err := a.sdk.VNetRules.ListByNetwork(ctx, vnetID)
	if err != nil {
		return firewallRule{}, false, false, err
	}
	found, err := findManagedRule(current, existingKey, desired.Name)
	if err != nil {
		return firewallRule{}, false, false, err
	}
	if gate.hasRemoving(worker) {
		gate.setRest(worker, false)
		return firewallRule{}, false, false, nil
	}
	count := nonSystemCount(current)
	span := count
	if found == nil {
		span++
	} else {
		gate.markCreated(worker)
	}
	extra := gate.room(worker, count)
	limit := span + extra
	if err := rejectRuleOrder(desired.Name, desired.OrderID, limit); err != nil {
		return firewallRule{}, false, false, err
	}
	if desired.OrderID > span {
		gate.setRest(worker, false)
		return firewallRule{}, false, false, nil
	}

	var rule firewallRule
	wrote := false
	if !worker.written {
		rule, wrote, err = a.writeSingleRule(ctx, vnetID, existingKey, desired)
		if err != nil {
			return firewallRule{}, false, false, err
		}
		worker.written = true
		gate.markCreated(worker)
	} else if found == nil {
		return firewallRule{}, false, false, fmt.Errorf("firewall rule %q was not found after sync", desired.Name)
	} else {
		rule = ruleFromAPI(*found)
	}
	if rule.OrderID != desired.OrderID {
		gate.setRest(worker, false)
		order := desired.OrderID
		tflog.Debug(ctx, fmt.Sprintf("Moving firewall rule %d on network %d to orderid %d", rule.Key, vnetID, order))
		if _, err := a.sdk.VNetRules.Update(ctx, rule.Key, &vergeos.VNetRuleUpdateRequest{OrderID: &order}); err != nil {
			return firewallRule{}, false, wrote, err
		}
		moved, err := a.sdk.VNetRules.Get(ctx, rule.Key)
		if err != nil {
			return firewallRule{}, false, wrote, err
		}
		rule = ruleFromAPI(*moved)
		wrote = true
		if rule.OrderID != desired.OrderID {
			return rule, false, wrote, nil
		}
	}
	return rule, gate.setRest(worker, true), wrote, nil
}

func findManagedRule(rules []vergeos.VNetRule, key int, name string) (*vergeos.VNetRule, error) {
	if key > 0 {
		for i := range rules {
			if rules[i].Key.Int() != key {
				continue
			}
			if rules[i].SystemRule {
				return nil, &systemRuleError{Name: rules[i].Name}
			}
			return &rules[i], nil
		}
	}
	return findRuleByName(rules, strings.TrimSpace(name))
}

// deleteRule removes one rule. A system rule is refused. A missing rule is
// already gone. The network is still refreshed when a previous delete left
// need_fw_apply set.
func (a *RuleApi) deleteRule(ctx context.Context, vnetID, key int, apply bool) (*firewallNotice, error) {
	if key <= 0 {
		return a.finishApply(ctx, vnetID, false, apply)
	}
	gate := networkRuleOrderGate(vnetID)
	worker := gate.joinRemoving()
	wrote, err := a.deleteOrderedRule(ctx, gate, key)
	gate.leave(worker)
	if err != nil {
		return nil, err
	}
	return a.finishApply(ctx, vnetID, wrote, apply)
}

func (a *RuleApi) deleteOrderedRule(ctx context.Context, gate *orderGate, key int) (bool, error) {
	gate.place.Lock()
	defer gate.place.Unlock()
	rule, err := a.sdk.VNetRules.Get(ctx, key)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	if rule.SystemRule {
		return false, &systemRuleError{Name: rule.Name}
	}
	raw := a.sdk.VNetRules.Delete(ctx, key)
	if err := confirmDeleted(raw, namedRow("firewall rule", key, rule.Name), func() error {
		_, getErr := a.sdk.VNetRules.Get(ctx, key)
		return getErr
	}); err != nil {
		return false, err
	}
	return raw == nil, nil
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
			raw := a.sdk.VNetRules.Delete(ctx, change.Key)
			if err := confirmDeleted(raw, namedRow("firewall rule", change.Key, ""), func() error {
				_, getErr := a.sdk.VNetRules.Get(ctx, change.Key)
				return getErr
			}); err != nil {
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
	// VergeOS reuses vnet_rule_aliases keys after delete. A Get that returns a
	// different alias_id (readonly SHA1) must not be adopted into state (#227,
	// #231). Name is editable, so it cannot distinguish rename drift from reuse.
	if err := ensureAliasOwned(data, alias); err != nil {
		return err
	}
	data.ID = typesStringID(alias.Key.Int())
	data.AliasID = types.StringValue(alias.ID)
	data.Name = types.StringValue(alias.Name)
	data.Description = types.StringValue(alias.Description)
	data.Value = types.StringValue(alias.Value)
	data.PublishingScope = types.StringValue(alias.PublishingScope)
	return nil
}

// ensureAliasOwned returns NotFound when the stored key now points at another
// alias (alias_id / SHA1 mismatch). Skip the check when alias_id is unset
// (import). Same-id name drift still refreshes in place (#231).
func ensureAliasOwned(data *networkRuleAliasModel, alias *vergeos.VNetRuleAlias) error {
	if data.AliasID.IsNull() || data.AliasID.IsUnknown() || data.AliasID.ValueString() == "" {
		return nil
	}
	if alias.ID != data.AliasID.ValueString() {
		return &vergeos.NotFoundError{Resource: "VNetRuleAlias", ID: alias.Key.Int()}
	}
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
