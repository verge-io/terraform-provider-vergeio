package compute

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

// TestNICCreatePayloadOmitsUnsetFields locks the contract behind the disabled
// NIC bug (issue 52): a create built from a plan that does not set optional
// attributes must not send those fields. ValueBool() on an unset bool is
// false, and a JSON bool without omitempty would override the platform
// default. An omitted enabled leaves VergeOS's default, which is enabled.
func TestNICCreatePayloadOmitsUnsetFields(t *testing.T) {
	data := &nicResourceModel{
		Name:      types.StringValue("nic0"),
		Interface: types.StringValue("virtio"),
		VNET:      types.Int32Value(18),
		Enabled:   types.BoolNull(),
	}

	payload := decodePayload(t, nicCreatePayload(data))
	for _, key := range []string{"enabled", "description", "driver", "model", "vendor", "port", "macaddress", "asset", "machine"} {
		if _, ok := payload[key]; ok {
			t.Errorf("unset field %q was sent: %#v", key, payload[key])
		}
	}
	if payload["name"] != "nic0" || payload["interface"] != "virtio" || payload["vnet"] != float64(18) {
		t.Fatalf("required fields missing from %#v", payload)
	}
}

func TestNICCreatePayloadOmitsUnknownBool(t *testing.T) {
	data := &nicResourceModel{
		Name:      types.StringValue("nic0"),
		Interface: types.StringValue("virtio"),
		VNET:      types.Int32Value(18),
		Enabled:   types.BoolUnknown(),
	}
	payload := decodePayload(t, nicCreatePayload(data))
	if _, ok := payload["enabled"]; ok {
		t.Fatalf("unknown enabled was sent: %#v", payload["enabled"])
	}
}

func TestNICCreatePayloadSendsExplicitFalseZeroAndEmpty(t *testing.T) {
	data := &nicResourceModel{
		Name:        types.StringValue("nic0"),
		Interface:   types.StringValue("virtio"),
		Description: types.StringValue(""),
		Port:        types.Int32Value(0),
		VNET:        types.Int32Value(18),
		Enabled:     types.BoolValue(false),
	}
	payload := decodePayload(t, nicCreatePayload(data))
	enabled, ok := payload["enabled"].(bool)
	if !ok || enabled {
		t.Fatalf("explicit enabled=false was not sent, payload=%#v", payload)
	}
	if payload["description"] != "" {
		t.Fatalf("explicit empty description was not sent, payload=%#v", payload)
	}
	if payload["port"] != float64(0) {
		t.Fatalf("explicit port=0 was not sent, payload=%#v", payload)
	}
}

func TestNICUpdatePayloadSendsExplicitFalseZeroAndEmpty(t *testing.T) {
	plan := &nicResourceModel{
		Name:        types.StringValue("nic0"),
		Description: types.StringValue(""),
		Port:        types.Int32Value(0),
		Enabled:     types.BoolValue(false),
		VNET:        types.Int32Value(18),
	}
	state := &nicResourceModel{
		Name:        types.StringValue("nic0"),
		Description: types.StringValue("before"),
		Port:        types.Int32Value(1),
		Enabled:     types.BoolValue(true),
		VNET:        types.Int32Value(18),
	}
	body := jsonObject(t, nicUpdatePayload(plan, state))
	requireBool(t, body, "enabled", false)
	requireString(t, body, "description", "")
	requireNumber(t, body, "port", 0)
	requireAbsent(t, body, "name")
	requireAbsent(t, body, "vnet")
}

func TestNICUpdatePayloadOmitsUnsetEnabled(t *testing.T) {
	plan := &nicResourceModel{
		Name:    types.StringValue("nic1"),
		Enabled: types.BoolNull(),
	}
	state := &nicResourceModel{
		Name:    types.StringValue("nic0"),
		Enabled: types.BoolValue(true),
	}
	body := jsonObject(t, nicUpdatePayload(plan, state))
	requireString(t, body, "name", "nic1")
	requireAbsent(t, body, "enabled")
}

func TestNICCreatePayloadSendsExplicitFalse(t *testing.T) {
	data := &nicResourceModel{
		Name:      types.StringValue("nic0"),
		Interface: types.StringValue("virtio"),
		VNET:      types.Int32Value(18),
		Enabled:   types.BoolValue(false),
	}
	payload := decodePayload(t, nicCreatePayload(data))
	enabled, ok := payload["enabled"].(bool)
	if !ok || enabled {
		t.Fatalf("explicit enabled=false was not sent, payload=%#v", payload)
	}
}

func TestNICIPFromAPI(t *testing.T) {
	if got := nicIPFromAPI(""); !got.IsNull() {
		t.Fatalf("empty IP = %#v, want null", got)
	}
	if got := nicIPFromAPI("  "); !got.IsNull() {
		t.Fatalf("blank IP = %#v, want null", got)
	}
	if got := nicIPFromAPI("10.0.0.8"); got.IsNull() || got.ValueString() != "10.0.0.8" {
		t.Fatalf("assigned IP = %#v", got)
	}
}

func TestNICIPAssignPayloadIncludesRequestedIP(t *testing.T) {
	data := &nicResourceModel{
		VNET:      types.Int32Value(26),
		MAC:       types.StringValue("52:54:00:11:22:33"),
		IPAddress: types.StringValue("  10.0.0.50  "),
	}
	body := jsonObject(t, nicIPAssignPayload(data))
	requireNumber(t, body, "vnet", 26)
	requireString(t, body, "mac", "52:54:00:11:22:33")
	requireString(t, body, "type", "static")
	requireString(t, body, "ip", "10.0.0.50")
}

func TestNICIPAssignPayloadOmitsUnsetIP(t *testing.T) {
	cases := []types.String{
		types.StringNull(),
		types.StringUnknown(),
		types.StringValue(""),
		types.StringValue("   "),
	}
	for _, ip := range cases {
		data := &nicResourceModel{
			VNET:      types.Int32Value(26),
			MAC:       types.StringValue("52:54:00:11:22:33"),
			IPAddress: ip,
		}
		body := jsonObject(t, nicIPAssignPayload(data))
		requireAbsent(t, body, "ip")
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "N/A") {
			t.Fatalf("payload contained placeholder N/A: %s", raw)
		}
	}
}

func TestPreserveNICConfigFields(t *testing.T) {
	prior := []*nicResourceModel{{
		Id:              types.StringValue("7"),
		AssignIPAddress: types.BoolValue(true),
	}}
	current := []*nicResourceModel{{
		Id:              types.StringValue("7"),
		Name:            types.StringValue("nic0"),
		AssignIPAddress: types.BoolNull(),
	}}
	preserveNICConfigFields(prior, current)
	if !current[0].AssignIPAddress.ValueBool() {
		t.Fatal("assign_ipaddress was not kept from prior state")
	}

	imported := []*nicResourceModel{{
		Id:   types.StringValue("7"),
		Name: types.StringValue("nic0"),
	}}
	preserveNICConfigFields(nil, imported)
	if !imported[0].AssignIPAddress.IsNull() {
		t.Fatal("import with no prior state should leave assign_ipaddress unset")
	}
}

func TestSortNICsForState(t *testing.T) {
	nics := []vergeos.VMNIC{
		{Key: vergeos.FlexInt(3), Name: "nic1"},
		{Key: vergeos.FlexInt(1), Name: "nic0"},
		{Key: vergeos.FlexInt(2), Name: "nic1"},
	}
	sortNICsForState(nics)
	if nics[0].Name != "nic0" || nics[1].Key.Int() != 2 || nics[2].Key.Int() != 3 {
		t.Fatalf("unexpected NIC order: %#v", nics)
	}
}

func decodePayload(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}
