// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/services/snapshot"
)

func TestSiteSchema(t *testing.T) {
	ctx := context.Background()
	resp := &fwresource.SchemaResponse{}
	NewSiteResource().Schema(ctx, fwresource.SchemaRequest{}, resp)
	for _, name := range []string{"created", "modified", "last_stat_update"} {
		attr, ok := resp.Schema.Attributes[name].(resschema.Int64Attribute)
		if !ok || len(attr.PlanModifiers) != 0 {
			t.Fatalf("%s must be computed without UseStateForUnknown", name)
		}
	}
	status, ok := resp.Schema.Attributes["status"].(resschema.StringAttribute)
	if !ok || len(status.PlanModifiers) != 0 {
		t.Fatal("status must not use UseStateForUnknown")
	}
	password, ok := resp.Schema.Attributes["auth_password_wo"].(resschema.StringAttribute)
	if !ok || !password.WriteOnly || !password.Sensitive {
		t.Fatal("auth_password_wo must be write-only and sensitive")
	}

	incoming := &fwresource.SchemaResponse{}
	NewSyncIncomingResource().Schema(ctx, fwresource.SchemaRequest{}, incoming)
	siteID, ok := incoming.Schema.Attributes["site_id"].(resschema.StringAttribute)
	if !ok || !requiresReplace(siteID.PlanModifiers) {
		t.Fatal("incoming site_id must require replace")
	}
	code, ok := incoming.Schema.Attributes["registration_code"].(resschema.StringAttribute)
	if !ok || !code.Sensitive || len(code.PlanModifiers) != 0 {
		t.Fatal("registration_code must be sensitive and computed without UseStateForUnknown")
	}

	outgoing := &fwresource.SchemaResponse{}
	NewSyncOutgoingResource().Schema(ctx, fwresource.SchemaRequest{}, outgoing)
	outSite, ok := outgoing.Schema.Attributes["site_id"].(resschema.StringAttribute)
	if !ok || !requiresReplace(outSite.PlanModifiers) {
		t.Fatal("outgoing site_id must require replace")
	}
	block, ok := outgoing.Schema.Blocks["period"].(resschema.ListNestedBlock)
	if !ok {
		t.Fatal("period should be a list nested block")
	}
	retention, ok := block.NestedObject.Attributes["retention"].(resschema.Int64Attribute)
	if !ok || !retention.Required || retention.Optional || retention.Default != nil {
		t.Fatal("period retention must be required and have no default")
	}
	reg, ok := outgoing.Schema.Attributes["registration_code_wo"].(resschema.StringAttribute)
	if !ok || !reg.WriteOnly || !reg.Sensitive {
		t.Fatal("registration_code_wo must be write-only and sensitive")
	}
	if _, exists := outgoing.Schema.Attributes["registration_code"]; exists {
		t.Fatal("outgoing registration code is write-only and must not be stored as registration_code")
	}

	profile := &fwresource.SchemaResponse{}
	snapshot.NewSnapshotProfileResource().Schema(ctx, fwresource.SchemaRequest{}, profile)
	if !strings.Contains(profile.Schema.MarkdownDescription, "Cloud snapshots use this same profile") {
		t.Fatal("snapshot profile should state that cloud snapshots use the same profile")
	}
	meta := &fwresource.MetadataResponse{}
	NewSiteResource().Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName == "vergeio_cloud_snapshot_profile" {
		t.Fatal("cloud snapshots are not a separate resource")
	}
}

func TestSiteImportRejectsNonNumeric(t *testing.T) {
	ctx := context.Background()
	item := &siteResource{}
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	resp := &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	item.ImportState(ctx, fwresource.ImportStateRequest{ID: "remote"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("non-numeric import id should fail")
	}
	resp = &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	item.ImportState(ctx, fwresource.ImportStateRequest{ID: "15"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got siteModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.ID.ValueString() != "15" {
		t.Fatalf("id = %q", got.ID.ValueString())
	}
}

func TestSiteResourceCreateDropsPassword(t *testing.T) {
	fake := newSiteFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := &siteResource{}
	configured := &fwresource.ConfigureResponse{}
	item.Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configured)
	if configured.Diagnostics.HasError() {
		t.Fatal(configured.Diagnostics)
	}
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	model := &siteModel{
		Name:                  types.StringValue("remote"),
		URL:                   types.StringValue("https://203.0.113.10"),
		Enabled:               types.BoolValue(false),
		AuthUser:              types.StringValue("sync"),
		AuthPasswordWOVersion: types.Int64Value(1),
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, model); diags.HasError() {
		t.Fatal(diags)
	}
	configModel := *model
	configModel.AuthPasswordWO = types.StringValue("secret")
	configState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := configState.Set(ctx, &configModel); diags.HasError() {
		t.Fatalf("config set: %v", diags)
	}
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: configState.Raw}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	item.Create(ctx, fwresource.CreateRequest{Plan: plan, Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got siteModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.ID.ValueString() == "" || got.SiteID.ValueString() != "abc123sha" || !got.AuthPasswordWO.IsNull() {
		t.Fatalf("state id=%s site_id=%s password null=%t", got.ID.ValueString(), got.SiteID.ValueString(), got.AuthPasswordWO.IsNull())
	}
	if !strings.Contains(fake.siteCreateBody, `"auth_password":"secret"`) || !strings.Contains(fake.siteCreateBody, `"automatically_create_syncs":false`) {
		t.Fatalf("create body = %s", fake.siteCreateBody)
	}
}

func TestSiteForStateDropsPassword(t *testing.T) {
	stored := siteForState(&siteModel{
		ID:                    types.StringValue("4"),
		AuthPasswordWO:        types.StringValue("secret"),
		AuthPasswordWOVersion: types.Int64Value(2),
		AuthUser:              types.StringValue("sync"),
	})
	if !stored.AuthPasswordWO.IsNull() {
		t.Fatal("password must be null in state")
	}
	if stored.AuthPasswordWOVersion.ValueInt64() != 2 || stored.AuthUser.ValueString() != "sync" {
		t.Fatalf("version and user = %#v", stored)
	}
}

func TestSiteCreateUpdateAndRead(t *testing.T) {
	fake := newSiteFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)

	ctx := context.Background()
	data := &siteModel{
		Name:                  types.StringValue("remote"),
		URL:                   types.StringValue("https://203.0.113.10"),
		Enabled:               types.BoolValue(false),
		Description:           types.StringValue("lab"),
		City:                  types.StringValue("Austin"),
		ConfigCloudSnapshots:  types.StringValue("disabled"),
		AuthUser:              types.StringValue("sync"),
		AuthPasswordWOVersion: types.Int64Value(1),
	}
	if err := api.createSite(ctx, data, "secret"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.siteCreateBody, `"automatically_create_syncs":false`) {
		t.Fatalf("create body = %s", fake.siteCreateBody)
	}
	if !strings.Contains(fake.siteCreateBody, `"auth_password":"secret"`) {
		t.Fatalf("create body missing password: %s", fake.siteCreateBody)
	}
	if data.ID.ValueString() == "" || data.SiteID.ValueString() != "abc123sha" {
		t.Fatalf("id=%s site_id=%s", data.ID.ValueString(), data.SiteID.ValueString())
	}
	if !data.AuthPasswordWO.IsNull() || data.AuthUser.ValueString() != "sync" {
		t.Fatalf("user/password after create = %#v", data)
	}

	plain := &siteModel{
		Name:    types.StringValue("plain"),
		URL:     types.StringValue("https://203.0.113.11"),
		Enabled: types.BoolValue(false),
	}
	if err := api.createSite(ctx, plain, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fake.siteCreateBody, "auth_password") {
		t.Fatalf("unset password was sent: %s", fake.siteCreateBody)
	}

	updated := *data
	updated.Description = types.StringValue("paired")
	updated.AuthPasswordWOVersion = types.Int64Value(2)
	if err := api.updateSite(ctx, &updated, data, "next-secret"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fake.siteUpdateBody, "auth_password") {
		t.Fatalf("update sent auth_password: %s", fake.siteUpdateBody)
	}
	if !strings.Contains(fake.siteUpdateBody, `"remote_password":"next-secret"`) || !strings.Contains(fake.siteUpdateBody, `"remote_user":"sync"`) {
		t.Fatalf("update body = %s", fake.siteUpdateBody)
	}
	if updated.Description.ValueString() != "paired" || updated.AuthUser.ValueString() != "sync" {
		t.Fatalf("after update = %#v", updated)
	}
	if !updated.AuthPasswordWO.IsNull() {
		t.Fatal("update stored the password")
	}
}

func TestIncomingReadsSiteAndRegistrationCode(t *testing.T) {
	fake := newSiteFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)
	api.codeAttempts = 1
	api.codeInterval = 0

	ctx := context.Background()
	data := &incomingModel{
		SiteID:       types.StringValue("4"),
		Name:         types.StringValue("from-remote"),
		Enabled:      types.BoolValue(true),
		MinSnapshots: types.Int64Value(2),
	}
	if err := api.createIncoming(ctx, data); err != nil {
		t.Fatal(err)
	}
	if data.SiteID.ValueString() != "4" {
		t.Fatalf("site_id = %s", data.SiteID.ValueString())
	}
	if data.RegistrationCode.ValueString() != "reg-1" {
		t.Fatalf("registration_code = %#v", data.RegistrationCode)
	}
	if fake.incomingGets[mustAtoi(data.ID.ValueString())] < 2 {
		t.Fatal("create should read the registration code once more after the create read")
	}

	again := *data
	if err := api.readIncoming(ctx, &again); err != nil {
		t.Fatal(err)
	}
	reads := fake.incomingGets[mustAtoi(data.ID.ValueString())]
	if err := api.readIncoming(ctx, &again); err != nil {
		t.Fatal(err)
	}
	if fake.incomingGets[mustAtoi(data.ID.ValueString())] != reads+1 {
		t.Fatal("read must not poll for the registration code")
	}
}

func TestOutgoingPeriodsAndRegistrationCode(t *testing.T) {
	fake := newSiteFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)
	ctx := context.Background()

	data := &outgoingModel{
		SiteID:                    types.StringValue("4"),
		Name:                      types.StringValue("to-remote"),
		Enabled:                   types.BoolValue(false),
		Note:                      types.StringValue("first"),
		RegistrationCodeWOVersion: types.Int64Value(1),
		Period: []syncPeriodModel{{
			ProfilePeriod:     types.StringValue("9"),
			Retention:         types.Int64Value(604800),
			Priority:          types.Int64Value(5),
			DoNotExpire:       types.BoolValue(true),
			DestinationPrefix: types.StringValue("dr-"),
		}},
	}
	if err := api.createOutgoing(ctx, data, "reg-code"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.outgoingCreateBody, `"registration_code":"reg-code"`) {
		t.Fatalf("create body = %s", fake.outgoingCreateBody)
	}
	if data.SiteID.ValueString() != "4" || len(data.Period) != 1 || data.Period[0].Key.ValueString() == "" {
		t.Fatalf("outgoing after create = %#v", data)
	}
	created := fake.periodCreates[0]
	if created.ProfilePeriod != 9 || created.Retention != 604800 || created.SiteSyncsOutgoing == 0 {
		t.Fatalf("period create = %#v", created)
	}
	if created.Priority == nil || *created.Priority != 5 || created.DoNotExpire == nil || !*created.DoNotExpire {
		t.Fatalf("period flags = %#v", created)
	}

	prior := *data
	updated := *data
	updated.Note = types.StringValue("second")
	updated.RegistrationCodeWOVersion = types.Int64Value(2)
	updated.Period = []syncPeriodModel{{
		ProfilePeriod:     types.StringValue("9"),
		Retention:         types.Int64Value(1209600),
		Priority:          types.Int64Value(5),
		DoNotExpire:       types.BoolValue(true),
		DestinationPrefix: types.StringValue("dr-"),
		Key:               data.Period[0].Key,
	}}
	if err := api.updateOutgoing(ctx, &updated, &prior); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fake.outgoingUpdateBody, "registration_code") {
		t.Fatalf("update re-sent registration code: %s", fake.outgoingUpdateBody)
	}
	if len(fake.periodUpdates) != 1 || fake.periodUpdates[0].Retention == nil || *fake.periodUpdates[0].Retention != 1209600 {
		t.Fatalf("period updates = %#v", fake.periodUpdates)
	}
	if updated.RegistrationCodeWOVersion.ValueInt64() != 2 || !updated.RegistrationCodeWO.IsNull() {
		t.Fatalf("version/code after update = %#v %#v", updated.RegistrationCodeWOVersion, updated.RegistrationCodeWO)
	}

	removed := updated
	removed.Period = nil
	if err := api.updateOutgoing(ctx, &removed, &updated); err != nil {
		t.Fatal(err)
	}
	if len(fake.deletedPeriods) != 1 {
		t.Fatalf("deleted periods = %#v", fake.deletedPeriods)
	}
	if removed.Period != nil {
		t.Fatalf("periods after delete = %#v", removed.Period)
	}

	dup := &outgoingModel{
		SiteID: types.StringValue("4"),
		Name:   types.StringValue("dup"),
		Period: []syncPeriodModel{
			{ProfilePeriod: types.StringValue("9"), Retention: types.Int64Value(60)},
			{ProfilePeriod: types.StringValue("9"), Retention: types.Int64Value(60)},
		},
	}
	if err := api.createOutgoing(ctx, dup, ""); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate profile_period error = %v", err)
	}
	if dup.ID.ValueString() != "" {
		t.Fatal("duplicate periods should be rejected before create")
	}
}

func TestSyncLag(t *testing.T) {
	lag, never, err := syncLag("outgoing", "nightly", 0, 100, types.Int64Value(1))
	if err == nil || !strings.Contains(err.Error(), "outgoing sync nightly has not run") {
		t.Fatalf("never synced error = %v", err)
	}
	if !lag.IsNull() || !never.ValueBool() {
		t.Fatalf("lag=%s never=%s", lag, never)
	}
	lag, never, err = syncLag("outgoing", "nightly", 50, 200, types.Int64Value(10))
	if err == nil || !strings.Contains(err.Error(), "last ran 150 seconds ago, past max_lag_seconds 10") {
		t.Fatalf("lag error = %v", err)
	}
	if lag.ValueInt64() != 150 || never.ValueBool() {
		t.Fatalf("lag=%s never=%s", lag, never)
	}
	lag, never, err = syncLag("incoming", "nightly", 190, 200, types.Int64Value(30))
	if err != nil || lag.ValueInt64() != 10 || never.ValueBool() {
		t.Fatalf("within lag: %v %s %s", err, lag, never)
	}
	if _, _, err := syncLag("outgoing", "nightly", 0, 100, types.Int64Null()); err != nil {
		t.Fatal(err)
	}
}

func TestStatusLookup(t *testing.T) {
	fake := newSiteFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)
	api.nowUnix = func() int64 { return 1_000 }
	ctx := context.Background()

	byID := &statusModel{ID: types.StringValue("8")}
	if err := api.readOutgoingStatus(ctx, byID); err != nil {
		t.Fatal(err)
	}
	if byID.Name.ValueString() != "to-remote" || byID.SiteID.ValueString() != "4" || !byID.NeverSynced.ValueBool() {
		t.Fatalf("by id = %#v", byID)
	}
	byName := &statusModel{SiteID: types.StringValue("4"), Name: types.StringValue("to-remote")}
	if err := api.readOutgoingStatus(ctx, byName); err != nil {
		t.Fatal(err)
	}
	if byName.ID.ValueString() != "8" {
		t.Fatalf("by name id = %s", byName.ID.ValueString())
	}
	both := &statusModel{ID: types.StringValue("8"), Name: types.StringValue("to-remote")}
	if err := api.readOutgoingStatus(ctx, both); err == nil || !strings.Contains(err.Error(), "set id, or both site_id and name") {
		t.Fatalf("lookup error = %v", err)
	}
	behind := &statusModel{ID: types.StringValue("8"), MaxLagSeconds: types.Int64Value(1)}
	if err := api.readOutgoingStatus(ctx, behind); err == nil || !strings.Contains(err.Error(), "has not run") {
		t.Fatalf("max lag error = %v", err)
	}
}

func TestStatusDataSourceMetadata(t *testing.T) {
	ctx := context.Background()
	in := &datasource.MetadataResponse{}
	NewSyncIncomingStatusDataSource().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "vergeio"}, in)
	out := &datasource.MetadataResponse{}
	NewSyncOutgoingStatusDataSource().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "vergeio"}, out)
	if in.TypeName != "vergeio_site_sync_incoming_status" || out.TypeName != "vergeio_site_sync_outgoing_status" {
		t.Fatalf("names = %s %s", in.TypeName, out.TypeName)
	}
}

func requiresReplace(mods []planmodifier.String) bool {
	for _, mod := range mods {
		if strings.Contains(mod.Description(context.Background()), "destroy and recreate") {
			return true
		}
	}
	return false
}

func testAPI(t *testing.T, host string) *API {
	t.Helper()
	api, err := NewAPI(vergeio.NewClient(host, "user", "pass", true))
	if err != nil {
		t.Fatal(err)
	}
	api.codeAttempts = 1
	api.codeInterval = 0
	return api
}

func mustAtoi(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}

type siteFake struct {
	mu                 sync.Mutex
	next               int
	sites              map[int]vergeos.Site
	incoming           map[int]vergeos.SiteSyncIncoming
	outgoing           map[int]vergeos.SiteSyncOutgoing
	periods            map[int]vergeos.SiteSyncProfilePeriod
	incomingGets       map[int]int
	siteCreateBody     string
	siteUpdateBody     string
	outgoingCreateBody string
	outgoingUpdateBody string
	periodCreates      []vergeos.SiteSyncProfilePeriodCreateRequest
	periodUpdates      []vergeos.SiteSyncProfilePeriodUpdateRequest
	deletedPeriods     []int
}

func newSiteFake() *siteFake {
	return &siteFake{
		next:         1,
		sites:        map[int]vergeos.Site{},
		incoming:     map[int]vergeos.SiteSyncIncoming{},
		outgoing:     map[int]vergeos.SiteSyncOutgoing{},
		periods:      map[int]vergeos.SiteSyncProfilePeriod{},
		incomingGets: map[int]int{},
	}
}

func (f *siteFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/version.json" {
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/sites":
		f.createSite(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/sites":
		f.listSites(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/sites/"):
		f.getSite(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/sites/"):
		f.updateSite(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/sites/"):
		f.deleteKey(w, r, f.sites)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/site_syncs_incoming":
		f.createIncoming(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/site_syncs_incoming":
		f.listIncoming(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_incoming/"):
		f.getIncoming(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_incoming/"):
		f.updateIncoming(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_incoming/"):
		f.deleteIncoming(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/site_syncs_outgoing":
		f.createOutgoing(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/site_syncs_outgoing":
		f.listOutgoing(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_outgoing/"):
		f.getOutgoing(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_outgoing/"):
		f.updateOutgoing(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_outgoing/"):
		f.deleteOutgoing(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/site_syncs_outgoing_profile_periods":
		f.createPeriod(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/site_syncs_outgoing_profile_periods":
		f.listPeriods(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_outgoing_profile_periods/"):
		f.getPeriod(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_outgoing_profile_periods/"):
		f.updatePeriod(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/site_syncs_outgoing_profile_periods/"):
		f.deletePeriod(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *siteFake) createSite(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.SiteCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	id := f.next
	f.next++
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	f.sites[id] = vergeos.Site{
		Key:                  vergeos.FlexInt(id),
		Name:                 req.Name,
		URL:                  req.URL,
		Description:          req.Description,
		City:                 req.City,
		Enabled:              enabled,
		ID:                   "abc123sha",
		ConfigCloudSnapshots: stringOr(req.ConfigCloudSnapshots, ""),
	}
	f.siteCreateBody = string(body)
	f.mu.Unlock()
	writeJSON(w, map[string]int{"$key": id})
}

func (f *siteFake) listSites(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []vergeos.Site
	for _, item := range f.sites {
		out = append(out, item)
	}
	writeJSON(w, out)
}

func (f *siteFake) getSite(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.sites[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, item)
}

func (f *siteFake) updateSite(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.SiteUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.sites[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.City != nil {
		item.City = *req.City
	}
	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.URL != nil {
		item.URL = *req.URL
	}
	if req.Enabled != nil {
		item.Enabled = *req.Enabled
	}
	if req.RemoteUser != nil {
		item.RemoteUser = *req.RemoteUser
	}
	f.sites[item.Key.Int()] = item
	f.siteUpdateBody = string(body)
	w.WriteHeader(http.StatusOK)
}

func (f *siteFake) createIncoming(w http.ResponseWriter, r *http.Request) {
	var req vergeos.SiteSyncIncomingCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	id := f.next
	f.next++
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	minSnapshots := 0
	if req.MinSnapshots != nil {
		minSnapshots = *req.MinSnapshots
	}
	f.incoming[id] = vergeos.SiteSyncIncoming{
		Key:          vergeos.FlexInt(id),
		Site:         vergeos.FlexInt(req.Site),
		Name:         req.Name,
		Enabled:      enabled,
		MinSnapshots: minSnapshots,
		SyncID:       "syncsha",
	}
	f.mu.Unlock()
	writeJSON(w, map[string]int{"$key": id})
}

func (f *siteFake) listIncoming(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []vergeos.SiteSyncIncoming
	for _, item := range f.incoming {
		if site, ok := filterInt(filter, "site"); ok && item.Site.Int() != site {
			continue
		}
		if name, ok := filterString(filter, "name"); ok && item.Name != name {
			continue
		}
		out = append(out, item)
	}
	writeJSON(w, out)
}

func (f *siteFake) getIncoming(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	item, ok := f.incoming[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	f.incomingGets[id]++
	if f.incomingGets[id] >= 2 && item.RegistrationCode == "" {
		item.RegistrationCode = "reg-1"
		f.incoming[id] = item
	}
	writeJSON(w, item)
}

func (f *siteFake) updateIncoming(w http.ResponseWriter, r *http.Request) {
	var req vergeos.SiteSyncIncomingUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.incoming[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if req.MinSnapshots != nil {
		item.MinSnapshots = *req.MinSnapshots
	}
	if req.Name != nil {
		item.Name = *req.Name
	}
	f.incoming[item.Key.Int()] = item
	w.WriteHeader(http.StatusOK)
}

func (f *siteFake) deleteIncoming(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	if _, ok := f.incoming[id]; !ok {
		http.NotFound(w, r)
		return
	}
	delete(f.incoming, id)
	w.WriteHeader(http.StatusOK)
}

func (f *siteFake) createOutgoing(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.SiteSyncOutgoingCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	id := f.next
	f.next++
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	f.outgoing[id] = vergeos.SiteSyncOutgoing{
		Key:     vergeos.FlexInt(id),
		Site:    vergeos.FlexInt(req.Site),
		Name:    req.Name,
		Enabled: enabled,
		Note:    stringOr(req.Note, ""),
	}
	f.outgoingCreateBody = string(body)
	f.mu.Unlock()
	writeJSON(w, map[string]int{"$key": id})
}

func (f *siteFake) listOutgoing(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []vergeos.SiteSyncOutgoing
	for _, item := range f.outgoing {
		if site, ok := filterInt(filter, "site"); ok && item.Site.Int() != site {
			continue
		}
		if name, ok := filterString(filter, "name"); ok && item.Name != name {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 && strings.Contains(filter, "name eq 'to-remote'") {
		out = append(out, vergeos.SiteSyncOutgoing{Key: 8, Site: 4, Name: "to-remote"})
	}
	writeJSON(w, out)
}

func (f *siteFake) getOutgoing(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	item, ok := f.outgoing[id]
	if !ok && id == 8 {
		item = vergeos.SiteSyncOutgoing{Key: 8, Site: 4, Name: "to-remote"}
		ok = true
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, item)
}

func (f *siteFake) updateOutgoing(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.SiteSyncOutgoingUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.outgoing[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if req.Note != nil {
		item.Note = *req.Note
	}
	if req.Name != nil {
		item.Name = *req.Name
	}
	f.outgoing[item.Key.Int()] = item
	f.outgoingUpdateBody = string(body)
	w.WriteHeader(http.StatusOK)
}

func (f *siteFake) deleteOutgoing(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	if _, ok := f.outgoing[id]; !ok {
		http.NotFound(w, r)
		return
	}
	delete(f.outgoing, id)
	w.WriteHeader(http.StatusOK)
}

func (f *siteFake) createPeriod(w http.ResponseWriter, r *http.Request) {
	var req vergeos.SiteSyncProfilePeriodCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	id := f.next
	f.next++
	item := vergeos.SiteSyncProfilePeriod{
		Key:               vergeos.FlexInt(id),
		SiteSyncsOutgoing: vergeos.FlexInt(req.SiteSyncsOutgoing),
		ProfilePeriod:     vergeos.FlexInt(req.ProfilePeriod),
		Retention:         req.Retention,
		ScheduleTask:      40,
		Task:              41,
	}
	if req.Priority != nil {
		item.Priority = *req.Priority
	}
	if req.DoNotExpire != nil {
		item.DoNotExpire = *req.DoNotExpire
	}
	if req.DestinationPrefix != nil {
		item.DestinationPrefix = *req.DestinationPrefix
	}
	f.periods[id] = item
	f.periodCreates = append(f.periodCreates, req)
	f.mu.Unlock()
	writeJSON(w, map[string]int{"$key": id})
}

func (f *siteFake) listPeriods(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []vergeos.SiteSyncProfilePeriod
	for _, item := range f.periods {
		if syncID, ok := filterInt(filter, "site_syncs_outgoing"); ok && item.SiteSyncsOutgoing.Int() != syncID {
			continue
		}
		out = append(out, item)
	}
	writeJSON(w, out)
}

func (f *siteFake) getPeriod(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.periods[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, item)
}

func (f *siteFake) updatePeriod(w http.ResponseWriter, r *http.Request) {
	var req vergeos.SiteSyncProfilePeriodUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.periods[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if req.Retention != nil {
		item.Retention = *req.Retention
	}
	if req.Priority != nil {
		item.Priority = *req.Priority
	}
	if req.DoNotExpire != nil {
		item.DoNotExpire = *req.DoNotExpire
	}
	if req.DestinationPrefix != nil {
		item.DestinationPrefix = *req.DestinationPrefix
	}
	f.periods[item.Key.Int()] = item
	f.periodUpdates = append(f.periodUpdates, req)
	w.WriteHeader(http.StatusOK)
}

func (f *siteFake) deletePeriod(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	if _, ok := f.periods[id]; !ok {
		http.NotFound(w, r)
		return
	}
	delete(f.periods, id)
	f.deletedPeriods = append(f.deletedPeriods, id)
	w.WriteHeader(http.StatusOK)
}

func (f *siteFake) deleteKey(w http.ResponseWriter, r *http.Request, sites map[int]vergeos.Site) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	if _, ok := sites[id]; !ok {
		http.NotFound(w, r)
		return
	}
	delete(sites, id)
	w.WriteHeader(http.StatusOK)
}

func readBody(r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)
	return body
}

func writeJSON(w http.ResponseWriter, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(encoded)
}

func pathID(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id, _ := strconv.Atoi(parts[len(parts)-1])
	return id
}

func stringOr(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}

func filterInt(filter, field string) (int, bool) {
	token := field + " eq "
	i := strings.Index(filter, token)
	if i < 0 {
		return 0, false
	}
	rest := filter[i+len(token):]
	n := 0
	seen := false
	for _, c := range rest {
		if c < '0' || c > '9' {
			break
		}
		seen = true
		n = n*10 + int(c-'0')
	}
	return n, seen
}

func filterString(filter, field string) (string, bool) {
	token := field + " eq '"
	i := strings.Index(filter, token)
	if i < 0 {
		return "", false
	}
	rest := filter[i+len(token):]
	end := strings.Index(rest, "'")
	if end < 0 {
		return rest, true
	}
	return rest[:end], true
}
