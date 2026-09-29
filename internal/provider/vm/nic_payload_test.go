package vm

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestNICCreatePayloadOmitsUnsetFields locks the contract behind the disabled
// NIC bug: a create built from a plan that does not set optional attributes
// must not send those fields. ValueBool() on an unset bool is false, and a
// JSON bool without omitempty would override the platform default.
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
