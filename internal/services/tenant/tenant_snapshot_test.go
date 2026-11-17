// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"
)

func TestTenantSnapshotSchema(t *testing.T) {
	r := NewTenantSnapshotResource()
	meta := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_snapshot" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	tenantID := resp.Schema.Attributes["tenant_id"].(schema.StringAttribute)
	if !tenantID.IsRequired() || !stringRequiresReplace(t, tenantID.PlanModifiers) {
		t.Fatal("tenant_id should be required and replace the snapshot")
	}
	name := resp.Schema.Attributes["name"].(schema.StringAttribute)
	if !name.IsOptional() || !name.IsComputed() || !stringRequiresReplace(t, name.PlanModifiers) {
		t.Fatal("name should be optional, computed, and replace the snapshot")
	}
	typ := resp.Schema.Attributes["type"].(schema.StringAttribute)
	if !typ.IsOptional() || !typ.IsComputed() || !stringRequiresReplace(t, typ.PlanModifiers) {
		t.Fatal("type should be optional, computed, and replace the snapshot")
	}
	desc := resp.Schema.Attributes["description"].(schema.StringAttribute)
	if !desc.IsOptional() || !desc.IsComputed() || stringRequiresReplace(t, desc.PlanModifiers) {
		t.Fatal("description should update in place")
	}
	expires := resp.Schema.Attributes["expires"].(schema.Int64Attribute)
	if !expires.IsOptional() || expires.IsComputed() || expires.IsRequired() {
		t.Fatal("expires should be optional")
	}
	never := resp.Schema.Attributes["never_expires"].(schema.BoolAttribute)
	if !never.IsOptional() || !never.IsComputed() || never.Default == nil {
		t.Fatal("never_expires should be optional and computed with a default")
	}
	for _, attrName := range []string{"id", "created"} {
		attr := resp.Schema.Attributes[attrName]
		if attr == nil || !attr.IsComputed() || attr.IsOptional() || attr.IsRequired() {
			t.Fatalf("%s should be computed", attrName)
		}
	}
	if resp.Schema.MarkdownDescription == "" || !containsAll(t, resp.Schema.MarkdownDescription, "action", "state") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestTenantSnapshotNamePlan(t *testing.T) {
	resp := &resource.SchemaResponse{}
	NewTenantSnapshotResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	mods := resp.Schema.Attributes["name"].(schema.StringAttribute).PlanModifiers

	kept, replace := applyStringPlan(t, mods, types.StringValue("snap-1"), types.StringUnknown(), types.StringNull())
	if replace || kept.ValueString() != "snap-1" {
		t.Fatalf("assigned name plan = %s replace=%v", kept, replace)
	}
	renamed, replace := applyStringPlan(t, mods, types.StringValue("before"), types.StringValue("after"), types.StringValue("after"))
	if !replace || renamed.ValueString() != "after" {
		t.Fatalf("renamed plan = %s replace=%v", renamed, replace)
	}

	typeMods := resp.Schema.Attributes["type"].(schema.StringAttribute).PlanModifiers
	keptType, replace := applyStringPlan(t, typeMods, types.StringValue("full"), types.StringUnknown(), types.StringNull())
	if replace || keptType.ValueString() != "full" {
		t.Fatalf("assigned type plan = %s replace=%v", keptType, replace)
	}
	changedType, replace := applyStringPlan(t, typeMods, types.StringValue("full"), types.StringValue("partial_include"), types.StringValue("partial_include"))
	if !replace || changedType.ValueString() != "partial_include" {
		t.Fatalf("changed type plan = %s replace=%v", changedType, replace)
	}

	createdMods := resp.Schema.Attributes["created"].(schema.Int64Attribute).PlanModifiers
	keptCreated := applyInt64Plan(t, createdMods, types.Int64Value(1700000000), types.Int64Unknown())
	if keptCreated.ValueInt64() != 1700000000 {
		t.Fatalf("created plan = %s", keptCreated)
	}
}

func TestTenantSnapshotTypeRejectsUILabels(t *testing.T) {
	resp := &resource.SchemaResponse{}
	NewTenantSnapshotResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	attr := resp.Schema.Attributes["type"].(schema.StringAttribute)
	var v validator.String
	for _, candidate := range attr.Validators {
		v = candidate
	}
	if v == nil {
		t.Fatal("type should have a validator")
	}
	for _, badValue := range []string{"Provider", "Local", "incremental"} {
		bad := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("type"),
			ConfigValue: types.StringValue(badValue),
		}, bad)
		if !bad.Diagnostics.HasError() {
			t.Fatalf("type %q was accepted", badValue)
		}
	}
	for _, goodValue := range []string{"full", "partial_include", "partial_exclude"} {
		good := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("type"),
			ConfigValue: types.StringValue(goodValue),
		}, good)
		if good.Diagnostics.HasError() {
			t.Fatalf("%s: %v", goodValue, good.Diagnostics)
		}
	}
}

func TestTenantSnapshotValidateConfig(t *testing.T) {
	withExpires := newSnapshotModel()
	withExpires.NeverExpires = types.BoolNull()
	if diags := validateSnapshotConfig(t, withExpires); diags.HasError() {
		t.Fatal(diags)
	}
	never := newSnapshotModel()
	never.Expires = types.Int64Null()
	never.NeverExpires = types.BoolValue(true)
	if diags := validateSnapshotConfig(t, never); diags.HasError() {
		t.Fatal(diags)
	}
	unknown := newSnapshotModel()
	unknown.Expires = types.Int64Unknown()
	unknown.NeverExpires = types.BoolNull()
	if diags := validateSnapshotConfig(t, unknown); diags.HasError() {
		t.Fatal(diags)
	}
	both := newSnapshotModel()
	both.NeverExpires = types.BoolValue(true)
	if diags := validateSnapshotConfig(t, both); !diags.HasError() {
		t.Fatal("expires and never_expires were both accepted")
	}
	neither := newSnapshotModel()
	neither.Expires = types.Int64Null()
	neither.NeverExpires = types.BoolNull()
	if diags := validateSnapshotConfig(t, neither); !diags.HasError() {
		t.Fatal("missing expiration was accepted")
	}
	blankName := newSnapshotModel()
	blankName.Name = types.StringValue("  ")
	blankName.NeverExpires = types.BoolNull()
	if diags := validateSnapshotConfig(t, blankName); !diags.HasError() {
		t.Fatal("blank name was accepted")
	}
	badTenant := newSnapshotModel()
	badTenant.TenantID = types.StringValue("0")
	badTenant.NeverExpires = types.BoolNull()
	if diags := validateSnapshotConfig(t, badTenant); !diags.HasError() {
		t.Fatal("tenant_id 0 was accepted")
	}
}

func TestTenantSnapshotConfigure(t *testing.T) {
	r := &TenantSnapshotResource{}
	r.Configure(context.Background(), resource.ConfigureRequest{}, &resource.ConfigureResponse{})
	if r.api != nil {
		t.Fatal("nil provider data should skip configure")
	}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("invalid provider data should error")
	}
	ok := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: tenantTestClient(t)}, ok)
	if ok.Diagnostics.HasError() {
		t.Fatal(ok.Diagnostics)
	}
	if r.api == nil {
		t.Fatal("api was not configured")
	}
}

func TestTenantSnapshotCreateRequest(t *testing.T) {
	req, err := tenantSnapshotCreateRequest(newSnapshotModel())
	if err != nil {
		t.Fatal(err)
	}
	if req.Tenant != 7 || req.Name != "before-change" || req.Description != "safety" || req.Type != "full" {
		t.Fatalf("request = %#v", req)
	}
	if req.Expires == nil || *req.Expires != 1893456000 {
		t.Fatalf("expires = %v", req.Expires)
	}

	never := newSnapshotModel()
	never.Name = types.StringNull()
	never.Description = types.StringNull()
	never.Type = types.StringNull()
	never.Expires = types.Int64Null()
	never.NeverExpires = types.BoolValue(true)
	req, err = tenantSnapshotCreateRequest(never)
	if err != nil {
		t.Fatal(err)
	}
	if req.Name != "" || req.Description != "" || req.Type != "" {
		t.Fatalf("omitted fields were sent: %#v", req)
	}
	if req.Expires == nil || *req.Expires != 0 {
		t.Fatalf("never expires = %v", req.Expires)
	}

	missing := newSnapshotModel()
	missing.Expires = types.Int64Null()
	missing.NeverExpires = types.BoolNull()
	if _, err := tenantSnapshotCreateRequest(missing); err == nil {
		t.Fatal("missing expiration was accepted")
	}
}

func TestTenantSnapshotExpirationPlan(t *testing.T) {
	state := newSnapshotModel()
	plan := newSnapshotModel()
	plan.Expires = types.Int64Value(1924992000)
	got := tenantSnapshotExpirationPlan(plan, state)
	if got.setNever || got.expires == nil || *got.expires != 1924992000 {
		t.Fatalf("expires plan = %#v", got)
	}

	same := tenantSnapshotExpirationPlan(state, state)
	if same.setNever || same.expires != nil {
		t.Fatalf("unchanged plan = %#v", same)
	}

	toNever := newSnapshotModel()
	toNever.Expires = types.Int64Null()
	toNever.NeverExpires = types.BoolValue(true)
	got = tenantSnapshotExpirationPlan(toNever, state)
	if !got.setNever || got.expires != nil {
		t.Fatalf("never plan = %#v", got)
	}

	already := newSnapshotModel()
	already.Expires = types.Int64Null()
	already.NeverExpires = types.BoolValue(true)
	got = tenantSnapshotExpirationPlan(already, already)
	if got.setNever || got.expires != nil {
		t.Fatalf("already never plan = %#v", got)
	}

	back := newSnapshotModel()
	got = tenantSnapshotExpirationPlan(back, already)
	if got.setNever || got.expires == nil || *got.expires != 1893456000 {
		t.Fatalf("restore expires plan = %#v", got)
	}
}

func TestAssignTenantSnapshot(t *testing.T) {
	data := &TenantSnapshotResourceModel{}
	assignTenantSnapshot(data, &vergeos.TenantSnapshot{
		Key:         4,
		Tenant:      7,
		Name:        "before-change",
		Description: "safety",
		Type:        vergeos.TenantSnapshotTypePartialInclude,
		Created:     1700000000,
		Expires:     1893456000,
	})
	if data.Id.ValueString() != "4" || data.TenantID.ValueString() != "7" || data.Name.ValueString() != "before-change" {
		t.Fatalf("identity = %#v", data)
	}
	if data.Description.ValueString() != "safety" || data.Type.ValueString() != "partial_include" {
		t.Fatalf("text = %#v", data)
	}
	if data.Created.ValueInt64() != 1700000000 || data.Expires.ValueInt64() != 1893456000 || data.NeverExpires.ValueBool() {
		t.Fatalf("times = created %s expires %s never %s", data.Created, data.Expires, data.NeverExpires)
	}

	assignTenantSnapshot(data, &vergeos.TenantSnapshot{Key: 4, Tenant: 7, Name: "before-change", Expires: 0})
	if !data.Expires.IsNull() || !data.NeverExpires.ValueBool() {
		t.Fatalf("never = expires %s never %s", data.Expires, data.NeverExpires)
	}
	if data.Type.ValueString() != "full" || !data.Created.IsNull() {
		t.Fatalf("empty type/created = type %s created %s", data.Type, data.Created)
	}
}

func TestTenantSnapshotListModel(t *testing.T) {
	model := tenantSnapshotListModel(&vergeos.TenantSnapshot{
		Key: 4, Name: "before-change", Type: "full", Created: 1700000000, Expires: 0,
	})
	if model.Id.ValueString() != "4" || model.Name.ValueString() != "before-change" || model.Type.ValueString() != "full" {
		t.Fatalf("model = %#v", model)
	}
	if model.Created.ValueInt64() != 1700000000 || !model.Expires.IsNull() {
		t.Fatalf("times = created %s expires %s", model.Created, model.Expires)
	}
	blank := tenantSnapshotListModel(&vergeos.TenantSnapshot{Key: 5})
	if !blank.Type.IsNull() || !blank.Created.IsNull() || !blank.Expires.IsNull() {
		t.Fatalf("blank model = %#v", blank)
	}
}

func TestCreateTenantSnapshot(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := newSnapshotModel()
	if err := api.createTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() == "" || data.Created.ValueInt64() != 1700000000 || data.NeverExpires.ValueBool() {
		t.Fatalf("state = %#v", data)
	}
	creates := fake.bodiesFor(http.MethodPost, "/api/v4/tenant_snapshots")
	if len(creates) != 1 {
		t.Fatalf("creates = %#v", creates)
	}
	body := creates[0]
	if intField(body["tenant"]) != 7 || body["name"] != "before-change" || body["description"] != "safety" || body["type"] != "full" {
		t.Fatalf("create body = %#v", body)
	}
	if int64Field(body["expires"]) != 1893456000 {
		t.Fatalf("expires = %#v", body["expires"])
	}
}

func TestCreateTenantSnapshotAssignsNameAndType(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := newSnapshotModel()
	data.Name = types.StringNull()
	data.Type = types.StringNull()
	data.Description = types.StringNull()
	if err := api.createTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Name.ValueString() == "" || data.Type.ValueString() != "full" {
		t.Fatalf("assigned = name %s type %s", data.Name, data.Type)
	}
	body := fake.bodiesFor(http.MethodPost, "/api/v4/tenant_snapshots")[0]
	if _, ok := body["name"]; ok {
		t.Fatalf("create sent a name: %#v", body)
	}
	if _, ok := body["type"]; ok {
		t.Fatalf("create sent a type: %#v", body)
	}
}

func TestUpdateTenantSnapshotDescriptionAndExpiry(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	state := newSnapshotModel()
	if err := api.createTenantSnapshot(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantSnapshot(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	path := "/api/v4/tenant_snapshots/" + state.Id.ValueString()

	desc := *state
	desc.Description = types.StringValue("updated")
	if err := api.updateTenantSnapshot(context.Background(), &desc, state); err != nil {
		t.Fatal(err)
	}
	puts := fake.bodiesFor(http.MethodPut, path)
	if len(puts) != 1 || puts[0]["description"] != "updated" {
		t.Fatalf("description puts = %#v", puts)
	}
	if _, ok := puts[0]["expires"]; ok {
		t.Fatal("description update sent expires")
	}
	if _, ok := puts[0]["name"]; ok || puts[0]["type"] != nil {
		t.Fatalf("description update changed a readonly field: %#v", puts[0])
	}

	state = &desc
	later := *state
	later.Expires = types.Int64Value(1924992000)
	if err := api.updateTenantSnapshot(context.Background(), &later, state); err != nil {
		t.Fatal(err)
	}
	puts = fake.bodiesFor(http.MethodPut, path)
	if len(puts) != 2 || int64Field(puts[1]["expires"]) != 1924992000 {
		t.Fatalf("expiry puts = %#v", puts)
	}
	if _, ok := puts[1]["description"]; ok {
		t.Fatal("SetExpires sent a description")
	}

	state = &later
	keep := *state
	keep.Description = types.StringValue("kept")
	keep.Expires = types.Int64Null()
	keep.NeverExpires = types.BoolValue(true)
	if err := api.updateTenantSnapshot(context.Background(), &keep, state); err != nil {
		t.Fatal(err)
	}
	puts = fake.bodiesFor(http.MethodPut, path)
	if len(puts) != 4 {
		t.Fatalf("combined puts = %#v", puts)
	}
	if puts[2]["description"] != "kept" {
		t.Fatalf("description put = %#v", puts[2])
	}
	if _, ok := puts[2]["expires"]; ok {
		t.Fatal("description put included expires")
	}
	if _, ok := puts[3]["expires"]; !ok || int64Field(puts[3]["expires"]) != 0 {
		t.Fatalf("never expires put = %#v", puts[3])
	}
	if _, ok := puts[3]["description"]; ok {
		t.Fatal("SetNeverExpires sent a description")
	}
	if err := api.readTenantSnapshot(context.Background(), &keep); err != nil {
		t.Fatal(err)
	}
	if keep.Description.ValueString() != "kept" || !keep.Expires.IsNull() || !keep.NeverExpires.ValueBool() {
		t.Fatalf("read back = %#v", keep)
	}
}

func TestReadTenantSnapshotRejectsReusedKey(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := newSnapshotModel()
	if err := api.createTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	original := *data
	data.TenantID = types.StringValue("8")
	if err := api.readTenantSnapshot(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused tenant err = %v", err)
	}
	*data = original
	data.Name = types.StringValue("other")
	if err := api.readTenantSnapshot(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused name err = %v", err)
	}
}

func TestDeleteTenantSnapshot(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := newSnapshotModel()
	if err := api.createTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.deleteTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantSnapshot(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("read after delete = %v", err)
	}
	if err := api.deleteTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
}

func TestTenantSnapshotImport(t *testing.T) {
	for _, id := range []string{"0", "abc", "7/", "/name", "-3"} {
		resp := importResourceState(t, NewTenantSnapshotResource(), id)
		assertEmptyImportID(t, resp, "Invalid Tenant Snapshot Import ID", tenantSnapshotImportDetail)
	}
	resp := importResourceState(t, NewTenantSnapshotResource(), "15")
	assertImportIDPassthrough(t, resp, "15")

	missing := importResourceState(t, NewTenantSnapshotResource(), "7/before-change")
	if !missing.Diagnostics.HasError() {
		t.Fatal("name import without a client should fail")
	}

	fake := newFake(t)
	api := fake.api(t)
	data := newSnapshotModel()
	if err := api.createTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantSnapshot(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	imported := importResourceState(t, &TenantSnapshotResource{api: api}, data.TenantID.ValueString()+"/"+data.Name.ValueString())
	assertImportIDPassthrough(t, imported, data.Id.ValueString())

	refreshed := &TenantSnapshotResourceModel{Id: data.Id}
	if err := api.readTenantSnapshot(context.Background(), refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.TenantID.ValueString() != "7" || refreshed.Name.ValueString() != "before-change" || refreshed.Expires.ValueInt64() != 1893456000 {
		t.Fatalf("import read = %#v", refreshed)
	}
}

func TestTenantSnapshotsDataSource(t *testing.T) {
	item := NewTenantSnapshotsDataSource()
	meta := &datasource.MetadataResponse{}
	item.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_snapshots" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &datasource.SchemaResponse{}
	item.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	tenantID := resp.Schema.Attributes["tenant_id"]
	if tenantID == nil || !tenantID.IsRequired() {
		t.Fatal("tenant_id should be required")
	}
	list, ok := resp.Schema.Attributes["snapshots"].(dschema.ListNestedAttribute)
	if !ok || !list.IsComputed() {
		t.Fatal("snapshots should be a computed list")
	}
	for _, name := range []string{"id", "name", "type", "created", "expires"} {
		attr := list.NestedObject.Attributes[name]
		if attr == nil || !attr.IsComputed() {
			t.Fatalf("snapshots.%s should be computed", name)
		}
	}
	if resp.Schema.MarkdownDescription == "" || !containsAll(t, resp.Schema.MarkdownDescription, "Provider", "Local", "full") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}

	ds := &TenantSnapshotsDataSource{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{}, &datasource.ConfigureResponse{})
	if ds.api != nil {
		t.Fatal("nil provider data should skip configure")
	}
	bad := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "nope"}, bad)
	if !bad.Diagnostics.HasError() {
		t.Fatal("invalid provider data should error")
	}

	fake := newFake(t)
	api := fake.api(t)
	first := newSnapshotModel()
	first.Name = types.StringValue("b-snap")
	second := newSnapshotModel()
	second.Name = types.StringValue("a-snap")
	second.Expires = types.Int64Null()
	second.NeverExpires = types.BoolValue(true)
	other := newSnapshotModel()
	other.TenantID = types.StringValue("8")
	other.Name = types.StringValue("elsewhere")
	for _, model := range []*TenantSnapshotResourceModel{first, second, other} {
		if err := api.createTenantSnapshot(context.Background(), model); err != nil {
			t.Fatal(err)
		}
	}
	listed := &TenantSnapshotsDataSourceModel{TenantID: types.StringValue("7")}
	if err := api.readTenantSnapshots(context.Background(), listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Snapshots) != 2 {
		t.Fatalf("snapshots = %#v", listed.Snapshots)
	}
	if listed.Snapshots[0].Name.ValueString() != "a-snap" || !listed.Snapshots[0].Expires.IsNull() {
		t.Fatalf("first = %#v", listed.Snapshots[0])
	}
	if listed.Snapshots[1].Name.ValueString() != "b-snap" || listed.Snapshots[1].Type.ValueString() != "full" || listed.Snapshots[1].Created.ValueInt64() != 1700000000 {
		t.Fatalf("second = %#v", listed.Snapshots[1])
	}
	if listed.Snapshots[1].Expires.ValueInt64() != 1893456000 {
		t.Fatalf("expires = %s", listed.Snapshots[1].Expires)
	}
}

func newSnapshotModel() *TenantSnapshotResourceModel {
	return &TenantSnapshotResourceModel{
		TenantID:     types.StringValue("7"),
		Name:         types.StringValue("before-change"),
		Description:  types.StringValue("safety"),
		Type:         types.StringValue(vergeos.TenantSnapshotTypeFull),
		Expires:      types.Int64Value(1893456000),
		NeverExpires: types.BoolValue(false),
	}
}

func validateSnapshotConfig(t *testing.T, model *TenantSnapshotResourceModel) diag.Diagnostics {
	t.Helper()
	item := NewTenantSnapshotResource().(resource.ResourceWithValidateConfig)
	schemaResp := &resource.SchemaResponse{}
	NewTenantSnapshotResource().Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &resource.ValidateConfigResponse{}
	item.ValidateConfig(context.Background(), resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw},
	}, resp)
	return resp.Diagnostics
}

func applyInt64Plan(t *testing.T, mods []planmodifier.Int64, state, plan types.Int64) types.Int64 {
	t.Helper()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	resp := &planmodifier.Int64Response{PlanValue: plan}
	for _, mod := range mods {
		mod.PlanModifyInt64(context.Background(), planmodifier.Int64Request{
			StateValue: state,
			PlanValue:  resp.PlanValue,
			State:      tfsdk.State{Raw: raw},
			Plan:       tfsdk.Plan{Raw: raw},
		}, resp)
	}
	return resp.PlanValue
}

func applyStringPlan(t *testing.T, mods []planmodifier.String, state, plan, config types.String) (types.String, bool) {
	t.Helper()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	resp := &planmodifier.StringResponse{PlanValue: plan}
	for _, mod := range mods {
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			StateValue:  state,
			PlanValue:   resp.PlanValue,
			ConfigValue: config,
			State:       tfsdk.State{Raw: raw},
			Plan:        tfsdk.Plan{Raw: raw},
		}, resp)
	}
	return resp.PlanValue, resp.RequiresReplace
}
