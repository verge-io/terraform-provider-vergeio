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

func TestSyncDisksRenamePutsInPlace(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_drives/46":
			if !strings.Contains(string(body), `"name":"data2"`) {
				t.Errorf("PUT body = %s, want name data2", body)
			}
			if strings.Contains(string(body), "46") {
				t.Errorf("PUT body sent the drive key: %s", body)
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"data2","disksize":1073741824,"interface":"virtio","enabled":true,"orderid":0}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Key:       types.StringValue("46"),
		Name:      types.StringValue("data2"),
		Interface: types.StringValue("virtio"),
		Enabled:   types.BoolValue(true),
		DiskSize:  types.Float64Value(1),
	}}
	state := []*diskResourceModel{{
		Key:       types.StringValue("46"),
		Name:      types.StringValue("data"),
		Interface: types.StringValue("virtio"),
		Enabled:   types.BoolValue(true),
		DiskSize:  types.Float64Value(1),
		Media:     types.StringValue("disk"),
	}}
	// The plan keeps media from state. A rename must not look like a media change.
	plan[0].Media = state[0].Media

	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if len(state) != 1 || state[0].Key.ValueString() != "46" || state[0].Name.ValueString() != "data2" {
		t.Fatalf("state after rename = key %q name %q", state[0].Key.ValueString(), state[0].Name.ValueString())
	}
	if len(calls) != 2 || calls[0] != "PUT /api/v4/machine_drives/46" || calls[1] != "GET /api/v4/machine_drives/46" {
		t.Fatalf("calls = %#v, want PUT then GET of drive 46", calls)
	}
}

func TestSyncDisksMediaChangeDoesNotRecreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Key:   types.StringValue("46"),
		Name:  types.StringValue("data"),
		Media: types.StringValue("cdrom"),
	}}
	state := []*diskResourceModel{{
		Key:   types.StringValue("46"),
		Name:  types.StringValue("data"),
		Media: types.StringValue("disk"),
	}}
	err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7"))
	if err == nil {
		t.Fatal("expected media change to be refused")
	}
	if state[0].Key.ValueString() != "46" {
		t.Fatalf("drive key changed to %q", state[0].Key.ValueString())
	}
}

func TestSyncDisksUnrelatedVMUpdateDoesNotRewrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	stateDrive := &diskResourceModel{
		Key:                 types.StringValue("48"),
		Machine:             types.Int32Value(68),
		Name:                types.StringValue("data2"),
		Description:         types.StringValue("data"),
		Interface:           types.StringValue("ide"),
		Media:               types.StringValue("disk"),
		DiskSize:            types.Float64Value(1),
		PreferredTier:       types.StringValue("4"),
		Enabled:             types.BoolValue(true),
		ReadOnly:            types.BoolValue(false),
		Serial:              types.StringValue("abc"),
		Asset:               types.StringValue("48"),
		OrderId:             types.Int32Value(1),
		PreserveDriveFormat: types.BoolValue(true),
	}
	// Configured fields stay known and equal. Computed fields are unknown,
	// which is what an update of an unrelated VM attribute produces.
	planDrive := &diskResourceModel{
		Key:                 stateDrive.Key,
		Name:                stateDrive.Name,
		Interface:           stateDrive.Interface,
		Media:               stateDrive.Media,
		DiskSize:            stateDrive.DiskSize,
		Description:         types.StringUnknown(),
		PreferredTier:       types.StringUnknown(),
		Enabled:             types.BoolUnknown(),
		ReadOnly:            types.BoolUnknown(),
		Serial:              types.StringUnknown(),
		Asset:               types.StringUnknown(),
		OrderId:             types.Int32Unknown(),
		PreserveDriveFormat: types.BoolUnknown(),
		Machine:             types.Int32Unknown(),
	}
	plan := []*diskResourceModel{planDrive}
	state := []*diskResourceModel{stateDrive}

	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(68), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if state[0].Key.ValueString() != "48" || state[0].Interface.ValueString() != "ide" || state[0].DiskSize.ValueFloat64() != 1 {
		t.Fatalf("untouched drive state changed: key %q interface %q size %v", state[0].Key.ValueString(), state[0].Interface.ValueString(), state[0].DiskSize.ValueFloat64())
	}
}

func TestSyncDisksDescriptionChangeOmitsDiskSize(t *testing.T) {
	var putBody string
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_drives/49":
			putBody = string(body)
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/49":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":68,"name":"os","description":"note","disksize":1073741824,"interface":"virtio","enabled":true}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	untouched := &diskResourceModel{
		Key:       types.StringValue("48"),
		Name:      types.StringValue("data2"),
		Interface: types.StringValue("ide"),
		Media:     types.StringValue("disk"),
		DiskSize:  types.Float64Value(1),
		Enabled:   types.BoolValue(true),
		OrderId:   types.Int32Value(1),
		Asset:     types.StringValue("48"),
	}
	untouchedPlan := &diskResourceModel{
		Key:         untouched.Key,
		Name:        untouched.Name,
		Interface:   untouched.Interface,
		Media:       untouched.Media,
		DiskSize:    types.Float64Unknown(),
		Description: types.StringUnknown(),
		Enabled:     types.BoolUnknown(),
		OrderId:     types.Int32Unknown(),
		Asset:       types.StringUnknown(),
	}
	changedState := &diskResourceModel{
		Key:         types.StringValue("49"),
		Name:        types.StringValue("os"),
		Interface:   types.StringValue("virtio"),
		Media:       types.StringValue("disk"),
		DiskSize:    types.Float64Value(1),
		Description: types.StringValue("before"),
		Enabled:     types.BoolValue(true),
	}
	changedPlan := &diskResourceModel{
		Key:         changedState.Key,
		Name:        changedState.Name,
		Interface:   types.StringUnknown(),
		Media:       changedState.Media,
		DiskSize:    types.Float64Value(1),
		Description: types.StringValue("note"),
		Enabled:     types.BoolUnknown(),
	}
	plan := []*diskResourceModel{untouchedPlan, changedPlan}
	state := []*diskResourceModel{untouched, changedState}

	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(68), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(putBody, "disksize") {
		t.Errorf("description change sent disksize: %s", putBody)
	}
	if !strings.Contains(putBody, `"description":"note"`) {
		t.Errorf("PUT body = %s, want description note", putBody)
	}
	if len(calls) != 2 || calls[0] != "PUT /api/v4/machine_drives/49" || calls[1] != "GET /api/v4/machine_drives/49" {
		t.Fatalf("calls = %#v, want PUT and GET of the changed drive only", calls)
	}
}

func TestSyncDisksNameFallbackUpdatesExistingKey(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_drives/11" {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/11" {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"name":"os","description":"next"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Name:        types.StringValue("os"),
		Description: types.StringValue("next"),
	}}
	state := []*diskResourceModel{{
		Key:         types.StringValue("11"),
		Name:        types.StringValue("os"),
		Description: types.StringValue("prev"),
	}}
	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "PUT /api/v4/machine_drives/11" {
		t.Fatalf("calls = %#v, want PUT of the existing drive", calls)
	}
	if state[0].Key.ValueString() != "11" {
		t.Fatalf("key = %q, want 11", state[0].Key.ValueString())
	}
}

func TestSyncDisksDeletesLastWhenPlanIsEmpty(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"version":"26.0.0"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"powerstate":"offline"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	var plan []*diskResourceModel
	state := []*diskResourceModel{{
		Key:       types.StringValue("46"),
		Name:      types.StringValue("os"),
		Interface: types.StringValue("virtio"),
		Media:     types.StringValue("disk"),
	}}

	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if len(state) != 0 {
		t.Fatalf("state after removing the last drive = %d disks, want 0", len(state))
	}
	var driveCalls []string
	for _, call := range calls {
		if call != "GET /version.json" {
			driveCalls = append(driveCalls, call)
		}
	}
	if len(driveCalls) != 2 || driveCalls[0] != "GET /api/v4/machine_drives/46" || driveCalls[1] != "DELETE /api/v4/machine_drives/46" {
		t.Fatalf("calls = %#v, want power-state GET then DELETE of drive 46", driveCalls)
	}
}

func TestSyncDisksHotplugsCreatedDriveOnRunningVM(t *testing.T) {
	const driveJSON = `{"machine":1,"name":"data","disksize":1073741824,"interface":"virtio","enabled":true}`
	var calls []string
	var hotplugBody string
	statusReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"version":"26.0.0"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"powerstate":true}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives":
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":"99"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/99" && strings.Contains(r.URL.RawQuery, "powerState"):
			statusReads++
			payload := `{"powerstate":"offline"}`
			if statusReads > 1 {
				payload = `{"powerstate":"online"}`
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(payload)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/99":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(driveJSON)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			hotplugBody = string(body)
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.RequestURI(), body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Name:      types.StringValue("data"),
		Interface: types.StringValue("virtio"),
		Media:     types.StringValue("disk"),
		DiskSize:  types.Float64Value(1),
		Enabled:   types.BoolValue(true),
	}}
	var state []*diskResourceModel

	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if len(state) != 1 || state[0].Key.ValueString() != "99" {
		t.Fatalf("state after hotplug = %#v", state)
	}
	if !strings.Contains(hotplugBody, `"action":"hotplugdrive"`) || !strings.Contains(hotplugBody, `"device":"99"`) {
		t.Fatalf("hotplug body = %s, want hotplugdrive for drive 99", hotplugBody)
	}
	if strings.Contains(hotplugBody, "unplug") {
		t.Fatalf("attach hotplug included unplug: %s", hotplugBody)
	}
	if statusReads < 2 {
		t.Fatalf("status reads = %d, want a poll while the drive is still offline", statusReads)
	}
	if !callBefore(calls, "POST /api/v4/machine_drives", "POST /api/v4/vm_actions") {
		t.Fatalf("calls = %#v, want the drive created before hotplug", calls)
	}
}

func TestSyncDisksCreateOnStoppedVMDoesNotHotplug(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"powerstate":false}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives":
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":"99"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/99":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"data","disksize":1073741824,"interface":"virtio","enabled":true}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Name:     types.StringValue("data"),
		Media:    types.StringValue("disk"),
		DiskSize: types.Float64Value(1),
		Enabled:  types.BoolValue(true),
	}}
	var state []*diskResourceModel

	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if call == "POST /api/v4/vm_actions" {
			t.Fatalf("stopped VM hotplugged a drive: %#v", calls)
		}
	}
}

func TestSyncDisksSkipsHotplugForDisabledDrive(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"powerstate":true}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives":
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":"99"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/99":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"data","interface":"virtio","enabled":false}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Name:    types.StringValue("data"),
		Enabled: types.BoolValue(false),
	}}
	var state []*diskResourceModel

	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if call == "POST /api/v4/vm_actions" {
			t.Fatalf("disabled drive was hotplugged: %#v", calls)
		}
	}
}

func TestSyncDisksHotplugRefusalRequiresPowerCycle(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"version":"26.0.0"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"powerstate":true}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives":
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":"99"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/99":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"data","interface":"ide","enabled":true}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte(`{"err":"cannot hotplug ide drive"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Name:      types.StringValue("data"),
		Interface: types.StringValue("ide"),
		Enabled:   types.BoolValue(true),
	}}
	var state []*diskResourceModel

	err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7"))
	if err == nil {
		t.Fatal("expected hotplug refusal to fail the sync")
	}
	if !strings.Contains(err.Error(), "power cycle") || !strings.Contains(err.Error(), "cannot hotplug ide drive") {
		t.Fatalf("error = %v, want a power-cycle message that includes the API refusal", err)
	}
	if !callBefore(calls, "POST /api/v4/machine_drives", "POST /api/v4/vm_actions") {
		t.Fatalf("calls = %#v, want the drive created before the refused hotplug", calls)
	}
}

func TestHotplugDriveUnplugSetsFlag(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		body = string(payload)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/vm_actions" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	api := &VMApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	if err := api.hotplugDrive(t.Context(), "46", types.StringValue("7"), true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"action":"hotplugdrive"`) || !strings.Contains(body, `"unplug":true`) || !strings.Contains(body, `"device":"46"`) {
		t.Fatalf("unplug body = %s, want hotplugdrive with unplug true", body)
	}
}

func callBefore(calls []string, earlier, later string) bool {
	earlierAt, laterAt := -1, -1
	for i, call := range calls {
		if call == earlier && earlierAt < 0 {
			earlierAt = i
		}
		if call == later && laterAt < 0 {
			laterAt = i
		}
	}
	return earlierAt >= 0 && laterAt >= 0 && earlierAt < laterAt
}
