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

func TestNetworkPowerStateString(t *testing.T) {
	if got := networkPowerStateString(false); got != "false" {
		t.Fatalf("networkPowerStateString(false) = %q, want false", got)
	}
	if got := networkPowerStateString(true); got != "true" {
		t.Fatalf("networkPowerStateString(true) = %q, want true", got)
	}
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
