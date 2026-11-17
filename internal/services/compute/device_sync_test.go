package compute

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestSyncDevicesCreatesFirstWhenStateIsEmpty(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_devices":
			if !strings.Contains(string(body), `"name":"gpu"`) {
				t.Errorf("POST body = %s, want name gpu", body)
			}
			if strings.Contains(string(body), "settings_args") {
				t.Errorf("non-TPM create sent settings_args: %s", body)
			}
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":"12"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_devices/12":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"type":"node_pci_devices","name":"gpu","enabled":true}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*deviceResourceModel{{
		Name:    types.StringValue("gpu"),
		Type:    types.StringValue("node_pci_devices"),
		Enabled: types.BoolValue(true),
	}}
	var state []*deviceResourceModel

	if err := api.syncDevices(t.Context(), &plan, &state, types.Int32Value(1)); err != nil {
		t.Fatal(err)
	}
	if len(state) != 1 || state[0].Key.ValueString() != "12" || state[0].Name.ValueString() != "gpu" {
		t.Fatalf("state after first device = key %q name %q len %d", state[0].Key.ValueString(), state[0].Name.ValueString(), len(state))
	}
	if len(calls) < 2 || calls[0] != "POST /api/v4/machine_devices" || calls[1] != "GET /api/v4/machine_devices/12" {
		t.Fatalf("calls = %#v, want POST then GET of the new device", calls)
	}
}

func TestSyncDevicesDeletesLastWhenPlanIsEmpty(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/machine_devices/12" {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	var plan []*deviceResourceModel
	state := []*deviceResourceModel{{
		Key:  types.StringValue("12"),
		Name: types.StringValue("gpu"),
		Type: types.StringValue("node_pci_devices"),
	}}

	if err := api.syncDevices(t.Context(), &plan, &state, types.Int32Value(1)); err != nil {
		t.Fatal(err)
	}
	if len(state) != 0 {
		t.Fatalf("state after removing the last device = %d devices, want 0", len(state))
	}
	if len(calls) != 1 || calls[0] != "DELETE /api/v4/machine_devices/12" {
		t.Fatalf("calls = %#v, want DELETE of device 12", calls)
	}
}
