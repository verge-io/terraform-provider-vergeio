// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestSnapshotProfileResourceMetadata(t *testing.T) {
	resp := &fwresource.MetadataResponse{}
	NewSnapshotProfileResource().Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
	if resp.TypeName != "vergeio_snapshot_profile" {
		t.Fatalf("type = %s", resp.TypeName)
	}
}

func TestSnapshotProfileResourceSchema(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewSnapshotProfileResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if !strings.Contains(resp.Schema.MarkdownDescription, "no default") {
		t.Fatalf("description should state retention has no default: %s", resp.Schema.MarkdownDescription)
	}
	name, ok := resp.Schema.Attributes["name"]
	if !ok || !name.IsRequired() {
		t.Fatal("name should be required")
	}
	block, ok := resp.Schema.Blocks["period"].(resschema.ListNestedBlock)
	if !ok {
		t.Fatal("period should be a list nested block")
	}
	retention, ok := block.NestedObject.Attributes["retention"].(resschema.Int64Attribute)
	if !ok || !retention.Required || retention.Optional {
		t.Fatal("retention should be a required number")
	}
	if retention.Default != nil {
		t.Fatal("retention must not have a default")
	}
	if !strings.Contains(retention.MarkdownDescription, "seconds") || !strings.Contains(retention.MarkdownDescription, "no default") {
		t.Fatalf("retention description = %s", retention.MarkdownDescription)
	}
	frequency, ok := block.NestedObject.Attributes["frequency"].(resschema.StringAttribute)
	if !ok || !frequency.Required {
		t.Fatal("frequency should be required")
	}
	for _, name := range []string{"hour", "minute", "quiesce"} {
		attr, ok := block.NestedObject.Attributes[name]
		if !ok || !attr.IsOptional() {
			t.Fatalf("%s should be optional", name)
		}
	}
	quiesce, ok := block.NestedObject.Attributes["quiesce"].(resschema.BoolAttribute)
	if !ok || quiesce.Default != nil {
		t.Fatal("quiesce must not have a default")
	}
}

func TestSnapshotProfileResourceConfigure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	profile := &SnapshotProfileResource{}
	resp := &fwresource.ConfigureResponse{}
	profile.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() || profile.api == nil || profile.api.Name() != "Snapshot Profile Api" {
		t.Fatalf("configure failed: %v api=%v", resp.Diagnostics, profile.api)
	}

	profile = &SnapshotProfileResource{}
	resp = &fwresource.ConfigureResponse{}
	profile.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() || profile.api != nil {
		t.Fatal("invalid client should fail configure")
	}

	profile = &SnapshotProfileResource{}
	resp = &fwresource.ConfigureResponse{}
	profile.Configure(context.Background(), fwresource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || profile.api != nil {
		t.Fatal("nil provider data should leave the resource unconfigured")
	}
}

func TestSnapshotProfileImportState(t *testing.T) {
	ctx := context.Background()
	profile := &SnapshotProfileResource{}
	schemaResp := &fwresource.SchemaResponse{}
	profile.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	resp := &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	profile.ImportState(ctx, fwresource.ImportStateRequest{ID: "15"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got SnapshotProfileResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Id.ValueString() != "15" {
		t.Fatalf("id = %q, want 15", got.Id.ValueString())
	}

	resp = &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	profile.ImportState(ctx, fwresource.ImportStateRequest{ID: "nightly"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("non-numeric import id should fail")
	}
}

func TestSnapshotProfileCreateUpdateDelete(t *testing.T) {
	fake := newProfileFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := context.Background()
	profile := configuredSnapshotProfile(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	profile.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &SnapshotProfileResourceModel{
		Name:        types.StringValue("nightly"),
		Description: types.StringValue("Nightly VM snapshots"),
		Period: []periodModel{{
			Name:      types.StringValue("nightly"),
			Frequency: types.StringValue("daily"),
			Hour:      types.Int32Value(2),
			Minute:    types.Int32Value(0),
			Retention: types.Int64Value(604800),
			Quiesce:   types.BoolValue(true),
		}},
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	created := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	profile.Create(ctx, fwresource.CreateRequest{Plan: plan}, created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	var got SnapshotProfileResourceModel
	created.Diagnostics.Append(created.State.Get(ctx, &got)...)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	if got.Id.ValueString() == "" || got.Name.ValueString() != "nightly" {
		t.Fatalf("profile = %+v", got)
	}
	if len(got.Period) != 1 {
		t.Fatalf("periods = %d", len(got.Period))
	}
	nightly := got.Period[0]
	if nightly.Retention.ValueInt64() != 604800 || nightly.Hour.ValueInt32() != 2 || nightly.Minute.ValueInt32() != 0 || !nightly.Quiesce.ValueBool() {
		t.Fatalf("period = %+v", nightly)
	}
	if nightly.Key.ValueString() == "" {
		t.Fatal("period key was not stored")
	}
	body := fake.periodCreate("nightly")
	if body.Retention != 604800 {
		t.Fatalf("create retention = %d, want 604800", body.Retention)
	}
	if body.Minute == nil || *body.Minute != 0 || body.Hour == nil || *body.Hour != 2 {
		t.Fatalf("time of day was not sent: %+v", body)
	}
	if body.Quiesce == nil || !*body.Quiesce {
		t.Fatal("quiesce true was not sent")
	}

	state := created.State
	updatePlan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = updatePlan.Set(ctx, &SnapshotProfileResourceModel{
		Id:          got.Id,
		Name:        types.StringValue("nightly"),
		Description: types.StringValue("Nightly and weekly VM snapshots"),
		Period: []periodModel{
			{
				Name:      types.StringValue("nightly"),
				Frequency: types.StringValue("daily"),
				Hour:      types.Int32Value(2),
				Minute:    types.Int32Value(0),
				Retention: types.Int64Value(1209600),
				Quiesce:   types.BoolValue(false),
			},
			{
				Name:      types.StringValue("weekly"),
				Frequency: types.StringValue("weekly"),
				DayOfWeek: types.StringValue("sun"),
				Hour:      types.Int32Value(1),
				Minute:    types.Int32Value(0),
				Retention: types.Int64Value(2419200),
				Quiesce:   types.BoolValue(true),
			},
		},
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	updated := &fwresource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	profile.Update(ctx, fwresource.UpdateRequest{Plan: updatePlan, State: state}, updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	updated.Diagnostics.Append(updated.State.Get(ctx, &got)...)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	if got.Description.ValueString() != "Nightly and weekly VM snapshots" || len(got.Period) != 2 {
		t.Fatalf("updated profile periods = %d description %q", len(got.Period), got.Description.ValueString())
	}
	if got.Period[0].Name.ValueString() != "nightly" || got.Period[1].Name.ValueString() != "weekly" {
		t.Fatalf("period order = %s, %s", got.Period[0].Name.ValueString(), got.Period[1].Name.ValueString())
	}
	if got.Period[0].Retention.ValueInt64() != 1209600 || got.Period[0].Quiesce.ValueBool() {
		t.Fatalf("updated nightly = %+v", got.Period[0])
	}
	put := fake.periodUpdate("nightly")
	if put.Retention == nil || *put.Retention != 1209600 {
		t.Fatalf("update retention = %+v", put.Retention)
	}
	if put.Frequency != nil {
		t.Fatalf("unchanged frequency was sent: %s", *put.Frequency)
	}
	if put.Quiesce == nil || *put.Quiesce {
		t.Fatal("quiesce false was not sent")
	}
	weekly := fake.periodCreate("weekly")
	if weekly.Retention != 2419200 {
		t.Fatalf("weekly retention = %d", weekly.Retention)
	}

	dropPlan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = dropPlan.Set(ctx, &SnapshotProfileResourceModel{
		Id:          got.Id,
		Name:        got.Name,
		Description: got.Description,
		Period: []periodModel{{
			Name:      types.StringValue("nightly"),
			Frequency: types.StringValue("daily"),
			Retention: types.Int64Value(1209600),
		}},
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	dropped := &fwresource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	profile.Update(ctx, fwresource.UpdateRequest{Plan: dropPlan, State: updated.State}, dropped)
	if dropped.Diagnostics.HasError() {
		t.Fatal(dropped.Diagnostics)
	}
	dropped.Diagnostics.Append(dropped.State.Get(ctx, &got)...)
	if dropped.Diagnostics.HasError() {
		t.Fatal(dropped.Diagnostics)
	}
	if len(got.Period) != 1 || got.Period[0].Name.ValueString() != "nightly" {
		t.Fatalf("periods after delete = %+v", got.Period)
	}
	if !fake.periodDeleted("weekly") {
		t.Fatal("weekly period was not deleted")
	}

	deleteResp := &fwresource.DeleteResponse{}
	profile.Delete(ctx, fwresource.DeleteRequest{State: dropped.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal(deleteResp.Diagnostics)
	}
	if fake.profileExists(got.Id.ValueString()) {
		t.Fatal("profile was not deleted")
	}
}

func TestSnapshotProfileCreateKeepsIDWhenPeriodFails(t *testing.T) {
	fake := newProfileFake()
	fake.failPeriodCreate = true
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := context.Background()
	profile := configuredSnapshotProfile(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	profile.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &SnapshotProfileResourceModel{
		Name: types.StringValue("nightly"),
		Period: []periodModel{{
			Name:      types.StringValue("nightly"),
			Frequency: types.StringValue("daily"),
			Retention: types.Int64Value(604800),
		}},
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	profile.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected period create to fail")
	}
	var got SnapshotProfileResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Id.ValueString() == "" {
		t.Fatal("profile id was not kept after the period create failed")
	}
}

func TestSnapshotProfileReadRemovesMissingProfile(t *testing.T) {
	fake := newProfileFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := context.Background()
	profile := configuredSnapshotProfile(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	profile.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &SnapshotProfileResourceModel{
		Id:   types.StringValue("9"),
		Name: types.StringValue("missing"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	profile.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing profile should be removed from state")
	}
}

func TestSnapshotProfileDuplicatePeriodDoesNotCreate(t *testing.T) {
	fake := newProfileFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := context.Background()
	profile := configuredSnapshotProfile(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	profile.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &SnapshotProfileResourceModel{
		Name: types.StringValue("nightly"),
		Period: []periodModel{
			{Name: types.StringValue("nightly"), Frequency: types.StringValue("daily"), Retention: types.Int64Value(604800)},
			{Name: types.StringValue("nightly"), Frequency: types.StringValue("daily"), Retention: types.Int64Value(86400)},
		},
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	profile.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("duplicate period names should fail")
	}
	if fake.profileCount() != 0 {
		t.Fatal("duplicate periods should be rejected before the profile is created")
	}
}

func configuredSnapshotProfile(t *testing.T, host string) *SnapshotProfileResource {
	t.Helper()
	profile := &SnapshotProfileResource{}
	resp := &fwresource.ConfigureResponse{}
	profile.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return profile
}

type profileFake struct {
	mu               sync.Mutex
	next             int
	profiles         map[int]vergeos.SnapshotProfile
	periods          map[int]vergeos.SnapshotProfilePeriod
	periodCreates    map[string]vergeos.SnapshotProfilePeriodCreateRequest
	periodUpdates    map[string]vergeos.SnapshotProfilePeriodUpdateRequest
	deletedPeriods   map[string]bool
	failPeriodCreate bool
}

func newProfileFake() *profileFake {
	return &profileFake{
		next:           1,
		profiles:       map[int]vergeos.SnapshotProfile{},
		periods:        map[int]vergeos.SnapshotProfilePeriod{},
		periodCreates:  map[string]vergeos.SnapshotProfilePeriodCreateRequest{},
		periodUpdates:  map[string]vergeos.SnapshotProfilePeriodUpdateRequest{},
		deletedPeriods: map[string]bool{},
	}
}

func (f *profileFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/version.json" {
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/snapshot_profiles":
		f.createProfile(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/snapshot_profiles/"):
		f.getProfile(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/snapshot_profiles/"):
		f.updateProfile(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/snapshot_profiles/"):
		f.deleteProfile(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/snapshot_profile_periods":
		f.listPeriods(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/snapshot_profile_periods":
		f.createPeriod(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/snapshot_profile_periods/"):
		f.getPeriod(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/snapshot_profile_periods/"):
		f.updatePeriod(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/snapshot_profile_periods/"):
		f.deletePeriod(w, r)
	default:
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
	}
}

func (f *profileFake) createProfile(w http.ResponseWriter, r *http.Request) {
	var req vergeos.SnapshotProfileCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	id := f.next
	f.next++
	f.profiles[id] = vergeos.SnapshotProfile{Key: vergeos.FlexInt(id), Name: req.Name, Description: req.Description}
	f.mu.Unlock()
	writeJSON(w, map[string]int{"$key": id})
}

func (f *profileFake) getProfile(w http.ResponseWriter, r *http.Request) {
	id := pathID(r.URL.Path)
	f.mu.Lock()
	profile, ok := f.profiles[id]
	f.mu.Unlock()
	if !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, profile)
}

func (f *profileFake) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req vergeos.SnapshotProfileUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := pathID(r.URL.Path)
	f.mu.Lock()
	defer f.mu.Unlock()
	profile, ok := f.profiles[id]
	if !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Description != nil {
		profile.Description = *req.Description
	}
	if req.IgnoreWarnings != nil {
		profile.IgnoreWarnings = *req.IgnoreWarnings
	}
	f.profiles[id] = profile
	w.WriteHeader(http.StatusOK)
}

func (f *profileFake) deleteProfile(w http.ResponseWriter, r *http.Request) {
	id := pathID(r.URL.Path)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.profiles[id]; !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	delete(f.profiles, id)
	w.WriteHeader(http.StatusOK)
}

func (f *profileFake) listPeriods(w http.ResponseWriter, r *http.Request) {
	profileID := filterProfileID(r.URL.Query().Get("filter"))
	f.mu.Lock()
	defer f.mu.Unlock()
	periods := make([]vergeos.SnapshotProfilePeriod, 0)
	for _, period := range f.periods {
		if profileID != 0 && period.Profile.Int() != profileID {
			continue
		}
		periods = append(periods, period)
	}
	sort.Slice(periods, func(i, j int) bool {
		return periods[i].Key.Int() > periods[j].Key.Int()
	})
	writeJSON(w, periods)
}

func (f *profileFake) createPeriod(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req vergeos.SnapshotProfilePeriodCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Retention <= 0 {
		http.Error(w, `{"err":"retention is required"}`, http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPeriodCreate {
		http.Error(w, `{"err":"unavailable"}`, http.StatusInternalServerError)
		return
	}
	f.periodCreates[req.Name] = req
	id := f.next
	f.next++
	f.periods[id] = vergeos.SnapshotProfilePeriod{
		Key:          vergeos.FlexInt(id),
		Profile:      vergeos.FlexInt(req.Profile),
		Name:         req.Name,
		Frequency:    req.Frequency,
		Hour:         intOrZero(req.Hour),
		Minute:       intOrZero(req.Minute),
		DayOfWeek:    stringOrEmpty(req.DayOfWeek),
		DayOfMonth:   intOrZero(req.DayOfMonth),
		Month:        intOrZero(req.Month),
		Retention:    req.Retention,
		Quiesce:      boolOrFalse(req.Quiesce),
		SkipMissed:   boolOrFalse(req.SkipMissed),
		MaxTier:      stringOrEmpty(req.MaxTier),
		MinSnapshots: intOrZero(req.MinSnapshots),
		Immutable:    boolOrFalse(req.Immutable),
	}
	writeJSON(w, map[string]int{"$key": id})
}

func (f *profileFake) getPeriod(w http.ResponseWriter, r *http.Request) {
	id := pathID(r.URL.Path)
	f.mu.Lock()
	period, ok := f.periods[id]
	f.mu.Unlock()
	if !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, period)
}

func (f *profileFake) updatePeriod(w http.ResponseWriter, r *http.Request) {
	var req vergeos.SnapshotProfilePeriodUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := pathID(r.URL.Path)
	f.mu.Lock()
	defer f.mu.Unlock()
	period, ok := f.periods[id]
	if !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	f.periodUpdates[period.Name] = req
	if req.Name != nil {
		period.Name = *req.Name
	}
	if req.Frequency != nil {
		period.Frequency = *req.Frequency
	}
	if req.Hour != nil {
		period.Hour = *req.Hour
	}
	if req.Minute != nil {
		period.Minute = *req.Minute
	}
	if req.DayOfWeek != nil {
		period.DayOfWeek = *req.DayOfWeek
	}
	if req.DayOfMonth != nil {
		period.DayOfMonth = *req.DayOfMonth
	}
	if req.Month != nil {
		period.Month = *req.Month
	}
	if req.Retention != nil {
		period.Retention = *req.Retention
	}
	if req.Quiesce != nil {
		period.Quiesce = *req.Quiesce
	}
	if req.SkipMissed != nil {
		period.SkipMissed = *req.SkipMissed
	}
	if req.MaxTier != nil {
		period.MaxTier = *req.MaxTier
	}
	if req.MinSnapshots != nil {
		period.MinSnapshots = *req.MinSnapshots
	}
	if req.Immutable != nil {
		period.Immutable = *req.Immutable
	}
	f.periods[id] = period
	w.WriteHeader(http.StatusOK)
}

func (f *profileFake) deletePeriod(w http.ResponseWriter, r *http.Request) {
	id := pathID(r.URL.Path)
	f.mu.Lock()
	defer f.mu.Unlock()
	period, ok := f.periods[id]
	if !ok {
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		return
	}
	f.deletedPeriods[period.Name] = true
	delete(f.periods, id)
	w.WriteHeader(http.StatusOK)
}

func (f *profileFake) periodCreate(name string) vergeos.SnapshotProfilePeriodCreateRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.periodCreates[name]
}

func (f *profileFake) periodUpdate(name string) vergeos.SnapshotProfilePeriodUpdateRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.periodUpdates[name]
}

func (f *profileFake) periodDeleted(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deletedPeriods[name]
}

func (f *profileFake) profileExists(id string) bool {
	n, err := strconv.Atoi(id)
	if err != nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.profiles[n]
	return ok
}

func (f *profileFake) profileCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.profiles)
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

func filterProfileID(filter string) int {
	const prefix = "profile eq "
	if !strings.Contains(filter, prefix) {
		return 0
	}
	fields := strings.Fields(filter)
	for i, field := range fields {
		if field == "eq" && i > 0 && fields[i-1] == "profile" && i+1 < len(fields) {
			id, _ := strconv.Atoi(fields[i+1])
			return id
		}
	}
	return 0
}

func intOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func boolOrFalse(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}
