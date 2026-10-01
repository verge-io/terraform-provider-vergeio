// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

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

func TestTPMVersionPlanUsesStoredValue(t *testing.T) {
	ctx := t.Context()
	vmSchema := vmSchema(t)
	state := tfsdk.State{
		Schema: vmSchema,
		Raw:    tftypes.NewValue(vmSchema.Type().TerraformType(ctx), nil),
	}
	for _, tc := range []struct {
		config string
		want   string
	}{
		{config: "2.0", want: "2"},
		{config: "1.2", want: "1"},
		{config: "2", want: "2"},
		{config: "1", want: "1"},
		{config: " 1.2 ", want: "1"},
	} {
		device := configuredTPM("tpm")
		device.DeviceTPMSettingsModel.Version = types.StringValue(tc.config)
		config := vmValue(t, vmSchema, []*deviceResourceModel{device})
		assertDeviceString(t, ctx, vmSchema, config, state, 0, "tpm_settings.version", tc.want)
	}

	unknown := configuredTPM("tpm")
	unknown.DeviceTPMSettingsModel.Version = types.StringNull()
	config := vmValue(t, vmSchema, []*deviceResourceModel{unknown})
	assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "tpm_settings.version")
}

func TestTPMVersionPlanKeepsOmittedValue(t *testing.T) {
	resp := &planmodifier.StringResponse{PlanValue: types.StringValue("2")}
	tpmVersionModifier{}.PlanModifyString(t.Context(), planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		PlanValue:   types.StringValue("2"),
	}, resp)
	if resp.PlanValue.IsNull() || resp.PlanValue.IsUnknown() || resp.PlanValue.ValueString() != "2" {
		t.Fatalf("omitted version plan = %s, want prior stored value 2", resp.PlanValue)
	}

	unknown := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
	tpmVersionModifier{}.PlanModifyString(t.Context(), planmodifier.StringRequest{
		ConfigValue: types.StringUnknown(),
		PlanValue:   types.StringUnknown(),
	}, unknown)
	if unknown.PlanValue.IsNull() || !unknown.PlanValue.IsUnknown() {
		t.Fatalf("unknown version plan = %s, want unknown", unknown.PlanValue)
	}
}

func TestUpdateTPMSettingsSendsStoredVersion(t *testing.T) {
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

		version := types.StringNull()
		if !tc.omit {
			version = types.StringValue(tc.version)
		}
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
		if tc.omit {
			if _, ok := got["version"]; ok {
				t.Fatalf("omitted version was sent: %#v", got)
			}
			continue
		}
		if got["version"] != tc.want {
			t.Fatalf("version %q sent as %#v, want %q", tc.version, got["version"], tc.want)
		}
		if got["model"] != "crb" {
			t.Fatalf("model = %#v, want crb", got["model"])
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

func TestSyncDevicesSendsChangedTPMVersion(t *testing.T) {
	var version any
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
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode TPM update %s: %v", body, err)
			}
			version = payload["version"]
			writeTestBody(t, w, http.StatusOK, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_device_settings_tpm":
			writeTestBody(t, w, http.StatusOK, `[{"$key":9,"machine_device":3,"model":"crb","version":"1"}]`)
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
	if version != "1" {
		t.Fatalf("TPM update version = %#v, want 1", version)
	}
	if got := state[0].DeviceTPMSettingsModel.Version.ValueString(); got != "1" {
		t.Fatalf("state version = %q, want 1", got)
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
	device.DeviceTPMSettingsModel.Version = types.StringValue(version)
	return device
}
