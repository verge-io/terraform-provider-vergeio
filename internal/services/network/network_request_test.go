package network

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNetworkUpdateRequestSendsExplicitFalse(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:      types.StringValue("12"),
		Name:    types.StringValue("tf-acc-net"),
		Enabled: types.BoolValue(false),
		Type:    types.StringValue("internal"),
	}
	state := &NetworkResourceModel{
		Id:      types.StringValue("12"),
		Name:    types.StringValue("tf-acc-net"),
		Enabled: types.BoolValue(true),
		Type:    types.StringValue("internal"),
	}

	req, id, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if id != 12 {
		t.Fatalf("network id = %d, want 12", id)
	}
	if req.Enabled == nil || *req.Enabled {
		t.Fatalf("enabled pointer = %v, want false", req.Enabled)
	}

	body := marshalRequest(t, req)
	if !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("update body dropped enabled=false: %s", body)
	}
}

func TestNetworkCreateRequestOmitsInterfaceVnetZero(t *testing.T) {
	data := &NetworkResourceModel{
		Name:           types.StringValue("tf-acc-net"),
		Type:           types.StringValue("internal"),
		Interface_Vnet: types.Int32Value(0),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.InterfaceVnet != nil {
		t.Fatalf("interface_vnet pointer = %v, want nil", *req.InterfaceVnet)
	}
	requireAbsent(t, decodeJSON(t, marshalRequest(t, req)), "interface_vnet")
}

func TestNetworkCreateRequestSendsPositiveInterfaceVnet(t *testing.T) {
	data := &NetworkResourceModel{
		Name:           types.StringValue("tf-acc-net"),
		Type:           types.StringValue("external"),
		Interface_Vnet: types.Int32Value(4),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.InterfaceVnet == nil || *req.InterfaceVnet != 4 {
		t.Fatalf("interface_vnet pointer = %v, want 4", req.InterfaceVnet)
	}
	requireNumber(t, decodeJSON(t, marshalRequest(t, req)), "interface_vnet", 4)
}

func TestNetworkUpdateRequestOmitsInterfaceVnetZero(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("renamed"),
		Interface_Vnet: types.Int32Value(0),
	}
	state := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("tf-acc-net"),
		Interface_Vnet: types.Int32Null(),
	}

	req, _, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if req.InterfaceVnet != nil {
		t.Fatalf("interface_vnet pointer = %v, want nil", *req.InterfaceVnet)
	}
	obj := decodeJSON(t, marshalRequest(t, req))
	requireString(t, obj, "name", "renamed")
	requireAbsent(t, obj, "interface_vnet")

	// A plan that still holds 0 must not be sent over a real parent id.
	plan.Interface_Vnet = types.Int32Value(0)
	state.Interface_Vnet = types.Int32Value(4)
	plan.Name = state.Name
	req, _, err = networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if req.InterfaceVnet != nil {
		t.Fatalf("interface_vnet pointer = %v, want nil", *req.InterfaceVnet)
	}
	requireAbsent(t, decodeJSON(t, marshalRequest(t, req)), "interface_vnet")
}

func TestNetworkUpdateRequestSendsChangedInterfaceVnet(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("tf-acc-net"),
		Interface_Vnet: types.Int32Value(5),
	}
	state := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("tf-acc-net"),
		Interface_Vnet: types.Int32Value(4),
	}

	req, _, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if req.InterfaceVnet == nil || *req.InterfaceVnet != 5 {
		t.Fatalf("interface_vnet pointer = %v, want 5", req.InterfaceVnet)
	}
	requireNumber(t, decodeJSON(t, marshalRequest(t, req)), "interface_vnet", 5)
}

func TestNetworkCreateRequestOmitsUnsetEnabled(t *testing.T) {
	data := &NetworkResourceModel{
		Name:    types.StringValue("tf-acc-net"),
		Enabled: types.BoolNull(),
		Type:    types.StringValue("internal"),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Enabled != nil {
		t.Fatalf("unset enabled was sent: %v", *req.Enabled)
	}
	body := marshalRequest(t, req)
	if strings.Contains(body, `"enabled"`) {
		t.Fatalf("create body included unset enabled: %s", body)
	}
}

func TestNetworkCreateRequestSendsExplicitFalse(t *testing.T) {
	data := &NetworkResourceModel{
		Name:    types.StringValue("tf-acc-net"),
		Enabled: types.BoolValue(false),
		Type:    types.StringValue("internal"),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Enabled == nil || *req.Enabled {
		t.Fatalf("enabled pointer = %v, want false", req.Enabled)
	}
	body := marshalRequest(t, req)
	if !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("create body dropped enabled=false: %s", body)
	}
}

func TestNetworkCreateRequestKeepsFalseAndZero(t *testing.T) {
	data := &NetworkResourceModel{
		Name:            types.StringValue("tf-acc-net"),
		DHCP:            types.BoolValue(false),
		Dynamic_DHCP:    types.BoolValue(false),
		VLAN_TAG:        types.Int32Value(0),
		MTU:             types.Int32Value(0),
		IPaddress:       types.StringValue(""),
		DynamicIP_Start: types.StringNull(),
		Enabled:         types.BoolNull(),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	body := marshalRequest(t, req)
	obj := decodeJSON(t, body)
	requireBool(t, obj, "dhcp_enabled", false)
	requireBool(t, obj, "dhcp_dynamic", false)
	requireNumber(t, obj, "layer2_id", 0)
	requireNumber(t, obj, "mtu", 0)
	requireAbsent(t, obj, "enabled")
	requireAbsent(t, obj, "dhcp_start")
}

func TestNetworkUpdateRequestKeepsFalseZeroAndEmpty(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:              types.StringValue("12"),
		Name:            types.StringValue("tf-acc-net"),
		DHCP:            types.BoolValue(false),
		VLAN_TAG:        types.Int32Value(0),
		IPaddress:       types.StringValue(""),
		DynamicIP_Start: types.StringValue(""),
		Enabled:         types.BoolValue(true),
	}
	state := &NetworkResourceModel{
		Id:              types.StringValue("12"),
		Name:            types.StringValue("tf-acc-net"),
		DHCP:            types.BoolValue(true),
		VLAN_TAG:        types.Int32Value(20),
		IPaddress:       types.StringValue("10.0.0.1"),
		DynamicIP_Start: types.StringValue("10.0.0.10"),
		Enabled:         types.BoolValue(true),
	}

	req, id, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if id != 12 {
		t.Fatalf("network id = %d, want 12", id)
	}
	obj := decodeJSON(t, marshalRequest(t, req))
	requireBool(t, obj, "dhcp_enabled", false)
	requireNumber(t, obj, "layer2_id", 0)
	requireString(t, obj, "ipaddress", "")
	requireString(t, obj, "dhcp_start", "")
	requireAbsent(t, obj, "enabled")
	requireAbsent(t, obj, "name")
}

func TestNetworkCreateRequestOmitsUnsetClientSettings(t *testing.T) {
	data := &NetworkResourceModel{
		Name:        types.StringValue("tf-acc-net"),
		Description: types.StringNull(),
		DNSList:     types.StringNull(),
		Domain:      types.StringNull(),
		RateLimit:   types.Int64Null(),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeJSON(t, marshalRequest(t, req))
	requireAbsent(t, obj, "description")
	requireAbsent(t, obj, "dnslist")
	requireAbsent(t, obj, "domain")
	requireAbsent(t, obj, "rate_limit")
}

func TestNetworkCreateRequestSendsClientSettings(t *testing.T) {
	data := &NetworkResourceModel{
		Name:        types.StringValue("tf-acc-net"),
		Description: types.StringValue("Internal production network"),
		DNSList:     types.StringValue("8.8.8.8,8.8.4.4"),
		Domain:      types.StringValue("example.local"),
		RateLimit:   types.Int64Value(100),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeJSON(t, marshalRequest(t, req))
	requireString(t, obj, "description", "Internal production network")
	requireString(t, obj, "dnslist", "8.8.8.8,8.8.4.4")
	requireString(t, obj, "domain", "example.local")
	requireNumber(t, obj, "rate_limit", 100)
}

func TestNetworkCreateRequestSendsExplicitRateLimitZero(t *testing.T) {
	data := &NetworkResourceModel{
		Name:      types.StringValue("tf-acc-net"),
		RateLimit: types.Int64Value(0),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	requireNumber(t, decodeJSON(t, marshalRequest(t, req)), "rate_limit", 0)
}

func TestNetworkUpdateRequestSendsChangedClientSettingsOnly(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:          types.StringValue("12"),
		Name:        types.StringValue("tf-acc-net"),
		Description: types.StringValue(""),
		DNSList:     types.StringValue("1.1.1.1"),
		Domain:      types.StringValue("example.local"),
		RateLimit:   types.Int64Value(0),
	}
	state := &NetworkResourceModel{
		Id:          types.StringValue("12"),
		Name:        types.StringValue("tf-acc-net"),
		Description: types.StringValue("old"),
		DNSList:     types.StringValue("8.8.8.8,8.8.4.4"),
		Domain:      types.StringValue("example.local"),
		RateLimit:   types.Int64Value(100),
	}

	req, _, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeJSON(t, marshalRequest(t, req))
	requireString(t, obj, "description", "")
	requireString(t, obj, "dnslist", "1.1.1.1")
	requireAbsent(t, obj, "domain")
	requireNumber(t, obj, "rate_limit", 0)
}

func TestNetworkUpdateRequestOmitsUnsetClientSettings(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:          types.StringValue("12"),
		Name:        types.StringValue("renamed"),
		Description: types.StringNull(),
		DNSList:     types.StringNull(),
		Domain:      types.StringNull(),
		RateLimit:   types.Int64Null(),
	}
	state := &NetworkResourceModel{
		Id:          types.StringValue("12"),
		Name:        types.StringValue("tf-acc-net"),
		Description: types.StringValue("set in the UI"),
		DNSList:     types.StringValue("8.8.8.8"),
		Domain:      types.StringValue("example.local"),
		RateLimit:   types.Int64Value(100),
	}

	req, _, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeJSON(t, marshalRequest(t, req))
	requireString(t, obj, "name", "renamed")
	requireAbsent(t, obj, "description")
	requireAbsent(t, obj, "dnslist")
	requireAbsent(t, obj, "domain")
	requireAbsent(t, obj, "rate_limit")
}

func TestNetworkCreateRequestAcceptsBoolPowerState(t *testing.T) {
	data := &NetworkResourceModel{
		Name:       types.StringValue("tf-acc-net"),
		PowerState: types.BoolValue(false),
	}

	req, err := networkCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	// Power is a vnet action after create. The create body has no powerstate field.
	requireAbsent(t, decodeJSON(t, marshalRequest(t, req)), "powerstate")
}

func TestNetworkUpdateRequestSendsIPAddressType(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("tf-acc-net"),
		IPaddress_Type: types.StringValue("none"),
	}
	state := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("tf-acc-net"),
		IPaddress_Type: types.StringValue("static"),
	}

	req, id, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if id != 12 {
		t.Fatalf("network id = %d, want 12", id)
	}
	if req.IPAddressType == nil || *req.IPAddressType != "none" {
		t.Fatalf("ipaddress_type pointer = %v, want none", req.IPAddressType)
	}
	obj := decodeJSON(t, marshalRequest(t, req))
	requireString(t, obj, "ipaddress_type", "none")
	requireAbsent(t, obj, "name")
}

func TestNetworkUpdateRequestOmitsUnchangedIPAddressType(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("renamed"),
		IPaddress_Type: types.StringValue("static"),
	}
	state := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("tf-acc-net"),
		IPaddress_Type: types.StringValue("static"),
	}

	req, _, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	obj := decodeJSON(t, marshalRequest(t, req))
	requireString(t, obj, "name", "renamed")
	requireAbsent(t, obj, "ipaddress_type")
}

func TestUpdateNetworkSendsIPAddressType(t *testing.T) {
	var bodies []string
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnets/12":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			bodies = append(bodies, string(body))
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"$key":12,"name":"renamed"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	})

	plan := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("renamed"),
		IPaddress_Type: types.StringValue("none"),
		PowerState:     types.BoolNull(),
	}
	state := &NetworkResourceModel{
		Id:             types.StringValue("12"),
		Name:           types.StringValue("tf-acc-net"),
		IPaddress_Type: types.StringValue("static"),
		PowerState:     types.BoolNull(),
	}
	if err := api.updateNetwork(t.Context(), plan, state); err != nil {
		t.Fatal(err)
	}

	var sawName, sawType bool
	for _, body := range bodies {
		obj := decodeJSON(t, body)
		if _, ok := obj["name"]; ok {
			requireString(t, obj, "name", "renamed")
			sawName = true
		}
		if _, ok := obj["ipaddress_type"]; ok {
			requireString(t, obj, "ipaddress_type", "none")
			sawType = true
		}
	}
	if !sawName || !sawType {
		t.Fatalf("update bodies = %#v, want name and ipaddress_type", bodies)
	}
}

func TestNetworkUpdateRequestOmitsPowerState(t *testing.T) {
	plan := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("tf-acc-net"),
		PowerState: types.BoolValue(true),
	}
	state := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("tf-acc-net"),
		PowerState: types.BoolValue(false),
	}

	req, _, err := networkUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	requireAbsent(t, decodeJSON(t, marshalRequest(t, req)), "powerstate")
}

func marshalRequest(t *testing.T, req any) string {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func decodeJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	obj := map[string]any{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
	return obj
}

func requireBool(t *testing.T, obj map[string]any, key string, want bool) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %v", key, got, want)
	}
}

func requireString(t *testing.T, obj map[string]any, key, want string) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %q", key, got, want)
	}
}

func requireNumber(t *testing.T, obj map[string]any, key string, want float64) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %v", key, got, want)
	}
}

func requireAbsent(t *testing.T, obj map[string]any, key string) {
	t.Helper()
	if _, ok := obj[key]; ok {
		t.Fatalf("field %q was sent: %#v", key, obj[key])
	}
}
