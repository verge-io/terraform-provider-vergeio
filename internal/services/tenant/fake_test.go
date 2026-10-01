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

	mu            sync.Mutex
	next          int
	uiIP          string
	holdPower     bool
	tenantGets    int
	failTenantGet int
	tenants       map[int]map[string]any
	status        map[int]map[string]any
	addresses     map[int]map[string]any
	nodes         map[int]map[string]any
	storage       map[int]map[string]any
	calls         []string
	bodies        []recordedBody
	actions       []map[string]any
}

func newFake(t *testing.T) *fakeVerge {
	t.Helper()
	return &fakeVerge{
		t:         t,
		next:      1,
		tenants:   map[int]map[string]any{},
		status:    map[int]map[string]any{},
		addresses: map[int]map[string]any{},
		nodes:     map[int]map[string]any{},
		storage:   map[int]map[string]any{},
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
}

func (f *fakeVerge) recordedActions() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, len(f.actions))
	copy(out, f.actions)
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
	case "tenant_storage":
		f.serveStorage(w, r, id, payload)
	case "vnet_addresses":
		f.serveAddress(w, r, id)
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
		if _, ok := f.tenants[id]; !ok {
			writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
			return
		}
		delete(f.tenants, id)
		delete(f.status, id)
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
	rows := make([]map[string]any, 0, len(f.status))
	for _, row := range f.status {
		rows = append(rows, row)
	}
	writeJSON(f.t, w, http.StatusOK, f.filterMaps(rows, r.URL.Query().Get("filter")))
}

func (f *fakeVerge) serveTenantAction(w http.ResponseWriter, payload map[string]any) {
	f.actions = append(f.actions, payload)
	id := intField(payload["tenant"])
	action, _ := payload["action"].(string)
	if !f.holdPower {
		switch action {
		case "poweron":
			f.status[id] = statusObject(id, true)
		case "poweroff":
			f.status[id] = statusObject(id, false)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fakeVerge) serveNodes(w http.ResponseWriter, r *http.Request, id int, payload map[string]any) {
	f.serveCollection(w, r, id, payload, f.nodes, func(id int, payload map[string]any) map[string]any {
		return map[string]any{
			"$key":          id,
			"tenant":        intField(payload["tenant"]),
			"name":          stringField(payload, "name"),
			"description":   stringField(payload, "description"),
			"enabled":       boolField(payload, "enabled"),
			"cpu_cores":     intField(payload["cpu_cores"]),
			"ram":           intField(payload["ram"]),
			"nodeid":        1,
			"machine":       80,
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

func (f *fakeVerge) serveAddress(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != http.MethodGet || id == 0 {
		f.t.Errorf("unexpected address %s %d", r.Method, id)
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	obj, ok := f.addresses[id]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]string{"err": "not found"})
		return
	}
	writeJSON(f.t, w, http.StatusOK, obj)
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
