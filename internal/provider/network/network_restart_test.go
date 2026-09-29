// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

func TestStagedRestartDecision(t *testing.T) {
	tests := []struct {
		name            string
		needRestart     bool
		running         bool
		restartOnChange bool
		want            stagedRestartAction
	}{
		{name: "clear", want: stagedRestartNone},
		{name: "running and allowed", needRestart: true, running: true, restartOnChange: true, want: stagedRestartNow},
		{name: "opt out", needRestart: true, running: true, restartOnChange: false, want: stagedRestartSkipped},
		{name: "stopped", needRestart: true, running: false, restartOnChange: true, want: stagedRestartSkipped},
		{name: "stopped and opted out", needRestart: true, running: false, restartOnChange: false, want: stagedRestartSkipped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stagedRestartDecision(tt.needRestart, tt.running, tt.restartOnChange)
			if got != tt.want {
				t.Fatalf("decision = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkIsRunningUsesMachineStatus(t *testing.T) {
	if !networkIsRunning(&vergeos.Network{Running: true}) {
		t.Fatal("running flag should count as running")
	}
	if !networkIsRunning(&vergeos.Network{Status: "running"}) {
		t.Fatal("status running should count as running")
	}
	if networkIsRunning(&vergeos.Network{PowerState: true, Status: "stopped"}) {
		t.Fatal("powerstate alone should not count as running")
	}
	if networkIsRunning(&vergeos.Network{Status: "stopped"}) {
		t.Fatal("stopped status should not count as running")
	}
	if networkIsRunning(nil) {
		t.Fatal("nil network should not count as running")
	}
}

func TestRestartOnChangeDefaultsToTrue(t *testing.T) {
	if !restartOnChangeEnabled(types.BoolNull()) || !restartOnChangeEnabled(types.BoolUnknown()) {
		t.Fatal("null and unknown restart_on_change should default to true")
	}
	if !restartOnChangeEnabled(types.BoolValue(true)) {
		t.Fatal("explicit true should restart")
	}
	if restartOnChangeEnabled(types.BoolValue(false)) {
		t.Fatal("explicit false should not restart")
	}
}

func TestUpdateNetworkRestartsWhenChangeIsStaged(t *testing.T) {
	var calls []string
	var putBody, resetBody []byte
	resetSent := false
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnets/12":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read put body: %v", err)
			}
			putBody = body
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			payload := stagedNetworkJSON(true)
			if resetSent {
				payload = stagedNetworkJSON(false)
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(payload)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read reset body: %v", err)
			}
			resetBody = body
			resetSent = true
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	plan := &NetworkResourceModel{
		Id:              types.StringValue("12"),
		Name:            types.StringValue("lan"),
		DynamicIP_Stop:  types.StringValue("192.168.0.220"),
		RestartOnChange: types.BoolValue(true),
	}
	state := &NetworkResourceModel{
		Id:              types.StringValue("12"),
		Name:            types.StringValue("lan"),
		DynamicIP_Stop:  types.StringValue("192.168.0.200"),
		RestartOnChange: types.BoolValue(true),
	}
	if err := api.updateNetwork(t.Context(), plan, state); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(putBody), `"dhcp_stop":"192.168.0.220"`) {
		t.Fatalf("update body = %s, want the new dhcp_stop", putBody)
	}

	resource := &NetworkResource{networkApi: api}
	var diags diag.Diagnostics
	if err := resource.restartAfterUpdate(t.Context(), plan, &diags); err != nil {
		t.Fatal(err)
	}
	if diags.WarningsCount() != 0 || diags.HasError() {
		t.Fatalf("diagnostics = %v, want none", diags)
	}
	if plan.NeedRestart.IsNull() || plan.NeedRestart.ValueBool() {
		t.Fatalf("need_restart = %#v, want false after reset", plan.NeedRestart)
	}

	action := decodeJSONMap(t, resetBody)
	if action["action"] != "reset" {
		t.Fatalf("action = %v, want reset", action["action"])
	}
	if action["vnet"] != float64(12) {
		t.Fatalf("vnet = %v, want 12", action["vnet"])
	}
	params, ok := action["params"].(map[string]any)
	if !ok || params["apply"] != false {
		t.Fatalf("reset params = %#v, want apply false", action["params"])
	}

	if err := api.readNetwork(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	if plan.NeedRestart.ValueBool() {
		t.Fatal("read after reset still exposes need_restart")
	}
	if !plan.RestartOnChange.ValueBool() {
		t.Fatal("read cleared restart_on_change")
	}

	var postSeen bool
	for _, call := range calls {
		if call == "POST /api/v4/vnet_actions" {
			postSeen = true
			continue
		}
		if postSeen && call == "PUT /api/v4/vnets/12" {
			t.Fatalf("update ran after reset: %#v", calls)
		}
	}
	if !postSeen {
		t.Fatalf("calls = %#v, want a reset", calls)
	}
}

func TestRestartAfterUpdateWarnsWhenRestartDisabled(t *testing.T) {
	var calls []string
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			t.Errorf("restart_on_change false sent %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected reset", http.StatusInternalServerError)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12" {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(stagedNetworkJSON(true))); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
		http.Error(w, "unexpected", http.StatusInternalServerError)
	})

	data := &NetworkResourceModel{
		Id:              types.StringValue("12"),
		Name:            types.StringValue("lan"),
		RestartOnChange: types.BoolValue(false),
	}
	resource := &NetworkResource{networkApi: api}
	var diags diag.Diagnostics
	if err := resource.restartAfterUpdate(t.Context(), data, &diags); err != nil {
		t.Fatal(err)
	}
	if diags.HasError() {
		t.Fatalf("errors = %v", diags.Errors())
	}
	warnings := diags.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("warnings = %d, want 1", len(warnings))
	}
	if warnings[0].Summary() != "Network change is not live yet" {
		t.Fatalf("summary = %q", warnings[0].Summary())
	}
	if !strings.Contains(warnings[0].Detail(), "restart_on_change is false") || !strings.Contains(warnings[0].Detail(), "lan (id 12)") {
		t.Fatalf("detail = %q", warnings[0].Detail())
	}
	if data.NeedRestart.IsNull() || !data.NeedRestart.ValueBool() {
		t.Fatalf("need_restart = %#v, want true", data.NeedRestart)
	}

	if err := api.readNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if !data.NeedRestart.ValueBool() {
		t.Fatal("read hid a skipped need_restart")
	}
	if data.RestartOnChange.IsNull() || data.RestartOnChange.ValueBool() {
		t.Fatalf("restart_on_change = %#v, want false", data.RestartOnChange)
	}
	for _, call := range calls {
		if strings.Contains(call, "vnet_actions") {
			t.Fatalf("skipped restart still called reset: %#v", calls)
		}
	}
}

func TestRestartAfterUpdateSkipsStoppedNetwork(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Errorf("stopped network was reset")
			http.Error(w, "unexpected reset", http.StatusInternalServerError)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12" {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"$key":12,"name":"lan","enabled":true,"need_restart":true,"running":false,"status":"stopped","powerstate":true}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
		http.Error(w, "unexpected", http.StatusInternalServerError)
	})

	data := &NetworkResourceModel{
		Id:              types.StringValue("12"),
		Name:            types.StringValue("lan"),
		RestartOnChange: types.BoolNull(),
	}
	resource := &NetworkResource{networkApi: api}
	var diags diag.Diagnostics
	if err := resource.restartAfterUpdate(t.Context(), data, &diags); err != nil {
		t.Fatal(err)
	}
	warnings := diags.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0].Detail(), "not running") {
		t.Fatalf("warnings = %#v", warnings)
	}
	if !data.NeedRestart.ValueBool() {
		t.Fatal("stopped network should still expose need_restart")
	}
}

func TestRestartAfterUpdateErrorsWhenFlagStaysSet(t *testing.T) {
	origAttempts, origInterval := networkRestartAttempts, networkRestartInterval
	t.Cleanup(func() {
		networkRestartAttempts = origAttempts
		networkRestartInterval = origInterval
	})
	networkRestartAttempts = 2
	networkRestartInterval = 0

	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(stagedNetworkJSON(true))); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	data := &NetworkResourceModel{
		Id:              types.StringValue("12"),
		Name:            types.StringValue("lan"),
		RestartOnChange: types.BoolValue(true),
	}
	resource := &NetworkResource{networkApi: api}
	var diags diag.Diagnostics
	err := resource.restartAfterUpdate(t.Context(), data, &diags)
	if err == nil || !strings.Contains(err.Error(), "need_restart") {
		t.Fatalf("err = %v, want need_restart still set", err)
	}
	if diags.WarningsCount() != 0 {
		t.Fatal("a failed restart should be an error, not a warning")
	}
}

func TestReadNetworkDefaultsRestartOnChange(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"$key":12,"name":"lan","enabled":true,"need_restart":false,"running":false,"powerstate":false}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	})

	data := &NetworkResourceModel{Id: types.StringValue("12")}
	if err := api.readNetwork(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.RestartOnChange.IsNull() || !data.RestartOnChange.ValueBool() {
		t.Fatalf("restart_on_change = %#v, want true", data.RestartOnChange)
	}
	if data.NeedRestart.IsNull() || data.NeedRestart.ValueBool() {
		t.Fatalf("need_restart = %#v, want false", data.NeedRestart)
	}
}

func newRestartTestAPI(t *testing.T, handler http.HandlerFunc) *NetworkApi {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"version":"26.1.8"}`)); err != nil {
				t.Errorf("write version: %v", err)
			}
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	sdk, err := vergeos.NewClient(
		vergeos.WithBaseURL(server.URL),
		vergeos.WithCredentials("user", "pass"),
		vergeos.WithInsecureTLS(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &NetworkApi{
		name:   "Network Api",
		client: vergeio.NewClient(server.URL, "user", "pass", true),
		sdk:    sdk,
	}
}

func stagedNetworkJSON(needRestart bool) string {
	flag := "false"
	if needRestart {
		flag = "true"
	}
	return `{"$key":12,"name":"lan","enabled":true,"dhcp_enabled":true,"dhcp_stop":"192.168.0.220","need_restart":` + flag + `,"running":true,"status":"running","powerstate":true}`
}

func decodeJSONMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return obj
}
