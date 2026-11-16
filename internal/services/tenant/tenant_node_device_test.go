// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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

const testDeviceGroup = "24122fab-e8fa-c4a5-dcf5-b52bf1efa474"

func TestTenantNodeDeviceSchema(t *testing.T) {
	r := NewTenantNodeDeviceResource()
	meta := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_node_device" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	if _, ok := resp.Schema.Attributes["count"]; ok {
		t.Fatal("count is reserved by Terraform; the argument is device_count")
	}
	for _, name := range []string{"tenant_node_id", "type"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok || !attr.IsRequired() || !stringRequiresReplace(t, attr.PlanModifiers) {
			t.Fatalf("%s should be required and replace the device", name)
		}
	}
	name := resp.Schema.Attributes["name"].(schema.StringAttribute)
	if !name.IsRequired() || stringRequiresReplace(t, name.PlanModifiers) {
		t.Fatal("name should be required and update in place")
	}
	count := resp.Schema.Attributes["device_count"].(schema.Int32Attribute)
	if !count.IsOptional() || !count.IsComputed() || count.Default == nil || !int32RequiresReplace(t, count.PlanModifiers) {
		t.Fatal("device_count should be optional, defaulted, and replace the device")
	}
	for _, attrName := range []string{"description", "resource_group"} {
		attr := resp.Schema.Attributes[attrName].(schema.StringAttribute)
		if !attr.IsOptional() || !attr.IsComputed() || stringRequiresReplace(t, attr.PlanModifiers) {
			t.Fatalf("%s should update in place", attrName)
		}
	}
	enabled := resp.Schema.Attributes["enabled"].(schema.BoolAttribute)
	optional := resp.Schema.Attributes["optional"].(schema.BoolAttribute)
	if enabled.Default == nil || optional.Default == nil {
		t.Fatal("enabled and optional should have defaults")
	}
	if resp.Schema.MarkdownDescription == "" || !containsAll(t, resp.Schema.MarkdownDescription, "thick", "owns", "device_count") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestTenantNodeDeviceTypeValidator(t *testing.T) {
	resp := &resource.SchemaResponse{}
	NewTenantNodeDeviceResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	attr := resp.Schema.Attributes["type"].(schema.StringAttribute)
	var check validator.String
	for _, candidate := range attr.Validators {
		check = candidate
	}
	if check == nil {
		t.Fatal("type should have a validator")
	}
	for _, badValue := range []string{"disk", "nic", "host_gpu"} {
		bad := &validator.StringResponse{}
		check.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("type"),
			ConfigValue: types.StringValue(badValue),
		}, bad)
		if !bad.Diagnostics.HasError() {
			t.Fatalf("type %q was accepted", badValue)
		}
	}
	for _, goodValue := range tenantNodeDeviceTypes() {
		good := &validator.StringResponse{}
		check.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("type"),
			ConfigValue: types.StringValue(goodValue),
		}, good)
		if good.Diagnostics.HasError() {
			t.Fatalf("%s: %v", goodValue, good.Diagnostics)
		}
	}
}

func TestTenantNodeDeviceConfigure(t *testing.T) {
	r := &TenantNodeDeviceResource{}
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

func TestTenantNodeDeviceCreateRequestUsesSDKFields(t *testing.T) {
	count := int32(2)
	data := deviceModel()
	data.Count = types.Int32Value(count)
	req, err := tenantNodeDeviceCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Name != "pci0" || req.Type != vergeos.DeviceTypePCI {
		t.Fatalf("request = %+v", req)
	}
	if req.ResourceGroup != vergeos.DeviceResourceGroup(testDeviceGroup) {
		t.Fatalf("resource group = %q", req.ResourceGroup)
	}
	if req.Count == nil || *req.Count != 2 || req.Enabled == nil || !*req.Enabled || req.Optional == nil || *req.Optional {
		t.Fatalf("flags = %+v", req)
	}
	raw := marshalDeviceJSON(t, req)
	if _, ok := raw["device_count"]; ok {
		t.Fatal("device_count was posted; the API field is count")
	}
	if _, ok := raw["resourceGroup"]; ok {
		t.Fatal("resourceGroup is not a VergeOS field")
	}
	if string(raw["resource_group"]) != `"`+testDeviceGroup+`"` {
		t.Fatalf("resource_group JSON = %s", raw["resource_group"])
	}
	if string(raw["count"]) != "2" || string(raw["optional"]) != "false" || string(raw["enabled"]) != "true" {
		t.Fatalf("json = %s", raw)
	}
	if _, ok := raw["settings_args"]; ok {
		t.Fatal("settings_args was posted")
	}

	group := vergeos.DeviceResourceGroup(testDeviceGroup)
	updated := marshalDeviceJSON(t, &vergeos.VMDeviceUpdateRequest{ResourceGroup: &group})
	if string(updated["resource_group"]) != `"`+testDeviceGroup+`"` {
		t.Fatalf("update resource_group JSON = %s", updated["resource_group"])
	}
}

func TestTenantNodeDeviceCreateRequestRejectsBadInput(t *testing.T) {
	blank := deviceModel()
	blank.Name = types.StringValue("  ")
	if _, err := tenantNodeDeviceCreateRequest(blank); err == nil {
		t.Fatal("blank name was accepted")
	}
	missingType := deviceModel()
	missingType.Type = types.StringNull()
	if _, err := tenantNodeDeviceCreateRequest(missingType); err == nil {
		t.Fatal("missing type was accepted")
	}
	tooMany := deviceModel()
	tooMany.Count = types.Int32Value(17)
	if _, err := tenantNodeDeviceCreateRequest(tooMany); err == nil {
		t.Fatal("device_count 17 was accepted")
	}
	omitted := deviceModel()
	omitted.ResourceGroup = types.StringNull()
	req, err := tenantNodeDeviceCreateRequest(omitted)
	if err != nil {
		t.Fatal(err)
	}
	if req.ResourceGroup != "" {
		t.Fatalf("empty resource group = %q", req.ResourceGroup)
	}
	if _, ok := marshalDeviceJSON(t, req)["resource_group"]; ok {
		t.Fatal("empty resource_group was posted")
	}
}

func TestAssignTenantNodeDevice(t *testing.T) {
	kept := deviceModel()
	kept.Count = types.Int32Value(2)
	assignTenantNodeDevice(kept, &vergeos.VMDevice{
		Key:         vergeos.FlexInt(7),
		Machine:     80,
		MachineType: vergeos.DeviceMachineTypeTenant,
		Name:        "pci0",
		Type:        vergeos.DeviceTypePCI,
		Enabled:     true,
		Created:     1700000000,
	})
	if kept.Id.ValueString() != "7" || kept.Machine.ValueInt32() != 80 || kept.MachineType.ValueString() != "tenant" {
		t.Fatalf("assigned = %#v", kept)
	}
	if kept.ResourceGroup.ValueString() != testDeviceGroup {
		t.Fatalf("blank API resource group replaced %s", kept.ResourceGroup)
	}
	if kept.Count.ValueInt32() != 2 {
		t.Fatalf("count = %d", kept.Count.ValueInt32())
	}
	if kept.Created.ValueInt64() != 1700000000 || !kept.Modified.IsNull() {
		t.Fatalf("timestamps = %s %s", kept.Created, kept.Modified)
	}

	replaced := deviceModel()
	assignTenantNodeDevice(replaced, &vergeos.VMDevice{
		Key:           vergeos.FlexInt(8),
		ResourceGroup: vergeos.DeviceResourceGroup("bbbbbbbb-e8fa-c4a5-dcf5-b52bf1efa474"),
		Name:          "pci1",
		Type:          vergeos.DeviceTypePCI,
	})
	if replaced.ResourceGroup.ValueString() != "bbbbbbbb-e8fa-c4a5-dcf5-b52bf1efa474" {
		t.Fatalf("resource group = %s", replaced.ResourceGroup)
	}
	if replaced.Count.ValueInt32() != 1 {
		t.Fatalf("missing count = %s", replaced.Count)
	}
}

func TestCreateTenantNodeDevicePostsMachineKey(t *testing.T) {
	fake := newFake(t)
	fake.seedTenantNode(11, 7, 80)
	api := fake.api(t)
	data := deviceModel()
	data.TenantNodeID = types.StringValue("11")
	if err := api.createTenantNodeDevice(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() == "" || data.Machine.ValueInt32() != 80 || data.MachineType.ValueString() != "tenant" {
		t.Fatalf("created = %#v", data)
	}
	if data.ResourceGroup.ValueString() != testDeviceGroup || data.Count.ValueInt32() != 1 {
		t.Fatalf("group/count = %s %s", data.ResourceGroup, data.Count)
	}
	creates := fake.bodiesFor(http.MethodPost, "/api/v4/machine_devices")
	if len(creates) != 1 {
		t.Fatalf("create bodies = %#v", creates)
	}
	body := creates[0]
	if intField(body["machine"]) != 80 {
		t.Fatalf("posted machine %v, want the node machine 80 (node key is 11)", body["machine"])
	}
	if body["type"] != vergeos.DeviceTypePCI || body["name"] != "pci0" {
		t.Fatalf("create body = %#v", body)
	}
	if body["resource_group"] != testDeviceGroup {
		t.Fatalf("resource_group = %#v", body["resource_group"])
	}
	if intField(body["count"]) != 1 || body["enabled"] != true || body["optional"] != false {
		t.Fatalf("flags = %#v", body)
	}
	if _, ok := body["device_count"]; ok {
		t.Fatal("device_count was posted")
	}
	if err := api.readTenantNodeDevice(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.TenantNodeID.ValueString() != "11" || data.Enabled.ValueBool() != true {
		t.Fatalf("read = %#v", data)
	}
}

func TestUpdateTenantNodeDevice(t *testing.T) {
	fake := newFake(t)
	fake.seedTenantNode(11, 7, 80)
	api := fake.api(t)
	state := deviceModel()
	state.TenantNodeID = types.StringValue("11")
	if err := api.createTenantNodeDevice(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	plan := *state
	plan.Name = types.StringValue("pci-renamed")
	plan.Description = types.StringValue("passed through")
	plan.Enabled = types.BoolValue(false)
	plan.Optional = types.BoolValue(true)
	plan.ResourceGroup = types.StringValue("bbbbbbbb-e8fa-c4a5-dcf5-b52bf1efa474")
	if err := api.updateTenantNodeDevice(context.Background(), &plan, state); err != nil {
		t.Fatal(err)
	}
	updates := fake.bodiesFor(http.MethodPut, "/api/v4/machine_devices/"+state.Id.ValueString())
	if len(updates) != 1 {
		t.Fatalf("update bodies = %#v", updates)
	}
	body := updates[0]
	if body["name"] != "pci-renamed" || body["description"] != "passed through" || body["enabled"] != false || body["optional"] != true {
		t.Fatalf("update body = %#v", body)
	}
	if body["resource_group"] != "bbbbbbbb-e8fa-c4a5-dcf5-b52bf1efa474" {
		t.Fatalf("resource_group = %#v", body["resource_group"])
	}
	if _, ok := body["count"]; ok {
		t.Fatal("count was sent on update")
	}
	if _, ok := body["type"]; ok || body["machine"] != nil {
		t.Fatalf("update changed type or machine: %#v", body)
	}
	if plan.Name.ValueString() != "pci-renamed" || plan.Optional.ValueBool() != true || plan.Count.ValueInt32() != 1 {
		t.Fatalf("plan = %#v", plan)
	}
	same := plan
	if err := api.updateTenantNodeDevice(context.Background(), &same, &plan); err != nil {
		t.Fatal(err)
	}
	if again := fake.bodiesFor(http.MethodPut, "/api/v4/machine_devices/"+state.Id.ValueString()); len(again) != 1 {
		t.Fatalf("unchanged update posted again: %#v", again)
	}
}

func TestReadTenantNodeDeviceRejectsForeignRow(t *testing.T) {
	fake := newFake(t)
	fake.seedTenantNode(11, 7, 80)
	api := fake.api(t)
	fake.seedDevice(9, map[string]any{
		"machine":      22,
		"machine_type": "vm",
		"name":         "vm-pci",
		"type":         vergeos.DeviceTypePCI,
		"enabled":      true,
	})
	vmDevice := &TenantNodeDeviceResourceModel{Id: types.StringValue("9"), TenantNodeID: types.StringValue("11")}
	if err := api.readTenantNodeDevice(context.Background(), vmDevice); err == nil || !vergeos.IsNotFoundError(err) {
		t.Fatalf("vm device err = %v", err)
	}

	fake.seedDevice(8, map[string]any{
		"machine":      81,
		"machine_type": vergeos.DeviceMachineTypeTenant,
		"name":         "other",
		"type":         vergeos.DeviceTypePCI,
		"enabled":      true,
	})
	other := &TenantNodeDeviceResourceModel{Id: types.StringValue("8"), TenantNodeID: types.StringValue("11")}
	if err := api.readTenantNodeDevice(context.Background(), other); err == nil || !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused key err = %v", err)
	}
}

func TestReadTenantNodeDeviceFindsNodeOnImport(t *testing.T) {
	fake := newFake(t)
	fake.seedTenantNode(11, 7, 80)
	api := fake.api(t)
	created := deviceModel()
	created.TenantNodeID = types.StringValue("11")
	if err := api.createTenantNodeDevice(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	imported := &TenantNodeDeviceResourceModel{Id: created.Id}
	if err := api.readTenantNodeDevice(context.Background(), imported); err != nil {
		t.Fatal(err)
	}
	if imported.TenantNodeID.ValueString() != "11" || imported.Count.ValueInt32() != 1 {
		t.Fatalf("imported = %#v", imported)
	}
	if imported.ResourceGroup.ValueString() != testDeviceGroup {
		t.Fatalf("resource group = %s", imported.ResourceGroup)
	}
}

func TestDeleteTenantNodeDevice(t *testing.T) {
	fake := newFake(t)
	fake.seedTenantNode(11, 7, 80)
	api := fake.api(t)
	data := deviceModel()
	data.TenantNodeID = types.StringValue("11")
	if err := api.createTenantNodeDevice(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.deleteTenantNodeDevice(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantNodeDevice(context.Background(), data); err == nil || !vergeos.IsNotFoundError(err) {
		t.Fatalf("after delete err = %v", err)
	}
	if err := api.deleteTenantNodeDevice(context.Background(), data); err != nil {
		t.Fatal(err)
	}
}

func TestTenantNodeDevicesDataSource(t *testing.T) {
	item := NewTenantNodeDevicesDataSource()
	meta := &datasource.MetadataResponse{}
	item.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_node_devices" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &datasource.SchemaResponse{}
	item.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	tenantNodeID := resp.Schema.Attributes["tenant_node_id"]
	if tenantNodeID == nil || !tenantNodeID.IsRequired() {
		t.Fatal("tenant_node_id should be required")
	}
	list, ok := resp.Schema.Attributes["devices"].(dschema.ListNestedAttribute)
	if !ok || !list.IsComputed() {
		t.Fatal("devices should be a computed list")
	}
	for _, name := range []string{"id", "name", "type", "resource_group", "enabled", "optional", "machine", "machine_type"} {
		attr := list.NestedObject.Attributes[name]
		if attr == nil || !attr.IsComputed() {
			t.Fatalf("devices.%s should be computed", name)
		}
	}
	if _, ok := list.NestedObject.Attributes["device_count"]; ok {
		t.Fatal("device_count is not stored on the device row")
	}
	if resp.Schema.MarkdownDescription == "" || !containsAll(t, resp.Schema.MarkdownDescription, "thick", "owns") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}

	ds := &TenantNodeDevicesDataSource{}
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
	fake.seedTenantNode(11, 7, 80)
	fake.seedTenantNode(12, 7, 90)
	api := fake.api(t)
	first := deviceModel()
	first.TenantNodeID = types.StringValue("11")
	first.Name = types.StringValue("b-pci")
	second := deviceModel()
	second.TenantNodeID = types.StringValue("11")
	second.Name = types.StringValue("a-pci")
	second.ResourceGroup = types.StringNull()
	other := deviceModel()
	other.TenantNodeID = types.StringValue("12")
	other.Name = types.StringValue("elsewhere")
	for _, model := range []*TenantNodeDeviceResourceModel{first, second, other} {
		if err := api.createTenantNodeDevice(context.Background(), model); err != nil {
			t.Fatal(err)
		}
	}
	listed := &TenantNodeDevicesDataSourceModel{TenantNodeID: types.StringValue("11")}
	if err := api.readTenantNodeDevices(context.Background(), listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Devices) != 2 {
		t.Fatalf("devices = %#v", listed.Devices)
	}
	if listed.Devices[0].Name.ValueString() != "a-pci" || !listed.Devices[0].ResourceGroup.IsNull() {
		t.Fatalf("first = %#v", listed.Devices[0])
	}
	if listed.Devices[1].Name.ValueString() != "b-pci" || listed.Devices[1].ResourceGroup.ValueString() != testDeviceGroup {
		t.Fatalf("second = %#v", listed.Devices[1])
	}
	if listed.Devices[1].Machine.ValueInt32() != 80 || listed.Devices[1].MachineType.ValueString() != "tenant" {
		t.Fatalf("machine = %s %s", listed.Devices[1].Machine, listed.Devices[1].MachineType)
	}
	empty := &TenantNodeDevicesDataSourceModel{TenantNodeID: types.StringValue("12")}
	// The other node has one device. Read a node that has none by using a fresh node.
	fake.seedTenantNode(13, 7, 100)
	empty.TenantNodeID = types.StringValue("13")
	if err := api.readTenantNodeDevices(context.Background(), empty); err != nil {
		t.Fatal(err)
	}
	if empty.Devices == nil || len(empty.Devices) != 0 {
		t.Fatalf("empty list = %#v", empty.Devices)
	}
}

func deviceModel() *TenantNodeDeviceResourceModel {
	return &TenantNodeDeviceResourceModel{
		TenantNodeID:  types.StringValue("11"),
		Name:          types.StringValue("pci0"),
		Description:   types.StringValue("passthrough"),
		Type:          types.StringValue(vergeos.DeviceTypePCI),
		ResourceGroup: types.StringValue(testDeviceGroup),
		Enabled:       types.BoolValue(true),
		Optional:      types.BoolValue(false),
		Count:         types.Int32Value(1),
	}
}

func marshalDeviceJSON(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func int32RequiresReplace(t *testing.T, mods []planmodifier.Int32) bool {
	t.Helper()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	for _, mod := range mods {
		resp := &planmodifier.Int32Response{PlanValue: types.Int32Value(2)}
		mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
			StateValue:  types.Int32Value(1),
			PlanValue:   types.Int32Value(2),
			ConfigValue: types.Int32Value(2),
			State:       tfsdk.State{Raw: raw},
			Plan:        tfsdk.Plan{Raw: raw},
		}, resp)
		if resp.RequiresReplace {
			return true
		}
	}
	return false
}
