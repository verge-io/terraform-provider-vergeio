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

func marshalRequest(t *testing.T, req any) string {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
