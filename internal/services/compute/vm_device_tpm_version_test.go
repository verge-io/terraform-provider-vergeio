// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestNormalizeTPMVersion(t *testing.T) {
	tests := map[string]string{
		"2.0":   "2",
		"1.2":   "1",
		"2":     "2",
		"1":     "1",
		" 2.0 ": "2",
		" 1.2 ": "1",
		"other": "other",
		"":      "",
	}
	for in, want := range tests {
		if got := normalizeTPMVersion(in); got != want {
			t.Errorf("normalizeTPMVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTPMVersionSemanticEquals(t *testing.T) {
	for _, tc := range []struct {
		prior string
		next  string
		want  bool
	}{
		{prior: "2.0", next: "2", want: true},
		{prior: "2", next: "2.0", want: true},
		{prior: "1.2", next: "1", want: true},
		{prior: "1", next: "1.2", want: true},
		{prior: "2.0", next: "2.0", want: true},
		{prior: "2", next: "2", want: true},
		{prior: "1.2", next: "1.2", want: true},
		{prior: "1", next: "1", want: true},
		{prior: " 2.0 ", next: "2", want: true},
		{prior: " 1.2 ", next: "1", want: true},
		{prior: "2.0", next: "1.2", want: false},
		{prior: "2", next: "1", want: false},
		{prior: "2.0", next: "1", want: false},
		{prior: "other", next: "2", want: false},
	} {
		equal, diags := NewTPMVersionValue(tc.next).StringSemanticEquals(t.Context(), NewTPMVersionValue(tc.prior))
		if diags.HasError() {
			t.Fatalf("StringSemanticEquals(%q, %q) diagnostics: %s", tc.next, tc.prior, diags)
		}
		if equal != tc.want {
			t.Errorf("StringSemanticEquals(%q, %q) = %t, want %t", tc.next, tc.prior, equal, tc.want)
		}
	}

	equal, diags := NewTPMVersionValue("2.0").StringSemanticEquals(t.Context(), types.StringValue("2"))
	if equal || !diags.HasError() {
		t.Fatalf("unexpected type equal=%t diags=%s", equal, diags)
	}
}

func TestTPMVersionSchemaKeepsConfig(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewVMResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	version := deviceSettingString(t, resp.Schema.Blocks, "tpm_settings", "version")
	if len(version.PlanModifiers) != 1 {
		t.Fatalf("tpm_settings.version has %d plan modifiers, want 1 RequiresReplace", len(version.PlanModifiers))
	}
	custom, ok := version.CustomType.(TPMVersionType)
	if !ok {
		t.Fatalf("tpm_settings.version custom type = %T, want TPMVersionType", version.CustomType)
	}
	if !custom.Equal(TPMVersionType{}) {
		t.Fatal("tpm_settings.version custom type is not TPMVersionType")
	}
}

func TestUpdateTPMSettingsOmitsVersion(t *testing.T) {
	for _, version := range []TPMVersion{
		NewTPMVersionValue("2.0"),
		NewTPMVersionValue("1.2"),
		NewTPMVersionValue("2"),
		NewTPMVersionValue("1"),
		NewTPMVersionNull(),
	} {
		var got map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if vergeio.AnswerCredentialCheck(w, r) {
				return
			}
			if r.Method != http.MethodPut || r.URL.Path != "/api/v4/machine_device_settings_tpm/9" {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusInternalServerError)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Errorf("decode body %s: %v", body, err)
			}
			writeTestBody(t, w, http.StatusOK, `{}`)
		}))

		api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
		err := api.updateTPMSettings(t.Context(), &deviceResourceModel{
			Name: types.StringValue("tpm"),
			DeviceTPMSettingsModel: &DeviceTPMSettingsModel{
				Model:   types.StringValue("crb"),
				Version: version,
			},
		}, types.Int32Value(9))
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got["version"]; ok {
			t.Fatalf("version %q was sent on update: %#v", version.ValueString(), got)
		}
		if got["model"] != "crb" {
			t.Fatalf("model = %#v, want crb", got["model"])
		}
	}
}

func TestCreateDeviceSendsTPMVersionOnce(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    string
		omit    bool
	}{
		{version: "2.0", want: "2"},
		{version: "1.2", want: "1"},
		{version: "2", want: "2"},
		{version: "1", want: "1"},
		{omit: true},
	} {
		var createBody map[string]any
		var updateBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if vergeio.AnswerCredentialCheck(w, r) {
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_devices":
				if err := json.Unmarshal(body, &createBody); err != nil {
					t.Errorf("decode create %s: %v", body, err)
				}
				writeTestBody(t, w, http.StatusCreated, `{"$key":"3"}`)
			case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_devices/3":
				writeTestBody(t, w, http.StatusOK, `{"machine":70,"type":"tpm","name":"tpm","enabled":true}`)
			case r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_device_settings_tpm/9":
				if err := json.Unmarshal(body, &updateBody); err != nil {
					t.Errorf("decode update %s: %v", body, err)
				}
				writeTestBody(t, w, http.StatusOK, `{}`)
			case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_device_settings_tpm":
				writeTestBody(t, w, http.StatusOK, `[{"$key":9,"machine_device":3,"model":"crb","version":"2"}]`)
			default:
				t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
				http.Error(w, "unexpected", http.StatusInternalServerError)
			}
		}))

		version := NewTPMVersionNull()
		if !tc.omit {
			version = NewTPMVersionValue(tc.version)
		}
		api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
		err := api.createDevice(t.Context(), &deviceResourceModel{
			Machine: types.Int32Value(70),
			Type:    types.StringValue("tpm"),
			Name:    types.StringValue("tpm"),
			Enabled: types.BoolValue(true),
			DeviceTPMSettingsModel: &DeviceTPMSettingsModel{
				Model:   types.StringValue("crb"),
				Version: version,
			},
		})
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		args, _ := createBody["settings_args"].(map[string]any)
		if tc.omit {
			if _, ok := createBody["settings_args"]; ok {
				t.Fatalf("unset version was sent on create: %#v", createBody["settings_args"])
			}
		} else if args["version"] != tc.want {
			t.Fatalf("create version %q sent as %#v, want %q", tc.version, createBody["settings_args"], tc.want)
		}
		if _, ok := updateBody["version"]; ok {
			t.Fatalf("version %q was sent on settings update: %#v", tc.version, updateBody)
		}
		if updateBody["model"] != "crb" {
			t.Fatalf("settings update model = %#v, want crb", updateBody["model"])
		}
	}
}

func TestReadTPMSettingsStoresVersionKey(t *testing.T) {
	for _, tc := range []struct {
		api  string
		want string
	}{
		{api: "2", want: "2"},
		{api: "1", want: "1"},
		{api: "2.0", want: "2"},
		{api: "1.2", want: "1"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if vergeio.AnswerCredentialCheck(w, r) {
				return
			}
			if r.Method != http.MethodGet || r.URL.Path != "/api/v4/machine_device_settings_tpm" {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected", http.StatusInternalServerError)
				return
			}
			writeTestBody(t, w, http.StatusOK, `[{"$key":9,"machine_device":3,"model":"crb","version":"`+tc.api+`"}]`)
		}))
		api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
		data := &deviceResourceModel{
			Key:                    types.StringValue("3"),
			Name:                   types.StringValue("tpm"),
			DeviceTPMSettingsModel: &DeviceTPMSettingsModel{},
		}
		err := api.readTPMSettings(t.Context(), data, false)
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got := data.DeviceTPMSettingsModel.Version.ValueString(); got != tc.want {
			t.Fatalf("API version %q stored as %q, want %q", tc.api, got, tc.want)
		}
	}
}

func TestSyncDevicesReplacesTPMWhenVersionChanges(t *testing.T) {
	var calls []string
	var createBody map[string]any
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
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/machine_devices/3":
			writeTestBody(t, w, http.StatusOK, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_devices":
			if err := json.Unmarshal(body, &createBody); err != nil {
				t.Errorf("decode create %s: %v", body, err)
			}
			writeTestBody(t, w, http.StatusCreated, `{"$key":"11"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_devices/11":
			writeTestBody(t, w, http.StatusOK, `{"machine":70,"type":"tpm","name":"tpm","enabled":true}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_device_settings_tpm/12":
			var updateBody map[string]any
			if err := json.Unmarshal(body, &updateBody); err != nil {
				t.Errorf("decode settings update %s: %v", body, err)
			}
			if _, ok := updateBody["version"]; ok {
				t.Errorf("settings update sent version %#v", updateBody["version"])
			}
			writeTestBody(t, w, http.StatusOK, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_device_settings_tpm":
			writeTestBody(t, w, http.StatusOK, `[{"$key":12,"machine_device":11,"model":"crb","version":"1"}]`)
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*deviceResourceModel{tpmDevice("1.2")}
	state := []*deviceResourceModel{tpmDevice("2")}
	if err := api.syncDevices(t.Context(), &plan, &state, types.Int32Value(70)); err != nil {
		t.Fatal(err)
	}
	if len(calls) < 2 || calls[0] != "DELETE /api/v4/machine_devices/3" || calls[1] != "POST /api/v4/machine_devices" {
		t.Fatalf("calls = %#v, want DELETE then POST for version replace", calls)
	}
	args, _ := createBody["settings_args"].(map[string]any)
	if args["version"] != "1" {
		t.Fatalf("recreate settings_args = %#v, want version 1", createBody["settings_args"])
	}
	if got := state[0].Key.ValueString(); got != "11" {
		t.Fatalf("state key after replace = %q, want 11", got)
	}
	if got := state[0].DeviceTPMSettingsModel.Version.ValueString(); got != "1" {
		t.Fatalf("state version after replace = %q, want 1", got)
	}
}

func TestSyncDevicesOmitsTPMVersionOnModelUpdate(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_devices/3":
			writeTestBody(t, w, http.StatusOK, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_devices/3":
			writeTestBody(t, w, http.StatusOK, `{"machine":70,"type":"tpm","name":"tpm","enabled":true}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_device_settings_tpm/9":
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode TPM update %s: %v", body, err)
			}
			writeTestBody(t, w, http.StatusOK, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_device_settings_tpm":
			writeTestBody(t, w, http.StatusOK, `[{"$key":9,"machine_device":3,"model":"tis","version":"2"}]`)
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*deviceResourceModel{tpmDevice("2")}
	plan[0].DeviceTPMSettingsModel.Model = types.StringValue("tis")
	state := []*deviceResourceModel{tpmDevice("2")}
	if err := api.syncDevices(t.Context(), &plan, &state, types.Int32Value(70)); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["version"]; ok {
		t.Fatalf("model-only TPM update sent version %#v", payload["version"])
	}
	if payload["model"] != "tis" {
		t.Fatalf("TPM update model = %#v, want tis", payload["model"])
	}
	if got := state[0].DeviceTPMSettingsModel.Version.ValueString(); got != "2" {
		t.Fatalf("state version = %q, want 2", got)
	}
}

func TestSyncDevicesSkipsSemanticallyEqualTPMVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		t.Errorf("semantically equal TPM version made a request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*deviceResourceModel{tpmDevice("2.0")}
	state := []*deviceResourceModel{tpmDevice("2")}
	if err := api.syncDevices(t.Context(), &plan, &state, types.Int32Value(70)); err != nil {
		t.Fatal(err)
	}
}

func TestTPMStoredVersionDiffers(t *testing.T) {
	for _, tc := range []struct {
		plan, state string
		want        bool
	}{
		{plan: "2", state: "2", want: false},
		{plan: "2.0", state: "2", want: false},
		{plan: "1.2", state: "1", want: false},
		{plan: "1.2", state: "2", want: true},
		{plan: "2", state: "1", want: true},
	} {
		got := tpmStoredVersionDiffers(tpmDevice(tc.plan), tpmDevice(tc.state))
		if got != tc.want {
			t.Errorf("tpmStoredVersionDiffers(%q, %q) = %t, want %t", tc.plan, tc.state, got, tc.want)
		}
	}
}

func TestSyncDevicesSkipsUnchangedTPMVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		t.Errorf("unchanged TPM version made a request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &DeviceApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*deviceResourceModel{tpmDevice("2")}
	state := []*deviceResourceModel{tpmDevice("2")}
	if err := api.syncDevices(t.Context(), &plan, &state, types.Int32Value(70)); err != nil {
		t.Fatal(err)
	}
}

func writeTestBody(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func tpmDevice(version string) *deviceResourceModel {
	device := configuredTPM("tpm")
	device.Key = types.StringValue("3")
	device.DeviceTPMSettingsModel.Key = types.Int32Value(9)
	device.DeviceTPMSettingsModel.Version = NewTPMVersionValue(version)
	return device
}
