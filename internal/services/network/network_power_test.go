// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCreateNetworkPowersOnWhenRequested(t *testing.T) {
	var createBody string
	var actions []string
	powered := false
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			createBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":12}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`[]`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeNetworkPower(t, w, powered)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			actions = append(actions, string(body))
			if strings.Contains(string(body), `"action":"poweron"`) {
				powered = true
			}
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	data := &NetworkResourceModel{
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(true),
	}
	if err := api.createNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() != "12" {
		t.Fatalf("id = %s, want 12", data.Id.ValueString())
	}
	if strings.Contains(createBody, "powerstate") {
		t.Fatalf("create body included powerstate: %s", createBody)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweron"`) || !strings.Contains(actions[0], `"vnet":12`) {
		t.Fatalf("actions = %#v, want one poweron for vnet 12", actions)
	}
	if err := api.readNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.PowerState.IsNull() || !data.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want true from running", data.PowerState)
	}
}

func TestCreateNetworkLeavesStoppedWhenPowerStateIsFalse(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":12}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`[]`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeNetworkPower(t, w, false)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			t.Errorf("stopped create posted %s", r.URL.Path)
			http.Error(w, "unexpected action", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	data := &NetworkResourceModel{
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.PowerState.IsNull() || data.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want false", data.PowerState)
	}
}

func TestCreateNetworkOmitsPowerWhenUnset(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":12}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`[]`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeNetworkPower(t, w, false)
		case r.Method == http.MethodPost:
			t.Errorf("unset powerstate posted %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected action", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	data := &NetworkResourceModel{
		Name:       types.StringValue("lan"),
		PowerState: types.BoolNull(),
	}
	if err := api.createNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
}

func TestCreateNetworkDeletesStaleIPSec(t *testing.T) {
	var calls []string
	phase1Gone := map[string]bool{}
	phase2Gone := map[string]bool{}
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			calls = append(calls, r.URL.Path)
			if strings.Contains(r.URL.Path, "phase1s") {
				phase1Gone[r.URL.Path] = true
			}
			if strings.Contains(r.URL.Path, "phase2s") {
				phase2Gone[r.URL.Path] = true
			}
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":18}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/18":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"$key":18,"name":"lan","running":false}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`[{"$key":1,"vnet":18},{"$key":2,"vnet":18}]`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			w.Header().Set("Content-Type", "application/json")
			filter := r.URL.Query().Get("filter")
			body := `[]`
			switch {
			case strings.Contains(filter, "ipsec eq 1") && !phase1Gone["/api/v4/vnet_ipsec_phase1s/1"]:
				body = `[{"$key":1,"ipsec":1,"name":"old"}]`
			case strings.Contains(filter, "ipsec eq 2") && !phase1Gone["/api/v4/vnet_ipsec_phase1s/3"]:
				body = `[{"$key":3,"ipsec":2,"name":"other"}]`
			}
			if _, err := w.Write([]byte(body)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			w.Header().Set("Content-Type", "application/json")
			filter := r.URL.Query().Get("filter")
			body := `[]`
			switch {
			case strings.Contains(filter, "phase1 eq 1") && !phase2Gone["/api/v4/vnet_ipsec_phase2s/1"]:
				body = `[{"$key":1,"phase1":1,"name":"lan-to-lan"}]`
			case strings.Contains(filter, "phase1 eq 3") && !phase2Gone["/api/v4/vnet_ipsec_phase2s/4"]:
				body = `[{"$key":4,"phase1":3,"name":"extra"}]`
			}
			if _, err := w.Write([]byte(body)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_ipsec"):
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	data := &NetworkResourceModel{
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() != "18" {
		t.Fatalf("id = %s", data.Id.ValueString())
	}
	want := []string{
		"/api/v4/vnet_ipsec_phase2s/1",
		"/api/v4/vnet_ipsec_phase1s/1",
		"/api/v4/vnet_ipsecs/1",
		"/api/v4/vnet_ipsec_phase2s/4",
		"/api/v4/vnet_ipsec_phase1s/3",
		"/api/v4/vnet_ipsecs/2",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("deletes = %v, want both stale IPsec trees removed", calls)
	}
}

func TestCreateNetworkReportsStaleIPSecDeleteFailure(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":18}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/18":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"$key":18,"name":"lan","running":false}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`[{"$key":1,"vnet":18}]`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			http.Error(w, "list failed", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	err := api.createNetwork(t.Context(), &NetworkResourceModel{
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(false),
	})
	if err == nil || !strings.Contains(err.Error(), "network 18") || !strings.Contains(err.Error(), "key 18") {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdateNetworkPowersOnWithoutPutField(t *testing.T) {
	var putBody string
	var actions []string
	powered := false
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnets/12":
			putBody = string(body)
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeNetworkPower(t, w, powered)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			actions = append(actions, string(body))
			if strings.Contains(string(body), `"action":"poweron"`) {
				powered = true
			}
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	plan := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(true),
	}
	state := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(false),
	}
	if err := api.updateNetwork(t.Context(), plan, state); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(putBody, "powerstate") {
		t.Fatalf("update body included powerstate: %s", putBody)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweron"`) || strings.Contains(actions[0], `"kill"`) {
		t.Fatalf("actions = %#v, want one poweron", actions)
	}
}

func TestUpdateNetworkKillsWhenPowerStateIsFalse(t *testing.T) {
	shortenNetworkStop(t, time.Second, 0)

	var putBody string
	var actions []string
	killed := false
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnets/12":
			putBody = string(body)
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			// The powerstate column stays true. Running is what changes.
			writeNetworkPowerColumn(t, w, !killed, true)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
			writeNetworkPowerList(t, w, !killed)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			actions = append(actions, string(body))
			if strings.Contains(string(body), `"action":"kill"`) {
				killed = true
			}
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	plan := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(false),
	}
	state := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(true),
	}
	if err := api.updateNetwork(t.Context(), plan, state); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(putBody, "powerstate") {
		t.Fatalf("update body included powerstate: %s", putBody)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"kill"`) || !strings.Contains(actions[0], `"vnet":12`) {
		t.Fatalf("actions = %#v, want one kill for vnet 12", actions)
	}
	if plan.PowerState.IsNull() || plan.PowerState.ValueBool() {
		t.Fatalf("powerstate after kill = %#v, want false", plan.PowerState)
	}
}

func TestUpdateNetworkLeavesRunningMachineWhenPlanIsOn(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnets/12":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			// Column says stopped. The machine is running, so do not power-cycle.
			writeNetworkPowerColumn(t, w, true, false)
		case r.Method == http.MethodPost:
			t.Errorf("running network with powerstate true posted %s", r.URL.Path)
			http.Error(w, "unexpected action", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	plan := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(true),
	}
	state := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(false),
	}
	if err := api.updateNetwork(t.Context(), plan, state); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateNetworkOmitsPowerWhenUnset(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnets/12":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			// Update reads the network back. Power is left unchanged, so this
			// must not be followed by an action.
			writeNetworkPower(t, w, true)
		case r.Method == http.MethodPost:
			t.Errorf("unset powerstate posted %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected action", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	plan := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("renamed"),
		PowerState: types.BoolNull(),
	}
	state := &NetworkResourceModel{
		Id:         types.StringValue("12"),
		Name:       types.StringValue("lan"),
		PowerState: types.BoolValue(true),
	}
	if err := api.updateNetwork(t.Context(), plan, state); err != nil {
		t.Fatal(err)
	}
}

func TestReadNetworkPowerStateUsesRunning(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v4/vnets/12" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		// The powerstate column is false while the machine is running.
		writeNetworkPowerColumn(t, w, true, false)
	})

	data := &NetworkResourceModel{Id: types.StringValue("12")}
	if err := api.readNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.PowerState.IsNull() || !data.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want true from running", data.PowerState)
	}
}

func TestStopNetworkBeforeDeleteUsesRunningNotPowerColumn(t *testing.T) {
	shortenNetworkStop(t, time.Second, 0)

	var actions []string
	killed := false
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
			writeNetworkPowerListColumn(t, w, !killed, false)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			actions = append(actions, string(body))
			killed = true
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	data := &NetworkResourceModel{
		Id:   types.StringValue("12"),
		Name: types.StringValue("lan"),
	}
	if err := api.stopNetworkBeforeDelete(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"kill"`) {
		t.Fatalf("actions = %#v, want one kill when running is true and the powerstate column is false", actions)
	}
	if data.PowerState.IsNull() || data.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want false", data.PowerState)
	}
}

func writeNetworkPower(t *testing.T, w http.ResponseWriter, running bool) {
	t.Helper()
	writeNetworkPowerColumn(t, w, running, false)
}

func writeNetworkPowerColumn(t *testing.T, w http.ResponseWriter, running, column bool) {
	t.Helper()
	status := "stopped"
	if running {
		status = "running"
	}
	payload := `{"$key":12,"name":"lan","enabled":true,"running":` + boolJSON(running) + `,"status":"` + status + `","powerstate":` + boolJSON(column) + `}`
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func writeNetworkPowerList(t *testing.T, w http.ResponseWriter, running bool) {
	t.Helper()
	writeNetworkPowerListColumn(t, w, running, false)
}

func writeNetworkPowerListColumn(t *testing.T, w http.ResponseWriter, running, column bool) {
	t.Helper()
	status := "stopped"
	if running {
		status = "running"
	}
	payload := `[{"$key":12,"name":"lan","running":` + boolJSON(running) + `,"status":"` + status + `","powerstate":` + boolJSON(column) + `}]`
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func boolJSON(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
