// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

type recordedBody struct {
	method string
	path   string
	body   map[string]any
}

type fakeVerge struct {
	t *testing.T

	mu              sync.Mutex
	next            int
	uiIP            string
	holdPower       bool
	transitionPower bool
	seenTransition  map[int]bool
	tenantGets      int
	failTenantGet   int
	// lastNodeDeleteFails is how many remaining DELETE /tenant_nodes/{id}
	// calls should return 405 "Only the last node can be deleted" (#222).
	lastNodeDeleteFails int
	// nodeDeleteFailStatus/Message force a permanent delete failure when set.
	nodeDeleteFailStatus  int
	nodeDeleteFailMessage string
	// nodePowerOffStuck leaves the machine running after poweroff so tests
	// can assert the Kill fallback (#220). Kill still stops the machine
	// unless nodeKillStuck is also set.
	nodePowerOffStuck bool
	nodeKillStuck     bool
	// nodePowerOffNotRunning makes poweroff return 422 as when VergeOS
	// rejects poweroff on a non-running node.
	nodePowerOffNotRunning bool
	// isolateFail makes isolateon and isolateoff return 500 and leave the
	// tenant row unchanged.
	isolateFail bool
	// isolateIgnore accepts isolateon and isolateoff without changing the
	// row, so readback still disagrees with the plan.
	isolateIgnore bool
	tenants       map[int]map[string]any
	status        map[int]map[string]any
	addresses     map[int]map[string]any
	cidrs         map[int]map[string]any
	// cidrDeleteStatus, when non-zero, makes DELETE /vnet_cidrs/{id} fail
	// with that status and cidrDeleteMessage, leaving the row in place.
	cidrDeleteStatus  int
	cidrDeleteMessage string
	layer2            map[int]map[string]any
	// layer2DisableStatus, when non-zero, makes a PUT that sets enabled to
	// false fail and leaves the row unchanged.
	layer2DisableStatus  int
	layer2DisableMessage string
	// layer2DeleteStatus, when non-zero, makes DELETE of a disabled row fail
	// and leave it in place. A delete of an enabled row always fails.
	layer2DeleteStatus  int
	layer2DeleteMessage string
	nodes               map[int]map[string]any
	storage             map[int]map[string]any
	vnets               map[int]map[string]any
	machines            map[int]map[string]any
	calls               []string
	bodies              []recordedBody
	actions             []map[string]any
	vnetActions         []map[string]any
	nodeActions         []map[string]any
	// fwSettleReads is how many vnet GETs after a refresh still report
	// need_fw_apply. Zero clears the flag inside the refresh handler.
	fwSettleReads int
	fwSettleLeft  map[int]int
}

func newFake(t *testing.T) *fakeVerge {
	t.Helper()
	return &fakeVerge{
		t:              t,
		next:           1,
		seenTransition: map[int]bool{},
		tenants:        map[int]map[string]any{},
		status:         map[int]map[string]any{},
		addresses:      map[int]map[string]any{},
		cidrs:          map[int]map[string]any{},
		layer2:         map[int]map[string]any{},
		nodes:          map[int]map[string]any{},
		storage:        map[int]map[string]any{},
		vnets:          map[int]map[string]any{},
		machines:       map[int]map[string]any{},
	}
}

func (f *fakeVerge) api(t *testing.T) *API {
	t.Helper()
	f.t = t
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	sdk, err := vergeos.NewClient(
		vergeos.WithBaseURL(server.URL),
		vergeos.WithCredentials("user", "pass"),
		vergeos.WithInsecureTLS(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &API{name: "Tenant Api", sdk: sdk}
}

func (f *fakeVerge) seedTenant(id int, name, uiIP string, online bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id >= f.next {
		f.next = id + 1
	}
	tenant := map[string]any{
		"$key":        id,
		"name":        name,
		"description": "",
		"uuid":        fmt.Sprintf("uuid-%d", id),
		"vnet":        40 + id,
		"isolate":     false,
		"is_snapshot": false,
		"creator":     "admin",
		"created":     int64(1700000000),
	}
	if uiIP != "" {
		addressID := 100 + id
		tenant["ui_address"] = addressID
		f.addresses[addressID] = map[string]any{
			"$key": addressID,
			"vnet": 3,
			"ip":   uiIP,
			"type": "static",
		}
	}
	f.tenants[id] = tenant
	f.status[id] = statusObject(id, online)
	vnetID := 40 + id
	f.vnets[vnetID] = map[string]any{
		"$key":    vnetID,
		"name":    "tenant_" + name,
		"running": online,
	}
}

func (f *fakeVerge) recordedActions() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, len(f.actions))
	copy(out, f.actions)
	return out
}

func (f *fakeVerge) recordedVNetActions() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, len(f.vnetActions))
	copy(out, f.vnetActions)
	return out
}

func (f *fakeVerge) recordedNodeActions() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, len(f.nodeActions))
	copy(out, f.nodeActions)
	return out
}

func (f *fakeVerge) recordedCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeVerge) callCount(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, call := range f.calls {
		if call == method+" "+path {
			n++
		}
	}
	return n
}

func (f *fakeVerge) bodiesFor(method, path string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, body := range f.bodies {
		if body.method == method && body.path == path {
			out = append(out, body.body)
		}
	}
	return out
}

func (f *fakeVerge) serve(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	if r.URL.Path == "/version.json" {
		writeJSON(f.t, w, http.StatusOK, map[string]string{"version": "26.1.8"})
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
		http.Error(w, "read", http.StatusInternalServerError)
		return
	}
	var payload map[string]any
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			f.t.Errorf("decode %s %s: %v body %s", r.Method, r.URL.Path, err, raw)
			http.Error(w, "json", http.StatusBadRequest)
			return
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	if payload != nil {
		f.bodies = append(f.bodies, recordedBody{method: r.Method, path: r.URL.Path, body: payload})
	}

	collection, id, ok := splitAPIPath(r.URL.Path)
	if !ok {
		f.t.Errorf("unexpected path %s", r.URL.Path)
		http.Error(w, "path", http.StatusNotFound)
		return
	}
	switch collection {
	case "tenants":
		f.serveTenants(w, r, id, payload)
	case "tenant_status":
		f.serveStatus(w, r)
	case "tenant_actions":
		f.serveTenantAction(w, payload)
	case "tenant_nodes":
		f.serveNodes(w, r, id, payload)
	case "tenant_node_actions":
		f.serveNodeAction(w, payload)
	case "tenant_storage":
		f.serveStorage(w, r, id, payload)
	case "vnet_addresses":
		f.serveAddress(w, r, id, payload)
	case "vnet_cidrs":
		f.serveCIDR(w, r, id, payload)
	case "tenant_layer2_vnets":
		f.serveLayer2(w, r, id, payload)
	case "vnets":
		f.serveVNet(w, r, id)
	case "vnet_actions":
		f.serveVNetAction(w, payload)
	case "machine_status":
		f.serveMachineStatus(w, r)
	default:
		f.t.Errorf("unexpected collection %s", collection)
		http.Error(w, "collection", http.StatusNotFound)
	}
}

func (f *fakeVerge) serveTenants(w http.ResponseWriter, r *http.Request, id int, payload map[string]any) {
	switch {
	case r.Method == http.MethodPost && id == 0:
		id = f.alloc()
		obj := map[string]any{
			"$key":        id,
			"name":        payload["name"],
			"description": stringField(payload, "description"),
			"url":         stringField(payload, "url"),
			"uuid":        fmt.Sprintf("uuid-%d", id),
			"vnet":        9,
			"isolate":     false,
			"is_snapshot": false,
			"creator":     "admin",
			"created":     int64(1700000000),
		}
		copyPresent(obj, payload, "expose_cloud_snapshots", "allow_branding", "change_password", "theme_access", "help_url", "note", "oidc_application")
		if f.uiIP != "" {
			addressID := 15
			obj["ui_address"] = addressID
			f.addresses[addressID] = map[string]any{
				"$key": addressID,
				"ip":   f.uiIP,
				"type": "static",
				"vnet": 3,
			}
		}
		f.tenants[id] = obj
		f.status[id] = statusObject(id, false)
		f.vnets[9] = map[string]any{
			"$key":    9,
			"name":    fmt.Sprintf("tenant_%v", payload["name"]),
			"running": false,
		}
		writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
	case r.Method == http.MethodGet && id == 0:
		writeJSON(f.t, w, http.StatusOK, f.filterMaps(f.tenantSlice(), r.URL.Query().Get("filter")))
	case r.Method == http.MethodGet && id > 0:
		// Create reads the new tenant back. failTenantGet is how many of
		// those reads should succeed before a later provider read fails.
		if f.failTenantGet > 0 {
			f.tenantGets++
			if f.tenantGets > f.failTenantGet {
				writeJSON(f.t, w, http.StatusInternalServerError, map[string]string{"err": "read failed"})
				return
			}
		}
		obj, ok := f.tenants[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		writeJSON(f.t, w, http.StatusOK, obj)
	case r.Method == http.MethodPut && id > 0:
		obj, ok := f.tenants[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		for k, v := range payload {
			obj[k] = v
		}
		writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
	case r.Method == http.MethodDelete && id > 0:
		obj, ok := f.tenants[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		if vnetID := intField(obj["vnet"]); vnetID > 0 {
			if vnet, ok := f.vnets[vnetID]; ok {
				if running, _ := vnet["running"].(bool); running {
					writeJSON(f.t, w, http.StatusMethodNotAllowed, map[string]string{
						"err": "Tenant network must be powered off to delete tenant",
					})
					return
				}
			}
		}
		delete(f.tenants, id)
		delete(f.status, id)
		if vnetID := intField(obj["vnet"]); vnetID > 0 {
			delete(f.vnets, vnetID)
		}
		w.WriteHeader(http.StatusOK)
	default:
		f.t.Errorf("unexpected tenants %s id %d", r.Method, id)
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (f *fakeVerge) serveStatus(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}

	if r.Method != http.MethodGet {
		f.t.Errorf("unexpected tenant_status %s", r.Method)
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	for id := range f.status {
		f.advancePower(id)
	}
	rows := make([]map[string]any, 0, len(f.status))
	for _, row := range f.status {
		rows = append(rows, row)
	}
	writeJSON(f.t, w, http.StatusOK, f.filterMaps(rows, r.URL.Query().Get("filter")))
}

// advancePower lets one transitional status poll be observed, then settles
// to terminal online/offline on the next poll when transitionPower is set.
func (f *fakeVerge) advancePower(id int) {
	if !f.transitionPower || f.holdPower {
		return
	}
	st := f.status[id]
	if st == nil {
		return
	}
	starting, _ := st["starting"].(bool)
	stopping, _ := st["stopping"].(bool)
	if !starting && !stopping {
		return
	}
	if !f.seenTransition[id] {
		f.seenTransition[id] = true
		return
	}
	if starting {
		f.status[id] = statusObject(id, true)
		f.setTenantVNetRunning(id, true)
	} else {
		f.status[id] = statusObject(id, false)
		f.setTenantVNetRunning(id, false)
	}
	delete(f.seenTransition, id)
}

func (f *fakeVerge) serveTenantAction(w http.ResponseWriter, payload map[string]any) {
	f.actions = append(f.actions, payload)
	id := intField(payload["tenant"])
	action, _ := payload["action"].(string)
	if action == "isolateon" || action == "isolateoff" {
		if f.isolateFail {
			writeJSON(f.t, w, http.StatusInternalServerError, map[string]string{"err": "isolate failed"})
			return
		}
		if !f.isolateIgnore {
			if tenant, ok := f.tenants[id]; ok {
				tenant["isolate"] = action == "isolateon"
			}
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	if f.holdPower {
		w.WriteHeader(http.StatusOK)
		return
	}
	switch action {
	case "poweron":
		if f.transitionPower {
			f.status[id] = statusObjectTransitional(id, true)
			f.seenTransition[id] = false
		} else {
			f.status[id] = statusObject(id, true)
		}
		f.setTenantVNetRunning(id, true)
	case "poweroff":
		if f.transitionPower {
			f.status[id] = statusObjectTransitional(id, false)
			f.seenTransition[id] = false
			// Vnet stays running until status settles offline (mirrors lab lag).
			f.setTenantVNetRunning(id, true)
		} else {
			f.status[id] = statusObject(id, false)
			f.setTenantVNetRunning(id, false)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fakeVerge) setTenantVNetRunning(tenantID int, running bool) {
	tenant, ok := f.tenants[tenantID]
	if !ok {
		return
	}
	vnetID := intField(tenant["vnet"])
	if vnetID <= 0 {
		return
	}
	vnet, ok := f.vnets[vnetID]
	if !ok {
		vnet = map[string]any{"$key": vnetID, "name": fmt.Sprintf("tenant_%d", tenantID)}
		f.vnets[vnetID] = vnet
	}
	vnet["running"] = running
}

func (f *fakeVerge) serveNodes(w http.ResponseWriter, r *http.Request, id int, payload map[string]any) {
	if r.Method == http.MethodDelete && id > 0 {
		if node, ok := f.nodes[id]; ok {
			machineID := intField(node["machine"])
			if st := f.machines[machineID]; st != nil {
				if running, _ := st["running"].(bool); running {
					writeJSON(f.t, w, http.StatusMethodNotAllowed, map[string]string{
						"err": "Tenant node cannot be deleted while running",
					})
					return
				}
			}
		}
		if f.nodeDeleteFailStatus > 0 {
			msg := f.nodeDeleteFailMessage
			if msg == "" {
				msg = "forced node delete failure"
			}
			writeJSON(f.t, w, f.nodeDeleteFailStatus, map[string]string{"err": msg})
			return
		}
		if f.lastNodeDeleteFails > 0 {
			f.lastNodeDeleteFails--
			writeJSON(f.t, w, http.StatusMethodNotAllowed, map[string]string{
				"err": "Only the last node can be deleted",
			})
			return
		}
	}
	f.serveCollection(w, r, id, payload, f.nodes, func(id int, payload map[string]any) map[string]any {
		machineID := 80 + id
		f.machines[machineID] = map[string]any{
			"$key":    machineID,
			"machine": machineID,
			"running": false,
			"status":  "stopped",
			"state":   "offline",
		}
		return map[string]any{
			"$key":          id,
			"tenant":        intField(payload["tenant"]),
			"name":          stringField(payload, "name"),
			"description":   stringField(payload, "description"),
			"enabled":       boolField(payload, "enabled"),
			"cpu_cores":     intField(payload["cpu_cores"]),
			"ram":           intField(payload["ram"]),
			"nodeid":        1,
			"machine":       machineID,
			"is_snapshot":   false,
			"creator":       "admin",
			"created":       int64(1700000000),
			"modified":      int64(1700000001),
			"on_power_loss": stringField(payload, "on_power_loss"),
			"ha_group":      stringField(payload, "ha_group"),
		}
	})
}

func (f *fakeVerge) serveStorage(w http.ResponseWriter, r *http.Request, id int, payload map[string]any) {
	f.serveCollection(w, r, id, payload, f.storage, func(id int, payload map[string]any) map[string]any {
		return map[string]any{
			"$key":        id,
			"tenant":      intField(payload["tenant"]),
			"tier":        intField(payload["tier"]),
			"provisioned": int64Field(payload["provisioned"]),
			"used":        int64(0),
			"allocated":   int64(0),
			"used_pct":    0,
		}
	})
}

func (f *fakeVerge) serveCollection(w http.ResponseWriter, r *http.Request, id int, payload map[string]any, table map[int]map[string]any, create func(int, map[string]any) map[string]any) {
	switch {
	case r.Method == http.MethodPost && id == 0:
		id = f.alloc()
		obj := create(id, payload)
		copyPresent(obj, payload, "cluster", "cluster_failover", "preferred_node")
		table[id] = obj
		writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
	case r.Method == http.MethodGet && id == 0:
		rows := make([]map[string]any, 0, len(table))
		for _, row := range table {
			rows = append(rows, row)
		}
		writeJSON(f.t, w, http.StatusOK, f.filterMaps(rows, r.URL.Query().Get("filter")))
	case r.Method == http.MethodGet && id > 0:
		obj, ok := table[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		writeJSON(f.t, w, http.StatusOK, obj)
	case r.Method == http.MethodPut && id > 0:
		obj, ok := table[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		for k, v := range payload {
			obj[k] = v
		}
		writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
	case r.Method == http.MethodDelete && id > 0:
		if _, ok := table[id]; !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		delete(table, id)
		w.WriteHeader(http.StatusOK)
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (f *fakeVerge) serveVNet(w http.ResponseWriter, r *http.Request, id int) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}
	switch {
	case r.Method == http.MethodGet && id == 0:
		rows := make([]map[string]any, 0, len(f.vnets))
		for _, row := range f.vnets {
			rows = append(rows, row)
		}
		writeJSON(f.t, w, http.StatusOK, f.filterMaps(rows, r.URL.Query().Get("filter")))
	case r.Method == http.MethodGet && id > 0:
		obj, ok := f.vnets[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		writeJSON(f.t, w, http.StatusOK, f.vnetRead(obj, id))
	default:
		f.t.Errorf("unexpected vnets %s id %d", r.Method, id)
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (f *fakeVerge) serveVNetAction(w http.ResponseWriter, payload map[string]any) {
	f.vnetActions = append(f.vnetActions, payload)
	id := intField(payload["vnet"])
	action, _ := payload["action"].(string)
	if action == "kill" || action == "poweroff" {
		if vnet, ok := f.vnets[id]; ok {
			vnet["running"] = false
		}
	}
	if action == "refresh" {
		if vnet, ok := f.vnets[id]; ok {
			if f.fwSettleReads > 0 {
				vnet["need_fw_apply"] = true
				if f.fwSettleLeft == nil {
					f.fwSettleLeft = map[int]int{}
				}
				f.fwSettleLeft[id] = f.fwSettleReads
			} else {
				vnet["need_fw_apply"] = false
				delete(f.fwSettleLeft, id)
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

// vnetRead reports need_fw_apply for one GET. The first fwSettleReads
// reads after a refresh stay pending; the stored flag is clear after that.
func (f *fakeVerge) vnetRead(obj map[string]any, id int) map[string]any {
	left := f.fwSettleLeft[id]
	if left <= 0 {
		return obj
	}
	left--
	if left == 0 {
		delete(f.fwSettleLeft, id)
		obj["need_fw_apply"] = false
	} else {
		f.fwSettleLeft[id] = left
	}
	reported := make(map[string]any, len(obj))
	for key, value := range obj {
		reported[key] = value
	}
	reported["need_fw_apply"] = true
	return reported
}

func (f *fakeVerge) serveNodeAction(w http.ResponseWriter, payload map[string]any) {
	f.nodeActions = append(f.nodeActions, payload)
	id := intField(payload["tenant_node"])
	action, _ := payload["action"].(string)
	if action == "poweroff" && f.nodePowerOffNotRunning {
		writeJSON(f.t, w, http.StatusUnprocessableEntity, map[string]string{
			"err": "Tenant node must be in running state to poweroff",
		})
		return
	}
	if action == "poweroff" && f.nodePowerOffStuck {
		w.WriteHeader(http.StatusOK)
		return
	}
	if action == "kill" && f.nodeKillStuck {
		w.WriteHeader(http.StatusOK)
		return
	}
	if action == "kill" || action == "poweroff" {
		if node, ok := f.nodes[id]; ok {
			machineID := intField(node["machine"])
			if st := f.machines[machineID]; st != nil {
				st["running"] = false
				st["status"] = "stopped"
				st["state"] = "offline"
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fakeVerge) serveMachineStatus(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		f.t.Errorf("unexpected machine_status %s", r.Method)
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows := make([]map[string]any, 0, len(f.machines))
	for _, row := range f.machines {
		rows = append(rows, row)
	}
	writeJSON(f.t, w, http.StatusOK, f.filterMaps(rows, r.URL.Query().Get("filter")))
}

func (f *fakeVerge) serveAddress(w http.ResponseWriter, r *http.Request, id int, payload map[string]any) {
	switch {
	case r.Method == http.MethodGet && id == 0:
		writeJSON(f.t, w, http.StatusOK, f.filterMaps(f.addressSlice(), r.URL.Query().Get("filter")))
	case r.Method == http.MethodGet && id > 0:
		obj, ok := f.addresses[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		writeJSON(f.t, w, http.StatusOK, obj)
	case r.Method == http.MethodPost && id == 0:
		id = f.alloc()
		obj := map[string]any{
			"$key":        id,
			"vnet":        intField(payload["vnet"]),
			"ip":          stringField(payload, "ip"),
			"type":        stringField(payload, "type"),
			"owner":       stringField(payload, "owner"),
			"hostname":    stringField(payload, "hostname"),
			"description": stringField(payload, "description"),
		}
		f.addresses[id] = obj
		f.markFirewallPending(intField(obj["vnet"]))
		f.assignFirstUIAddress(obj)
		writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
	case r.Method == http.MethodDelete && id > 0:
		obj, ok := f.addresses[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		vnetID := intField(obj["vnet"])
		delete(f.addresses, id)
		f.markFirewallPending(vnetID)
		w.WriteHeader(http.StatusOK)
	default:
		f.t.Errorf("unexpected address %s %d", r.Method, id)
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (f *fakeVerge) serveCIDR(w http.ResponseWriter, r *http.Request, id int, payload map[string]any) {
	switch {
	case r.Method == http.MethodGet && id == 0:
		writeJSON(f.t, w, http.StatusOK, f.filterMaps(f.cidrSlice(), r.URL.Query().Get("filter")))
	case r.Method == http.MethodGet && id > 0:
		obj, ok := f.cidrs[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		writeJSON(f.t, w, http.StatusOK, obj)
	case r.Method == http.MethodPost && id == 0:
		id = f.alloc()
		vnetID := intField(payload["vnet"])
		obj := map[string]any{
			"$key":         id,
			"vnet":         vnetID,
			"network_name": fmt.Sprintf("vnet-%d", vnetID),
			"cidr":         stringField(payload, "cidr"),
			"owner":        stringField(payload, "owner"),
			"description":  stringField(payload, "description"),
		}
		f.cidrs[id] = obj
		f.markFirewallPending(vnetID)
		writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
	case r.Method == http.MethodDelete && id > 0:
		obj, ok := f.cidrs[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		if f.cidrDeleteStatus != 0 {
			writeJSON(f.t, w, f.cidrDeleteStatus, map[string]string{"err": f.cidrDeleteMessage})
			return
		}
		vnetID := intField(obj["vnet"])
		delete(f.cidrs, id)
		f.markFirewallPending(vnetID)
		w.WriteHeader(http.StatusOK)
	default:
		f.t.Errorf("unexpected cidr %s %d", r.Method, id)
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (f *fakeVerge) serveLayer2(w http.ResponseWriter, r *http.Request, id int, payload map[string]any) {
	switch {
	case r.Method == http.MethodGet && id == 0:
		writeJSON(f.t, w, http.StatusOK, f.filterMaps(f.layer2Slice(), r.URL.Query().Get("filter")))
	case r.Method == http.MethodGet && id > 0:
		obj, ok := f.layer2[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		writeJSON(f.t, w, http.StatusOK, obj)
	case r.Method == http.MethodPost && id == 0:
		id = f.alloc()
		enabled := true
		if _, ok := payload["enabled"]; ok {
			enabled = boolField(payload, "enabled")
		}
		f.layer2[id] = map[string]any{
			"$key":    id,
			"tenant":  intField(payload["tenant"]),
			"vnet":    intField(payload["vnet"]),
			"enabled": enabled,
		}
		writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
	case r.Method == http.MethodPut && id > 0:
		obj, ok := f.layer2[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		if f.layer2DisableStatus != 0 && payload["enabled"] == false {
			writeJSON(f.t, w, f.layer2DisableStatus, map[string]string{"err": f.layer2DisableMessage})
			return
		}
		if _, ok := payload["enabled"]; ok {
			obj["enabled"] = boolField(payload, "enabled")
		}
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodDelete && id > 0:
		obj, ok := f.layer2[id]
		if !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		if boolField(obj, "enabled") {
			writeJSON(f.t, w, http.StatusBadRequest, map[string]string{"err": "layer 2 network must be disabled before it is deleted"})
			return
		}
		if f.layer2DeleteStatus != 0 {
			writeJSON(f.t, w, f.layer2DeleteStatus, map[string]string{"err": f.layer2DeleteMessage})
			return
		}
		delete(f.layer2, id)
		w.WriteHeader(http.StatusOK)
	default:
		f.t.Errorf("unexpected layer2 %s %d", r.Method, id)
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (f *fakeVerge) layer2Slice() []map[string]any {
	rows := make([]map[string]any, 0, len(f.layer2))
	for _, row := range f.layer2 {
		rows = append(rows, row)
	}
	return rows
}

func (f *fakeVerge) cidrSlice() []map[string]any {
	rows := make([]map[string]any, 0, len(f.cidrs))
	for _, row := range f.cidrs {
		rows = append(rows, row)
	}
	return rows
}

func (f *fakeVerge) addressSlice() []map[string]any {
	rows := make([]map[string]any, 0, len(f.addresses))
	for _, row := range f.addresses {
		rows = append(rows, row)
	}
	return rows
}

func (f *fakeVerge) markFirewallPending(vnetID int) {
	if vnetID <= 0 {
		return
	}
	vnet, ok := f.vnets[vnetID]
	if !ok {
		vnet = map[string]any{
			"$key":    vnetID,
			"name":    fmt.Sprintf("vnet-%d", vnetID),
			"running": false,
		}
		f.vnets[vnetID] = vnet
	}
	vnet["need_fw_apply"] = true
}

// assignFirstUIAddress models VergeOS: the first virtual IP owned by a tenant
// becomes that tenant's UI address.
func (f *fakeVerge) assignFirstUIAddress(address map[string]any) {
	if stringField(address, "type") != "virtual" {
		return
	}
	tenantID := tenantIDFromOwner(stringField(address, "owner"))
	tenant, ok := f.tenants[tenantID]
	if !ok || intField(tenant["ui_address"]) > 0 {
		return
	}
	tenant["ui_address"] = intField(address["$key"])
}

func tenantIDFromOwner(owner string) int {
	rest, ok := strings.CutPrefix(owner, "tenants/")
	if !ok || rest == "" {
		return 0
	}
	id, err := strconv.Atoi(rest)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func (f *fakeVerge) alloc() int {
	id := f.next
	f.next++
	return id
}

func (f *fakeVerge) tenantSlice() []map[string]any {
	rows := make([]map[string]any, 0, len(f.tenants))
	for _, row := range f.tenants {
		rows = append(rows, row)
	}
	return rows
}

func (f *fakeVerge) filterMaps(rows []map[string]any, filter string) []map[string]any {
	if strings.TrimSpace(filter) == "" {
		return rows
	}
	kept := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if matchFilter(filter, row) {
			kept = append(kept, row)
		}
	}
	return kept
}

func matchFilter(filter string, row map[string]any) bool {
	for _, part := range strings.Split(filter, " and ") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "name eq '"):
			name := strings.TrimSuffix(strings.TrimPrefix(part, "name eq '"), "'")
			if stringField(row, "name") != name {
				return false
			}
		case strings.HasPrefix(part, "tenant eq "):
			want, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "tenant eq ")))
			if err != nil || intField(row["tenant"]) != want {
				return false
			}
		case strings.HasPrefix(part, "vnet eq "):
			want, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "vnet eq ")))
			if err != nil || intField(row["vnet"]) != want {
				return false
			}
		case strings.HasPrefix(part, "machine eq "):
			want, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "machine eq ")))
			if err != nil || intField(row["machine"]) != want {
				return false
			}
		case strings.HasPrefix(part, "$key eq "):
			want, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "$key eq ")))
			if err != nil || intField(row["$key"]) != want {
				return false
			}
		case strings.HasPrefix(part, "type eq '"):
			want := strings.TrimSuffix(strings.TrimPrefix(part, "type eq '"), "'")
			if stringField(row, "type") != want {
				return false
			}
		case strings.HasPrefix(part, "owner bw '"):
			prefix := strings.TrimSuffix(strings.TrimPrefix(part, "owner bw '"), "'")
			if !strings.HasPrefix(stringField(row, "owner"), prefix) {
				return false
			}
		case strings.HasPrefix(part, "owner eq '"):
			want := strings.TrimSuffix(strings.TrimPrefix(part, "owner eq '"), "'")
			if stringField(row, "owner") != want {
				return false
			}
		case strings.HasPrefix(part, "ip eq '"):
			want := strings.TrimSuffix(strings.TrimPrefix(part, "ip eq '"), "'")
			if stringField(row, "ip") != want {
				return false
			}
		case strings.HasPrefix(part, "cidr eq '"):
			want := strings.TrimSuffix(strings.TrimPrefix(part, "cidr eq '"), "'")
			if stringField(row, "cidr") != want {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func statusObject(tenantID int, online bool) map[string]any {
	if online {
		return map[string]any{
			"$key":     tenantID,
			"tenant":   tenantID,
			"running":  true,
			"starting": false,
			"stopping": false,
			"status":   "online",
			"state":    "online",
		}
	}
	return map[string]any{
		"$key":     tenantID,
		"tenant":   tenantID,
		"running":  false,
		"starting": false,
		"stopping": false,
		"status":   "offline",
		"state":    "offline",
	}
}

func statusObjectTransitional(tenantID int, starting bool) map[string]any {
	if starting {
		return map[string]any{
			"$key":     tenantID,
			"tenant":   tenantID,
			"running":  false,
			"starting": true,
			"stopping": false,
			"status":   "starting",
			"state":    "offline",
		}
	}
	return map[string]any{
		"$key":     tenantID,
		"tenant":   tenantID,
		"running":  true,
		"starting": false,
		"stopping": true,
		"status":   "stopping",
		"state":    "online",
	}
}

func splitAPIPath(path string) (string, int, bool) {
	path = strings.TrimPrefix(path, "/api/v4/")
	path = strings.Trim(path, "/")
	if path == "" {
		return "", 0, false
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		return parts[0], 0, true
	}
	if len(parts) != 2 {
		return "", 0, false
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return parts[0], id, true
}

func copyPresent(dst, src map[string]any, keys ...string) {
	for _, key := range keys {
		if v, ok := src[key]; ok {
			dst[key] = v
		}
	}
}

func stringField(obj map[string]any, key string) string {
	v, _ := obj[key].(string)
	return v
}

func boolField(obj map[string]any, key string) bool {
	v, _ := obj[key].(bool)
	return v
}

func intField(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func int64Field(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
