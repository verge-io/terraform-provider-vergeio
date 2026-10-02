// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestFirewallResourceNames(t *testing.T) {
	ctx := t.Context()
	cases := []struct {
		resource resource.Resource
		name     string
	}{
		{NewNetworkRuleResource(), "vergeio_network_rule"},
		{NewNetworkRulesResource(), "vergeio_network_rules"},
		{NewNetworkRuleAliasResource(), "vergeio_network_rule_alias"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &resource.MetadataResponse{}
			tc.resource.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
			if resp.TypeName != tc.name {
				t.Fatalf("type = %s", resp.TypeName)
			}
			schemaResp := &resource.SchemaResponse{}
			tc.resource.Schema(ctx, resource.SchemaRequest{}, schemaResp)
			if len(schemaResp.Schema.Attributes) == 0 {
				t.Fatal("schema has no attributes")
			}
		})
	}
}

func TestRuleListAttributeNamesMatchStateType(t *testing.T) {
	var resp resource.SchemaResponse
	NewNetworkRulesResource().Schema(t.Context(), resource.SchemaRequest{}, &resp)
	nested, ok := resp.Schema.Attributes["rule"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("rule attribute type = %T", resp.Schema.Attributes["rule"])
	}
	got := ruleAttrTypes()
	if len(nested.NestedObject.Attributes) != len(got) {
		t.Fatalf("schema has %d attributes, state type has %d", len(nested.NestedObject.Attributes), len(got))
	}
	for name := range nested.NestedObject.Attributes {
		if _, ok := got[name]; !ok {
			t.Fatalf("schema attribute %s is missing from ruleAttrTypes", name)
		}
	}
}

func TestSyncRuleSetAppliesOnce(t *testing.T) {
	fix := newRuleFixture(t, true)
	api := newRuleTestAPI(t, fix.serve)
	desired := []firewallRule{
		ruleWith(baseRule("allow-ssh"), func(rule *firewallRule) { rule.DestinationPorts = "22" }),
		ruleWith(baseRule("allow-https"), func(rule *firewallRule) { rule.DestinationPorts = "443" }),
	}
	rules, notice, err := api.syncRuleSet(t.Context(), 3, desired, true)
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("notice = %#v", notice)
	}
	if len(rules) != 2 || rules[0].Name != "allow-ssh" || rules[1].Name != "allow-https" {
		t.Fatalf("rules = %#v", rules)
	}
	if fix.applyCount() != 1 {
		t.Fatalf("apply count = %d, want 1; calls = %v", fix.applyCount(), fix.calls)
	}
	if !callBefore(fix.calls, "POST /api/v4/vnet_rules", "POST /api/v4/vnet_actions") {
		t.Fatalf("apply ran before the rule writes: %v", fix.calls)
	}
	if strings.Count(strings.Join(fix.calls, "\n"), "POST /api/v4/vnet_rules") != 2 {
		t.Fatalf("creates = %v", fix.calls)
	}
	if fix.lastAction() != "refresh" || fix.lastVNet() != 3 {
		t.Fatalf("action = %s vnet = %d", fix.lastAction(), fix.lastVNet())
	}
}

func TestSyncRuleSetSecondPassDoesNotApply(t *testing.T) {
	fix := newRuleFixture(t, true)
	api := newRuleTestAPI(t, fix.serve)
	desired := []firewallRule{ruleWith(baseRule("allow-ssh"), func(rule *firewallRule) { rule.DestinationPorts = "22" })}
	if _, _, err := api.syncRuleSet(t.Context(), 3, desired, true); err != nil {
		t.Fatal(err)
	}
	fix.calls = nil
	if _, notice, err := api.syncRuleSet(t.Context(), 3, desired, true); err != nil {
		t.Fatal(err)
	} else if notice != nil {
		t.Fatalf("notice = %#v", notice)
	}
	for _, call := range fix.calls {
		if strings.Contains(call, "vnet_actions") || strings.HasPrefix(call, "PUT ") || strings.HasPrefix(call, "POST /api/v4/vnet_rules") {
			t.Fatalf("second pass wrote rules: %v", fix.calls)
		}
	}
}

func TestSyncRuleSetAppliesPendingFlag(t *testing.T) {
	fix := newRuleFixture(t, true)
	fix.needFW = true
	fix.seed(apiRule(12, "allow-ssh", 1))
	api := newRuleTestAPI(t, fix.serve)
	if _, _, err := api.syncRuleSet(t.Context(), 3, []firewallRule{baseRule("allow-ssh")}, true); err != nil {
		t.Fatal(err)
	}
	if fix.applyCount() != 1 {
		t.Fatalf("apply count = %d, calls = %v", fix.applyCount(), fix.calls)
	}
}

func TestSyncRuleSetSkipsStoppedNetwork(t *testing.T) {
	fix := newRuleFixture(t, false)
	api := newRuleTestAPI(t, fix.serve)
	_, notice, err := api.syncRuleSet(t.Context(), 3, []firewallRule{baseRule("allow-ssh")}, true)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || !strings.Contains(notice.Detail, "not running") {
		t.Fatalf("notice = %#v", notice)
	}
	if fix.applyCount() != 0 {
		t.Fatalf("stopped network was refreshed: %v", fix.calls)
	}
	if !strings.Contains(strings.Join(fix.calls, "\n"), "POST /api/v4/vnet_rules") {
		t.Fatal("stopped network skipped the rule write")
	}
}

func TestSyncRuleSetPreservesSystemRule(t *testing.T) {
	fix := newRuleFixture(t, true)
	fix.seed(systemRule(1, "builtin"), apiRule(12, "stale", 2))
	api := newRuleTestAPI(t, fix.serve)
	rules, _, err := api.syncRuleSet(t.Context(), 3, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("managed rules = %#v", rules)
	}
	if _, ok := fix.rule(1); !ok {
		t.Fatal("system rule was deleted")
	}
	if _, ok := fix.rule(12); ok {
		t.Fatal("stale rule was kept")
	}
	if fix.applyCount() != 1 {
		t.Fatalf("apply count = %d", fix.applyCount())
	}
}

func TestSyncRuleSetRefusesSystemNameBeforeWriting(t *testing.T) {
	fix := newRuleFixture(t, true)
	fix.seed(systemRule(1, "builtin"), apiRule(12, "keep", 1))
	api := newRuleTestAPI(t, fix.serve)
	_, _, err := api.syncRuleSet(t.Context(), 3, []firewallRule{baseRule("builtin")}, true)
	if err == nil || !strings.Contains(err.Error(), "system rule") {
		t.Fatalf("err = %v", err)
	}
	if _, ok := fix.rule(12); !ok {
		t.Fatal("refused sync still deleted a rule")
	}
	if fix.applyCount() != 0 {
		t.Fatal("refused sync applied rules")
	}
}

func TestSyncRuleApplyFalseDoesNotRefresh(t *testing.T) {
	fix := newRuleFixture(t, true)
	api := newRuleTestAPI(t, fix.serve)
	rule, notice, err := api.syncRule(t.Context(), 3, 0, singleRule("allow-ssh"), false)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || !strings.Contains(notice.Detail, "apply is false") {
		t.Fatalf("notice = %#v", notice)
	}
	if rule.Key == 0 {
		t.Fatal("created rule has no key")
	}
	if fix.applyCount() != 0 {
		t.Fatalf("apply=false refreshed the network: %v", fix.calls)
	}
}

func TestSyncRuleRetriesApplyWhenFlagStaysSet(t *testing.T) {
	fix := newRuleFixture(t, true)
	fix.seed(apiRule(12, "allow-ssh", 4))
	fix.needFW = true
	api := newRuleTestAPI(t, fix.serve)
	if _, _, err := api.syncRule(t.Context(), 3, 12, singleRule("allow-ssh"), true); err != nil {
		t.Fatal(err)
	}
	if fix.applyCount() != 1 {
		t.Fatalf("pending rule was not applied: %v", fix.calls)
	}
	for _, call := range fix.calls {
		if strings.HasPrefix(call, "PUT ") || strings.HasPrefix(call, "POST /api/v4/vnet_rules") {
			t.Fatalf("retry rewrote the rule: %v", fix.calls)
		}
	}
}

func TestDeleteRuleRefusesSystemRule(t *testing.T) {
	fix := newRuleFixture(t, true)
	fix.seed(systemRule(9, "builtin"))
	api := newRuleTestAPI(t, fix.serve)
	_, err := api.deleteRule(t.Context(), 3, 9, true)
	if err == nil || !strings.Contains(err.Error(), "system rule") {
		t.Fatalf("err = %v", err)
	}
	if _, ok := fix.rule(9); !ok {
		t.Fatal("system rule was deleted")
	}
	if fix.applyCount() != 0 {
		t.Fatal("system rule delete applied rules")
	}
}

func TestDeleteMissingNetworkRulesIsDone(t *testing.T) {
	fix := newRuleFixture(t, true)
	fix.missingNetwork = true
	api := newRuleTestAPI(t, fix.serve)
	if _, _, err := api.syncRuleSet(t.Context(), 3, nil, true); err != nil {
		t.Fatal(err)
	}
	if fix.applyCount() != 0 {
		t.Fatal("missing network was refreshed")
	}
}

func TestAliasCreateUpdateDelete(t *testing.T) {
	fix := newRuleFixture(t, true)
	api := newRuleTestAPI(t, fix.serve)
	data := &networkRuleAliasModel{
		Name:            types.StringValue("mgmt-nets"),
		Value:           types.StringValue("192.0.2.0/24"),
		PublishingScope: types.StringValue("private"),
	}
	if err := api.createAlias(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.ID.ValueString() == "" {
		t.Fatal("alias id was not stored")
	}
	if data.AliasID.ValueString() == "" {
		t.Fatal("alias_id was not stored")
	}
	plan := *data
	plan.Value = types.StringValue("192.0.2.0/24,198.51.100.0/24")
	if err := api.updateAlias(t.Context(), &plan, data); err != nil {
		t.Fatal(err)
	}
	if plan.Value.ValueString() != "192.0.2.0/24,198.51.100.0/24" {
		t.Fatalf("value = %s", plan.Value.ValueString())
	}
	if err := api.deleteAlias(t.Context(), &plan); err != nil {
		t.Fatal(err)
	}
	if err := api.readAlias(t.Context(), &plan); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("read after delete = %v", err)
	}
}

func TestReadAliasRejectsForeignAliasID(t *testing.T) {
	fix := newRuleFixture(t, true)
	api := newRuleTestAPI(t, fix.serve)
	data := &networkRuleAliasModel{
		Name:            types.StringValue("mgmt-nets"),
		Value:           types.StringValue("192.0.2.0/24"),
		PublishingScope: types.StringValue("private"),
	}
	if err := api.createAlias(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	ownedID := data.AliasID.ValueString()
	if ownedID == "" {
		t.Fatal("expected alias_id after create")
	}
	id, err := parsePositiveID(data.ID.ValueString())
	if err != nil {
		t.Fatal(err)
	}
	fix.aliases[id]["id"] = "ffffffffffffffffffffffffffffffffffffffff"
	fix.aliases[id]["name"] = "someone-elses-alias"

	err = api.readAlias(t.Context(), data)
	if err == nil || !vergeos.IsNotFoundError(err) {
		t.Fatalf("read foreign key = %v, want NotFound", err)
	}
	if data.AliasID.ValueString() != ownedID {
		t.Fatalf("state alias_id was overwritten to %q", data.AliasID.ValueString())
	}
	if data.Name.ValueString() != "mgmt-nets" {
		t.Fatalf("state name was overwritten to %q", data.Name.ValueString())
	}
}

func TestReadAliasAllowsMatchingAliasID(t *testing.T) {
	fix := newRuleFixture(t, true)
	api := newRuleTestAPI(t, fix.serve)
	data := &networkRuleAliasModel{
		Name:            types.StringValue("mgmt-nets"),
		Value:           types.StringValue("192.0.2.0/24"),
		PublishingScope: types.StringValue("private"),
	}
	if err := api.createAlias(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	ownedID := data.AliasID.ValueString()
	if err := api.readAlias(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.AliasID.ValueString() != ownedID {
		t.Fatalf("alias_id = %q, want %q", data.AliasID.ValueString(), ownedID)
	}
	if data.Name.ValueString() != "mgmt-nets" {
		t.Fatalf("name = %q", data.Name.ValueString())
	}
}

func TestReadAliasSkipsOwnershipOnImport(t *testing.T) {
	fix := newRuleFixture(t, true)
	api := newRuleTestAPI(t, fix.serve)
	created := &networkRuleAliasModel{
		Name:            types.StringValue("mgmt-nets"),
		Value:           types.StringValue("192.0.2.0/24"),
		PublishingScope: types.StringValue("private"),
	}
	if err := api.createAlias(t.Context(), created); err != nil {
		t.Fatal(err)
	}
	data := &networkRuleAliasModel{ID: created.ID}
	if err := api.readAlias(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Name.ValueString() != "mgmt-nets" {
		t.Fatalf("imported name = %q", data.Name.ValueString())
	}
	if data.AliasID.ValueString() == "" {
		t.Fatal("imported alias_id empty")
	}
}

// TestReadAliasAllowsNameDriftSameAliasID covers in-place renames: same
// alias_id with a different name must refresh, not return NotFound (#231).
func TestReadAliasAllowsNameDriftSameAliasID(t *testing.T) {
	fix := newRuleFixture(t, true)
	api := newRuleTestAPI(t, fix.serve)
	data := &networkRuleAliasModel{
		Name:            types.StringValue("mgmt-nets"),
		Value:           types.StringValue("192.0.2.0/24"),
		PublishingScope: types.StringValue("private"),
	}
	if err := api.createAlias(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	ownedID := data.AliasID.ValueString()
	id, err := parsePositiveID(data.ID.ValueString())
	if err != nil {
		t.Fatal(err)
	}
	fix.aliases[id]["name"] = "mgmt-nets-renamed"

	if err := api.readAlias(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Name.ValueString() != "mgmt-nets-renamed" {
		t.Fatalf("name = %q, want renamed", data.Name.ValueString())
	}
	if data.AliasID.ValueString() != ownedID {
		t.Fatalf("alias_id = %q, want %q", data.AliasID.ValueString(), ownedID)
	}
}

func TestManagedRulesOmitSystemAndSort(t *testing.T) {
	fix := newRuleFixture(t, true)
	second := apiRule(8, "b", 30)
	first := apiRule(7, "a", 10)
	fix.seed(second, systemRule(1, "builtin"), first)
	api := newRuleTestAPI(t, fix.serve)
	rules, err := api.managedRules(t.Context(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Name != "a" || rules[1].Name != "b" {
		t.Fatalf("rules = %#v", rules)
	}
}

func newRuleTestAPI(t *testing.T, handler http.HandlerFunc) *RuleApi {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		if r.URL.Path == "/version.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"version":"26.1.8"}`)); err != nil {
				t.Errorf("write version: %v", err)
			}
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	sdk, err := vergeos.NewClient(
		vergeos.WithBaseURL(server.URL),
		vergeos.WithCredentials("user", "pass"),
		vergeos.WithInsecureTLS(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &RuleApi{sdk: sdk}
}

type ruleFixture struct {
	mu             sync.Mutex
	t              *testing.T
	running        bool
	needFW         bool
	missingNetwork bool
	next           int
	rules          map[int]map[string]any
	aliases        map[int]map[string]any
	calls          []string
	applies        int
	action         string
	actionVNet     int
}

func newRuleFixture(t *testing.T, running bool) *ruleFixture {
	t.Helper()
	return &ruleFixture{
		t:       t,
		running: running,
		next:    20,
		rules:   map[int]map[string]any{},
		aliases: map[int]map[string]any{},
	}
}

func (f *ruleFixture) seed(rules ...vergeos.VNetRule) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rule := range rules {
		body, err := json.Marshal(rule)
		if err != nil {
			f.t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			f.t.Fatal(err)
		}
		key := rule.Key.Int()
		decoded["$key"] = key
		f.rules[key] = decoded
	}
}

func (f *ruleFixture) rule(key int) (map[string]any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rule, ok := f.rules[key]
	return rule, ok
}

func (f *ruleFixture) applyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.applies
}

func (f *ruleFixture) lastAction() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.action
}

func (f *ruleFixture) lastVNet() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.actionVNet
}

func (f *ruleFixture) serve(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_rules":
		writeJSON(f.t, w, http.StatusOK, f.ruleList())
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_rules":
		f.createRule(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_rules/"):
		f.getRule(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_rules/"):
		f.updateRule(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_rules/"):
		f.deleteRule(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/vnets/"):
		f.getNetwork(w)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
		f.apply(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_rule_aliases":
		f.createAlias(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_rule_aliases/"):
		f.getAlias(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_rule_aliases/"):
		f.updateAlias(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_rule_aliases/"):
		f.deleteAlias(w, r)
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}
}

func (f *ruleFixture) ruleList() []map[string]any {
	out := make([]map[string]any, 0, len(f.rules))
	for _, rule := range f.rules {
		out = append(out, rule)
	}
	return out
}

func (f *ruleFixture) createRule(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	body := readMap(f.t, r)
	f.next++
	body["$key"] = f.next
	f.rules[f.next] = body
	f.needFW = true
	writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": f.next})
}

func (f *ruleFixture) getRule(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	key := pathKey(r.URL.Path)
	rule, ok := f.rules[key]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	writeJSON(f.t, w, http.StatusOK, rule)
}

func (f *ruleFixture) updateRule(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	key := pathKey(r.URL.Path)
	rule, ok := f.rules[key]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	for k, v := range readMap(f.t, r) {
		rule[k] = v
	}
	f.needFW = true
	writeJSON(f.t, w, http.StatusOK, map[string]any{})
}

func (f *ruleFixture) deleteRule(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	key := pathKey(r.URL.Path)
	if _, ok := f.rules[key]; !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	delete(f.rules, key)
	f.needFW = true
	w.WriteHeader(http.StatusOK)
}

func (f *ruleFixture) getNetwork(w http.ResponseWriter) {
	if f.missingNetwork {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	status := "stopped"
	if f.running {
		status = "running"
	}
	writeJSON(f.t, w, http.StatusOK, map[string]any{
		"$key":          3,
		"name":          "lan",
		"running":       f.running,
		"status":        status,
		"need_fw_apply": f.needFW,
	})
}

func (f *ruleFixture) apply(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	body := readMap(f.t, r)
	f.applies++
	f.action, _ = body["action"].(string)
	switch v := body["vnet"].(type) {
	case float64:
		f.actionVNet = int(v)
	}
	f.needFW = false
	writeJSON(f.t, w, http.StatusOK, map[string]any{})
}

func (f *ruleFixture) createAlias(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	body := readMap(f.t, r)
	f.next++
	body["$key"] = f.next
	// Readonly SHA1 hex id assigned by VergeOS; stable across name updates.
	sum := sha1.Sum([]byte(fmt.Sprintf("fixture-alias-%d", f.next)))
	body["id"] = hex.EncodeToString(sum[:])
	f.aliases[f.next] = body
	writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": f.next})
}

func (f *ruleFixture) getAlias(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	key := pathKey(r.URL.Path)
	alias, ok := f.aliases[key]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	writeJSON(f.t, w, http.StatusOK, alias)
}

func (f *ruleFixture) updateAlias(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	key := pathKey(r.URL.Path)
	alias, ok := f.aliases[key]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	for k, v := range readMap(f.t, r) {
		alias[k] = v
	}
	writeJSON(f.t, w, http.StatusOK, map[string]any{})
}

func (f *ruleFixture) deleteAlias(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	key := pathKey(r.URL.Path)
	if _, ok := f.aliases[key]; !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	delete(f.aliases, key)
	w.WriteHeader(http.StatusOK)
}

func pathKey(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id, _ := strconv.Atoi(parts[len(parts)-1])
	return id
}

func readMap(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read body: %v", err)
		return map[string]any{}
	}
	var decoded map[string]any
	if len(body) == 0 {
		return map[string]any{}
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Errorf("decode %s: %v", body, err)
		return map[string]any{}
	}
	return decoded
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode: %v", err)
	}
}

func callBefore(calls []string, earlier, later string) bool {
	early, late := -1, -1
	for i, call := range calls {
		if early < 0 && call == earlier {
			early = i
		}
		if call == later {
			late = i
		}
	}
	return early >= 0 && late >= 0 && early < late
}

func singleRule(name string) firewallRule {
	rule := baseRule(name)
	rule.Optional = &optionalRuleFields{}
	return rule
}
