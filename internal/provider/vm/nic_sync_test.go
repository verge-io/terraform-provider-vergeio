package vm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

func TestSyncNICsRenamePutsInPlace(t *testing.T) {
	const mac = "52:54:00:11:22:33"
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_nics/98":
			if !strings.Contains(string(body), `"name":"lan"`) {
				t.Errorf("PUT body = %s, want name lan", body)
			}
			if strings.Contains(string(body), "macaddress") {
				t.Errorf("PUT body changed the MAC: %s", body)
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"lan","interface":"virtio","enabled":true,"vnet":6,"macaddress":"` + mac + `"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &NICApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*nicResourceModel{{
		Id:        types.StringValue("98"),
		Name:      types.StringValue("lan"),
		Interface: types.StringValue("virtio"),
		Enabled:   types.BoolValue(true),
		VNET:      types.Int32Value(6),
		MAC:       types.StringValue(mac),
	}}
	state := []*nicResourceModel{{
		Id:        types.StringValue("98"),
		Name:      types.StringValue("nic0"),
		Interface: types.StringValue("virtio"),
		Enabled:   types.BoolValue(true),
		VNET:      types.Int32Value(6),
		MAC:       types.StringValue(mac),
	}}
	if err := api.syncNICs(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if len(state) != 1 || state[0].Id.ValueString() != "98" || state[0].Name.ValueString() != "lan" || state[0].MAC.ValueString() != mac {
		t.Fatalf("state after rename = id %q name %q mac %q", state[0].Id.ValueString(), state[0].Name.ValueString(), state[0].MAC.ValueString())
	}
	if len(calls) != 2 || calls[0] != "PUT /api/v4/machine_nics/98" || calls[1] != "GET /api/v4/machine_nics/98" {
		t.Fatalf("calls = %#v, want PUT then GET of NIC 98", calls)
	}
}

func TestSyncNICsUnrelatedVMUpdateDoesNotRewrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &NICApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	stateNIC := &nicResourceModel{
		Id:          types.StringValue("98"),
		Machine:     types.Int32Value(68),
		Name:        types.StringValue("nic0"),
		Description: types.StringValue("lan"),
		Interface:   types.StringValue("virtio"),
		Driver:      types.StringValue("virtio"),
		Model:       types.StringValue("virtio"),
		Vendor:      types.StringValue("redhat"),
		Port:        types.Int32Value(1),
		Enabled:     types.BoolValue(true),
		VNET:        types.Int32Value(6),
		MAC:         types.StringValue("52:54:00:11:22:33"),
		Asset:       types.StringValue("nic-asset"),
	}
	planNIC := &nicResourceModel{
		Id:          stateNIC.Id,
		Name:        stateNIC.Name,
		Interface:   stateNIC.Interface,
		VNET:        stateNIC.VNET,
		MAC:         stateNIC.MAC,
		Description: types.StringUnknown(),
		Driver:      types.StringUnknown(),
		Model:       types.StringUnknown(),
		Vendor:      types.StringUnknown(),
		Port:        types.Int32Unknown(),
		Enabled:     types.BoolUnknown(),
		Asset:       types.StringUnknown(),
		Machine:     types.Int32Unknown(),
	}
	plan := []*nicResourceModel{planNIC}
	state := []*nicResourceModel{stateNIC}

	if err := api.syncNICs(t.Context(), &plan, &state, types.Int32Value(68), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if state[0].Id.ValueString() != "98" || state[0].MAC.ValueString() != "52:54:00:11:22:33" || !state[0].Enabled.ValueBool() {
		t.Fatalf("untouched NIC state changed: id %q mac %q enabled %v", state[0].Id.ValueString(), state[0].MAC.ValueString(), state[0].Enabled.ValueBool())
	}
}

func TestSyncNICsCreatesFirstWhenStateIsEmpty(t *testing.T) {
	const mac = "52:54:00:aa:bb:cc"
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_nics":
			if !strings.Contains(string(body), `"name":"lan"`) {
				t.Errorf("POST body = %s, want name lan", body)
			}
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":"98"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"lan","interface":"virtio","enabled":true,"vnet":6,"macaddress":"` + mac + `"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &NICApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*nicResourceModel{{
		Name:      types.StringValue("lan"),
		Interface: types.StringValue("virtio"),
		Enabled:   types.BoolValue(true),
		VNET:      types.Int32Value(6),
	}}
	var state []*nicResourceModel

	if err := api.syncNICs(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if len(state) != 1 || state[0].Id.ValueString() != "98" || state[0].Name.ValueString() != "lan" {
		t.Fatalf("state after first NIC = %#v", state)
	}
	if len(calls) < 2 || calls[0] != "POST /api/v4/machine_nics" || calls[1] != "GET /api/v4/machine_nics/98" {
		t.Fatalf("calls = %#v, want POST then GET of the new NIC", calls)
	}
}
