// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestDNSResourceNames(t *testing.T) {
	ctx := t.Context()
	cases := []struct {
		resource resource.Resource
		name     string
	}{
		{NewNetworkDNSViewResource(), "vergeio_network_dns_view"},
		{NewNetworkDNSZoneResource(), "vergeio_network_dns_zone"},
		{NewNetworkDNSRecordResource(), "vergeio_network_dns_record"},
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
			if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatal(diags)
			}
			if len(schemaResp.Schema.Attributes) == 0 {
				t.Fatal("schema has no attributes")
			}
		})
	}
}

func TestDNSSchemaProseHasNoHyphen(t *testing.T) {
	resources := []resource.Resource{
		NewNetworkDNSViewResource(),
		NewNetworkDNSZoneResource(),
		NewNetworkDNSRecordResource(),
	}
	for _, item := range resources {
		resp := &resource.SchemaResponse{}
		item.Schema(t.Context(), resource.SchemaRequest{}, resp)
		assertNoDashProse(t, "description", resp.Schema.MarkdownDescription)
		for name, attr := range resp.Schema.Attributes {
			assertNoDashProse(t, name, attributeDescription(attr))
		}
	}
}

func TestDNSTemplateProseHasNoHyphen(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	names := []string{
		"network_dns_view.md.tmpl",
		"network_dns_zone.md.tmpl",
		"network_dns_record.md.tmpl",
	}
	for _, name := range names {
		path := filepath.Join(root, "templates", "resources", name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if start := strings.Index(text, "---"); start >= 0 {
			rest := text[start+3:]
			if end := strings.Index(rest, "---"); end >= 0 {
				text = rest[end+3:]
			}
		}
		assertNoDashProse(t, name, stripTemplateActions(text))
	}
}

func TestCreateViewAppliesOnce(t *testing.T) {
	fix := newDNSFixture(t, true)
	api := newDNSTestAPI(t, fix.serve)
	view := &dnsViewModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("internal"),
		Apply:     types.BoolValue(true),
	}
	notice, err := api.createView(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("notice = %#v", notice)
	}
	if view.ID.ValueString() == "" {
		t.Fatal("view id was not stored")
	}
	if fix.applyCount() != 1 || fix.lastAction() != "refresh" || fix.lastTarget() != "dnsonly" {
		t.Fatalf("applies=%d action=%s target=%s", fix.applyCount(), fix.lastAction(), fix.lastTarget())
	}
}

func TestCreateViewSkipsStopped(t *testing.T) {
	fix := newDNSFixture(t, false)
	api := newDNSTestAPI(t, fix.serve)
	view := &dnsViewModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("internal"),
		Apply:     types.BoolValue(true),
	}
	notice, err := api.createView(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || !strings.Contains(notice.Detail, "not running") {
		t.Fatalf("notice = %#v", notice)
	}
	if fix.applyCount() != 0 {
		t.Fatalf("applies = %d, want 0", fix.applyCount())
	}
	if len(fix.snapshot("view")) != 1 {
		t.Fatal("stopped create did not keep the view")
	}
}

func TestCreateViewApplyFalseStages(t *testing.T) {
	fix := newDNSFixture(t, true)
	api := newDNSTestAPI(t, fix.serve)
	view := &dnsViewModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("internal"),
		Apply:     types.BoolValue(false),
	}
	notice, err := api.createView(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || !strings.Contains(notice.Detail, "apply is false") {
		t.Fatalf("notice = %#v", notice)
	}
	if fix.applyCount() != 0 {
		t.Fatalf("applies = %d, want 0", fix.applyCount())
	}
}

func TestPendingDNSFlagApplies(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.needDNS = true
	fix.put("view", 1, map[string]any{"$key": 1, "vnet": 12, "name": "internal"})
	api := newDNSTestAPI(t, fix.serve)
	state := dnsViewModel{
		ID:        types.StringValue("1"),
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("internal"),
		Apply:     types.BoolValue(true),
	}
	plan := state
	notice, err := api.updateView(t.Context(), &plan, &state)
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("notice = %#v", notice)
	}
	if fix.applyCount() != 1 {
		t.Fatalf("applies = %d, want 1", fix.applyCount())
	}
	if fix.countPrefix("PUT /api/v4/vnet_dns_views/") != 0 {
		t.Fatalf("pending flag wrote the view: %v", fix.callLog())
	}
	plan = state
	if _, err := api.updateView(t.Context(), &plan, &state); err != nil {
		t.Fatal(err)
	}
	if fix.applyCount() != 1 {
		t.Fatalf("second update applies = %d, want 1", fix.applyCount())
	}
}

func TestBatchApplyOnce(t *testing.T) {
	fix := newDNSFixture(t, true)
	api := newDNSTestAPI(t, fix.serve)
	view := &dnsViewModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("internal"),
		Apply:     types.BoolValue(false),
	}
	if _, err := api.createView(t.Context(), view); err != nil {
		t.Fatal(err)
	}
	zone := &dnsZoneModel{
		ViewID: view.ID,
		Domain: types.StringValue("example.com"),
		Type:   types.StringValue("master"),
		Apply:  types.BoolValue(false),
	}
	if _, err := api.createZone(t.Context(), zone); err != nil {
		t.Fatal(err)
	}
	record := &dnsRecordModel{
		ZoneID: zone.ID,
		Host:   types.StringValue("www"),
		Type:   types.StringValue("A"),
		Value:  types.StringValue("192.0.2.10"),
		Apply:  types.BoolValue(true),
	}
	notice, err := api.createRecord(t.Context(), record)
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("notice = %#v", notice)
	}
	if fix.applyCount() != 1 || fix.lastTarget() != "dnsonly" {
		t.Fatalf("applies=%d target=%s calls=%v", fix.applyCount(), fix.lastTarget(), fix.callLog())
	}
}

func TestRecordUsesNICAddress(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.put("view", 1, map[string]any{"$key": 1, "vnet": 12, "name": "internal"})
	fix.put("zone", 2, map[string]any{"$key": 2, "view": 1, "domain": "example.com", "type": "master"})
	fix.nics[5] = "192.0.2.40"
	api := newDNSTestAPI(t, fix.serve)
	record := &dnsRecordModel{
		ZoneID:  types.StringValue("2"),
		Type:    types.StringValue(vergeos.DNSRecordTypeA),
		VMNICID: types.StringValue("5"),
		Value:   types.StringNull(),
		Apply:   types.BoolValue(true),
	}
	if _, err := api.createRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if record.Value.ValueString() != "192.0.2.40" {
		t.Fatalf("value = %#v", record.Value)
	}
	if record.VMNICID.ValueString() != "5" {
		t.Fatalf("vm_nic_id = %#v", record.VMNICID)
	}
	stored := fix.snapshot("record")
	if len(stored) != 1 || stored[0]["value"] != "192.0.2.40" {
		t.Fatalf("stored records = %#v", stored)
	}
}

func TestRecordRejectsNICWithoutAddress(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.nics[5] = ""
	api := newDNSTestAPI(t, fix.serve)
	record := &dnsRecordModel{
		ZoneID:  types.StringValue("2"),
		Type:    types.StringValue("A"),
		VMNICID: types.StringValue("5"),
		Value:   types.StringNull(),
		Apply:   types.BoolValue(true),
	}
	_, err := api.createRecord(t.Context(), record)
	if err == nil || !strings.Contains(err.Error(), "no IP address") {
		t.Fatalf("err = %v", err)
	}
	if len(fix.snapshot("record")) != 0 {
		t.Fatal("record was created without an address")
	}
}

func TestRecordRejectsPlanThatDiffersFromNIC(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.nics[5] = "192.0.2.40"
	api := newDNSTestAPI(t, fix.serve)
	record := &dnsRecordModel{
		ZoneID:  types.StringValue("2"),
		Type:    types.StringValue("AAAA"),
		VMNICID: types.StringValue("5"),
		Value:   types.StringValue("192.0.2.1"),
		Apply:   types.BoolValue(true),
	}
	_, err := api.createRecord(t.Context(), record)
	if err == nil || !strings.Contains(err.Error(), "plan has") {
		t.Fatalf("err = %v", err)
	}
}

func TestViewDeleteIsOneCallWhenCascadeAccepts(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.put("view", 1, map[string]any{"$key": 1, "vnet": 12, "name": "internal"})
	fix.put("zone", 2, map[string]any{"$key": 2, "view": 1, "domain": "example.com"})
	fix.put("record", 3, map[string]any{"$key": 3, "zone": 2, "type": "A", "value": "192.0.2.10"})
	api := newDNSTestAPI(t, fix.serve)
	view := &dnsViewModel{
		ID:        types.StringValue("1"),
		NetworkID: types.StringValue("12"),
		Apply:     types.BoolValue(true),
	}
	if _, err := api.deleteView(t.Context(), view); err != nil {
		t.Fatal(err)
	}
	if fix.countExact("DELETE /api/v4/vnet_dns_views/1") != 1 {
		t.Fatalf("view deletes = %d, calls = %v", fix.countExact("DELETE /api/v4/vnet_dns_views/1"), fix.callLog())
	}
	if fix.countPrefix("DELETE /api/v4/vnet_dns_zones/") != 0 || fix.countPrefix("DELETE /api/v4/vnet_dns_zone_records/") != 0 {
		t.Fatalf("cascade delete walked children: %v", fix.callLog())
	}
	if len(fix.snapshot("zone")) != 1 {
		t.Fatal("accepted view delete should leave child cleanup to VergeOS")
	}
}

func TestZoneDeleteRemovesRecordsWhenRefused(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.refuseZoneDeletes = 1
	fix.put("view", 1, map[string]any{"$key": 1, "vnet": 12, "name": "internal"})
	fix.put("zone", 2, map[string]any{"$key": 2, "view": 1, "domain": "example.com"})
	fix.put("record", 3, map[string]any{"$key": 3, "zone": 2, "type": "A", "value": "192.0.2.10"})
	api := newDNSTestAPI(t, fix.serve)
	zone := &dnsZoneModel{
		ID:        types.StringValue("2"),
		ViewID:    types.StringValue("1"),
		NetworkID: types.StringValue("12"),
		Apply:     types.BoolValue(true),
	}
	if _, err := api.deleteZone(t.Context(), zone); err != nil {
		t.Fatal(err)
	}
	if len(fix.snapshot("zone")) != 0 || len(fix.snapshot("record")) != 0 {
		t.Fatalf("zones=%v records=%v", fix.snapshot("zone"), fix.snapshot("record"))
	}
	if !callBefore(fix.callLog(), "DELETE /api/v4/vnet_dns_zone_records/3", "DELETE /api/v4/vnet_dns_zones/2") {
		t.Fatalf("record was not removed before the zone retry: %v", fix.callLog())
	}
	if fix.countExact("DELETE /api/v4/vnet_dns_zones/2") != 2 {
		t.Fatalf("zone deletes = %d, calls = %v", fix.countExact("DELETE /api/v4/vnet_dns_zones/2"), fix.callLog())
	}
}

func TestDeleteNetworkDNSRowsDeletesViews(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.put("view", 1, map[string]any{"$key": 1, "vnet": 12, "name": "internal"})
	fix.put("view", 4, map[string]any{"$key": 4, "vnet": 12, "name": "guest"})
	fix.put("view", 9, map[string]any{"$key": 9, "vnet": 99, "name": "other"})
	fix.put("zone", 2, map[string]any{"$key": 2, "view": 1, "domain": "example.com"})
	api := newDNSTestAPI(t, fix.serve)
	if err := DeleteNetworkDNSRows(t.Context(), api.sdk, 12); err != nil {
		t.Fatal(err)
	}
	left := fix.snapshot("view")
	if len(left) != 1 || asInt(left[0]["vnet"]) != 99 {
		t.Fatalf("views left = %#v", left)
	}
	if len(fix.snapshot("zone")) != 1 {
		t.Fatal("accepted view delete walked zones")
	}
}

func TestPartialCreateRemovesViewWhenApplyFails(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.failApply = 1
	api := newDNSTestAPI(t, fix.serve)
	view := &dnsViewModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("internal"),
		Apply:     types.BoolValue(true),
	}
	_, err := api.createView(t.Context(), view)
	if err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("err = %v", err)
	}
	if len(fix.snapshot("view")) != 0 {
		t.Fatalf("view left in place: %#v", fix.snapshot("view"))
	}
}

func TestReadViewParentMismatch(t *testing.T) {
	fix := newDNSFixture(t, true)
	fix.put("view", 1, map[string]any{"$key": 1, "vnet": 99, "name": "internal"})
	api := newDNSTestAPI(t, fix.serve)
	view := &dnsViewModel{
		ID:        types.StringValue("1"),
		NetworkID: types.StringValue("12"),
	}
	err := api.readView(t.Context(), view)
	if err == nil || !vergeos.IsNotFoundError(err) {
		t.Fatalf("err = %v", err)
	}
}

func newDNSTestAPI(t *testing.T, handler http.HandlerFunc) *DNSApi {
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
	return &DNSApi{sdk: sdk}
}

type dnsFixture struct {
	mu                sync.Mutex
	t                 *testing.T
	running           bool
	needDNS           bool
	failApply         int
	refuseViewDeletes int
	refuseZoneDeletes int
	next              int
	views             map[int]map[string]any
	zones             map[int]map[string]any
	records           map[int]map[string]any
	nics              map[int]string
	calls             []string
	applies           int
	action            string
	target            string
}

func newDNSFixture(t *testing.T, running bool) *dnsFixture {
	t.Helper()
	return &dnsFixture{
		t:       t,
		running: running,
		next:    20,
		views:   map[int]map[string]any{},
		zones:   map[int]map[string]any{},
		records: map[int]map[string]any{},
		nics:    map[int]string{},
	}
}

func (f *dnsFixture) put(kind string, key int, row map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch kind {
	case "view":
		f.views[key] = row
	case "zone":
		f.zones[key] = row
	case "record":
		f.records[key] = row
	default:
		f.t.Fatalf("unknown kind %s", kind)
	}
}

func (f *dnsFixture) snapshot(kind string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var rows map[int]map[string]any
	switch kind {
	case "view":
		rows = f.views
	case "zone":
		rows = f.zones
	case "record":
		rows = f.records
	default:
		f.t.Fatalf("unknown kind %s", kind)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func (f *dnsFixture) applyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.applies
}

func (f *dnsFixture) lastAction() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.action
}

func (f *dnsFixture) lastTarget() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.target
}

func (f *dnsFixture) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *dnsFixture) countExact(call string) int {
	n := 0
	for _, got := range f.callLog() {
		if got == call {
			n++
		}
	}
	return n
}

func (f *dnsFixture) countPrefix(prefix string) int {
	n := 0
	for _, got := range f.callLog() {
		if strings.HasPrefix(got, prefix) {
			n++
		}
	}
	return n
}

func (f *dnsFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_dns_views":
		writeJSON(f.t, w, http.StatusOK, filterRows(f.views, r.URL.Query().Get("filter")))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_dns_views":
		f.createRow(f.views, w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_views/"):
		f.getRow(f.views, w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_views/"):
		f.updateRow(f.views, w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_views/"):
		f.deleteRow(f.views, &f.refuseViewDeletes, w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_dns_zones":
		writeJSON(f.t, w, http.StatusOK, filterRows(f.zones, r.URL.Query().Get("filter")))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_dns_zones":
		f.createRow(f.zones, w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_zones/"):
		f.getRow(f.zones, w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_zones/"):
		f.updateRow(f.zones, w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_zones/"):
		f.deleteRow(f.zones, &f.refuseZoneDeletes, w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_dns_zone_records":
		writeJSON(f.t, w, http.StatusOK, filterRows(f.records, r.URL.Query().Get("filter")))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_dns_zone_records":
		f.createRow(f.records, w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_zone_records/"):
		f.getRow(f.records, w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_zone_records/"):
		f.updateRow(f.records, w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_dns_zone_records/"):
		f.deleteRow(f.records, nil, w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/machine_nics/"):
		f.getNIC(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
		f.getNetwork(w)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
		f.apply(w, r)
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}
}

func (f *dnsFixture) createRow(rows map[int]map[string]any, w http.ResponseWriter, r *http.Request) {
	body := readMap(f.t, r)
	f.next++
	body["$key"] = f.next
	rows[f.next] = body
	f.needDNS = true
	writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": f.next})
}

func (f *dnsFixture) getRow(rows map[int]map[string]any, w http.ResponseWriter, r *http.Request) {
	row, ok := rows[pathKey(r.URL.Path)]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	writeJSON(f.t, w, http.StatusOK, row)
}

func (f *dnsFixture) updateRow(rows map[int]map[string]any, w http.ResponseWriter, r *http.Request) {
	row, ok := rows[pathKey(r.URL.Path)]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	for k, v := range readMap(f.t, r) {
		row[k] = v
	}
	f.needDNS = true
	writeJSON(f.t, w, http.StatusOK, map[string]any{})
}

func (f *dnsFixture) deleteRow(rows map[int]map[string]any, refuse *int, w http.ResponseWriter, r *http.Request) {
	key := pathKey(r.URL.Path)
	if _, ok := rows[key]; !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	if refuse != nil && *refuse > 0 {
		*refuse--
		writeJSON(f.t, w, http.StatusConflict, map[string]any{"err": "in use"})
		return
	}
	delete(rows, key)
	f.needDNS = true
	w.WriteHeader(http.StatusOK)
}

func (f *dnsFixture) getNIC(w http.ResponseWriter, r *http.Request) {
	key := pathKey(r.URL.Path)
	ip, ok := f.nics[key]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	writeJSON(f.t, w, http.StatusOK, map[string]any{
		"$key":      key,
		"name":      "nic",
		"ipaddress": ip,
		"enabled":   true,
	})
}

func (f *dnsFixture) getNetwork(w http.ResponseWriter) {
	status := "stopped"
	if f.running {
		status = "running"
	}
	writeJSON(f.t, w, http.StatusOK, map[string]any{
		"$key":           12,
		"name":           "lan",
		"running":        f.running,
		"status":         status,
		"need_dns_apply": f.needDNS,
	})
}

func (f *dnsFixture) apply(w http.ResponseWriter, r *http.Request) {
	body := readMap(f.t, r)
	if f.failApply > 0 {
		f.failApply--
		writeJSON(f.t, w, http.StatusInternalServerError, map[string]any{"err": "apply failed"})
		return
	}
	f.applies++
	f.action, _ = body["action"].(string)
	if params, ok := body["params"].(map[string]any); ok {
		f.target, _ = params["target"].(string)
	}
	f.needDNS = false
	writeJSON(f.t, w, http.StatusOK, map[string]any{})
}

func filterRows(rows map[int]map[string]any, filter string) []map[string]any {
	field, want, limited := parseEq(filter)
	out := make([]map[string]any, 0)
	for _, row := range rows {
		if limited && asInt(row[field]) != want {
			continue
		}
		out = append(out, row)
	}
	return out
}

func parseEq(filter string) (string, int, bool) {
	field, value, ok := strings.Cut(strings.TrimSpace(filter), " eq ")
	if !ok {
		return "", 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return "", 0, false
	}
	return strings.TrimSpace(field), n, true
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func attributeDescription(attr schema.Attribute) string {
	switch item := attr.(type) {
	case schema.StringAttribute:
		return item.MarkdownDescription
	case schema.BoolAttribute:
		return item.MarkdownDescription
	case schema.Int64Attribute:
		return item.MarkdownDescription
	default:
		return ""
	}
}

func assertNoDashProse(t *testing.T, name, text string) {
	t.Helper()
	stripped := stripBackticks(text)
	if strings.ContainsRune(stripped, '-') || strings.ContainsRune(stripped, '\u2014') || strings.ContainsRune(stripped, '\u2013') {
		t.Errorf("%s prose has a dash outside backticks: %s", name, text)
	}
}

func stripTemplateActions(text string) string {
	var b strings.Builder
	for {
		start := strings.Index(text, "{{")
		if start < 0 {
			b.WriteString(text)
			break
		}
		b.WriteString(text[:start])
		rest := text[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			b.WriteString(rest)
			break
		}
		text = rest[end+2:]
	}
	return b.String()
}

func stripBackticks(text string) string {
	var b strings.Builder
	in := false
	for _, r := range text {
		if r == '`' {
			in = !in
			continue
		}
		if !in {
			b.WriteRune(r)
		}
	}
	return b.String()
}
