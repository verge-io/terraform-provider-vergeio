package network

import (
	"encoding/json"
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
	// The SDK create body has no powerstate field, so a bool does not
	// become the string "false" on the wire.
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
