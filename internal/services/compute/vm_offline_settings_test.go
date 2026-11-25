package compute

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestVMUpdateRequiresStoppedVM(t *testing.T) {
	base := func() VMResourceModel {
		return VMResourceModel{
			Id:                   types.StringValue("7"),
			Name:                 types.StringValue("web"),
			Description:          types.StringValue("app"),
			CPUCores:             types.Int32Value(1),
			RAM:                  types.Int32Value(512),
			MachineType:          types.StringValue("q35"),
			Console:              types.StringValue("vnc"),
			Video:                types.StringValue("std"),
			UEFI:                 types.BoolValue(false),
			SecureBoot:           types.BoolValue(false),
			NestedVirtualization: types.BoolValue(false),
			DisableHypervisor:    types.BoolValue(false),
			GuestAgent:           types.BoolValue(true),
			PowerState:           types.BoolValue(true),
		}
	}

	cases := []struct {
		name  string
		field string
		edit  func(*VMResourceModel)
		want  bool
	}{
		{name: "cpu_cores", field: "cpu_cores", want: true, edit: func(m *VMResourceModel) { m.CPUCores = types.Int32Value(2) }},
		{name: "ram", field: "ram", want: true, edit: func(m *VMResourceModel) { m.RAM = types.Int32Value(1024) }},
		{name: "machine_type", field: "machine_type", want: true, edit: func(m *VMResourceModel) { m.MachineType = types.StringValue("pc") }},
		{name: "console", field: "console", want: true, edit: func(m *VMResourceModel) { m.Console = types.StringValue("spice") }},
		{name: "video", field: "video", want: true, edit: func(m *VMResourceModel) { m.Video = types.StringValue("virtio") }},
		{name: "uefi", field: "uefi", want: true, edit: func(m *VMResourceModel) { m.UEFI = types.BoolValue(true) }},
		{name: "secure_boot", field: "secure_boot", want: true, edit: func(m *VMResourceModel) { m.SecureBoot = types.BoolValue(true) }},
		{name: "nested_virtualization", field: "nested_virtualization", want: true, edit: func(m *VMResourceModel) { m.NestedVirtualization = types.BoolValue(true) }},
		{name: "disable_hypervisor", field: "disable_hypervisor", want: true, edit: func(m *VMResourceModel) { m.DisableHypervisor = types.BoolValue(true) }},
		{name: "name", edit: func(m *VMResourceModel) { m.Name = types.StringValue("other") }},
		{name: "description", edit: func(m *VMResourceModel) { m.Description = types.StringValue("notes") }},
		{name: "guest_agent", edit: func(m *VMResourceModel) { m.GuestAgent = types.BoolValue(false) }},
		{name: "powerstate only", edit: func(m *VMResourceModel) { m.PowerState = types.BoolValue(false) }},
		{name: "unchanged"},
	}

	if vmUpdateRequiresStoppedVM(nil, nil) {
		t.Fatal("nil plan should not require a stopped VM")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := base()
			plan := base()
			if tc.edit != nil {
				tc.edit(&plan)
			}
			if got := vmUpdateRequiresStoppedVM(&plan, &state); got != tc.want {
				t.Fatalf("vmUpdateRequiresStoppedVM = %v, want %v", got, tc.want)
			}
			if !tc.want {
				return
			}
			req, _, err := vmUpdateRequest(&plan, &state)
			if err != nil {
				t.Fatal(err)
			}
			body := jsonObject(t, req)
			if _, ok := body[tc.field]; !ok {
				t.Fatalf("update request missing %s: %#v", tc.field, body)
			}
		})
	}
}

func TestUpdatePowersOffBeforeCPUChange(t *testing.T) {
	shortenPowerWaits(t)

	got, events := runOfflineSettingsApply(t, offlineSettingsCase{
		running:   true,
		planPower: types.BoolValue(false),
		edit:      func(m *VMResourceModel) { m.CPUCores = types.Int32Value(2) },
		want:      []string{"poweroff", "put"},
	})
	if got.PowerState.IsNull() || got.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want false", got.PowerState)
	}
	if got.CPUCores.IsNull() || got.CPUCores.ValueInt32() != 2 {
		t.Fatalf("cpu_cores = %#v, want 2", got.CPUCores)
	}
	if eventsContain(events, "poweron") {
		t.Fatalf("events = %#v, want the VM left off", events)
	}
}

func TestUpdatePowersOffBeforeRAMChange(t *testing.T) {
	shortenPowerWaits(t)

	got, events := runOfflineSettingsApply(t, offlineSettingsCase{
		running:   true,
		planPower: types.BoolValue(false),
		edit:      func(m *VMResourceModel) { m.RAM = types.Int32Value(1024) },
		want:      []string{"poweroff", "put"},
	})
	if got.PowerState.IsNull() || got.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want false", got.PowerState)
	}
	if got.RAM.IsNull() || got.RAM.ValueInt32() != 1024 {
		t.Fatalf("ram = %#v, want 1024", got.RAM)
	}
	if eventsContain(events, "poweron") {
		t.Fatalf("events = %#v, want the VM left off", events)
	}
}

func TestUpdateCyclesPowerWhenCPUChangesAndVMStaysOn(t *testing.T) {
	shortenPowerWaits(t)

	got, _ := runOfflineSettingsApply(t, offlineSettingsCase{
		running:   true,
		planPower: types.BoolValue(true),
		edit:      func(m *VMResourceModel) { m.CPUCores = types.Int32Value(4) },
		want:      []string{"poweroff", "put", "poweron"},
	})
	if got.PowerState.IsNull() || !got.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want true", got.PowerState)
	}
	if got.CPUCores.IsNull() || got.CPUCores.ValueInt32() != 4 {
		t.Fatalf("cpu_cores = %#v, want 4", got.CPUCores)
	}
}

func TestUpdateAppliesCPUBeforePowerOnWhenVMIsStopped(t *testing.T) {
	shortenPowerWaits(t)

	got, events := runOfflineSettingsApply(t, offlineSettingsCase{
		running:   false,
		planPower: types.BoolValue(true),
		edit:      func(m *VMResourceModel) { m.CPUCores = types.Int32Value(2) },
		want:      []string{"put", "poweron"},
	})
	if eventsContain(events, "poweroff") {
		t.Fatalf("events = %#v, want no poweroff of a stopped VM", events)
	}
	if got.PowerState.IsNull() || !got.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want true", got.PowerState)
	}
	if got.CPUCores.IsNull() || got.CPUCores.ValueInt32() != 2 {
		t.Fatalf("cpu_cores = %#v, want 2", got.CPUCores)
	}
}

func TestUpdateRenamesBeforePowerOff(t *testing.T) {
	shortenPowerWaits(t)

	got, _ := runOfflineSettingsApply(t, offlineSettingsCase{
		running:   true,
		planPower: types.BoolValue(false),
		edit:      func(m *VMResourceModel) { m.Name = types.StringValue("renamed") },
		want:      []string{"put", "poweroff"},
	})
	if got.Name.IsNull() || got.Name.ValueString() != "renamed" {
		t.Fatalf("name = %#v, want renamed", got.Name)
	}
	if got.PowerState.IsNull() || got.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want false", got.PowerState)
	}
}

func TestUpdateRestoresPowerWhenCPUChangesAndPowerStateIsOmitted(t *testing.T) {
	shortenPowerWaits(t)

	got, _ := runOfflineSettingsApply(t, offlineSettingsCase{
		running:   true,
		planPower: types.BoolNull(),
		edit:      func(m *VMResourceModel) { m.CPUCores = types.Int32Value(2) },
		want:      []string{"poweroff", "put", "poweron"},
	})
	if got.PowerState.IsNull() || !got.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want true", got.PowerState)
	}
	if got.CPUCores.IsNull() || got.CPUCores.ValueInt32() != 2 {
		t.Fatalf("cpu_cores = %#v, want 2", got.CPUCores)
	}
}

func TestUpdateRestoresPowerWhenSettingsWriteFails(t *testing.T) {
	shortenPowerWaits(t)

	for _, planPower := range []types.Bool{types.BoolNull(), types.BoolValue(true)} {
		t.Run(planPower.String(), func(t *testing.T) {
			_, events := runOfflineSettingsApply(t, offlineSettingsCase{
				running:   true,
				planPower: planPower,
				failPut:   true,
				edit:      func(m *VMResourceModel) { m.CPUCores = types.Int32Value(2) },
				want:      []string{"poweroff", "put-failed", "poweron"},
				wantErr:   "settings update failed",
			})
			if eventsContain(events, "poweron-failed") {
				t.Fatalf("events = %#v, want the VM powered back on", events)
			}
		})
	}
}

func TestUpdateReportsPowerOnFailureAfterSettingsWriteFails(t *testing.T) {
	shortenPowerWaits(t)

	runOfflineSettingsApply(t, offlineSettingsCase{
		running:     true,
		planPower:   types.BoolNull(),
		failPut:     true,
		failPowerOn: true,
		edit:        func(m *VMResourceModel) { m.CPUCores = types.Int32Value(2) },
		want:        []string{"poweroff", "put-failed", "poweron-failed"},
		wantErr:     "settings update failed",
		wantErrAlso: "could not be powered back on",
	})
}

func TestUpdateLeavesVMOffWhenSettingsWriteFailsAndPlanIsOff(t *testing.T) {
	shortenPowerWaits(t)

	_, events := runOfflineSettingsApply(t, offlineSettingsCase{
		running:   true,
		planPower: types.BoolValue(false),
		failPut:   true,
		edit:      func(m *VMResourceModel) { m.CPUCores = types.Int32Value(2) },
		want:      []string{"poweroff", "put-failed"},
		wantErr:   "settings update failed",
	})
	if eventsContain(events, "poweron") || eventsContain(events, "poweron-failed") {
		t.Fatalf("events = %#v, want the VM left off", events)
	}
}

func TestUpdateRestoresPowerWhenBootDiskSyncFails(t *testing.T) {
	shortenPowerWaits(t)

	runOfflineSettingsApply(t, offlineSettingsCase{
		running:      true,
		planPower:    types.BoolValue(true),
		bootConflict: true,
		edit:         func(m *VMResourceModel) { m.CPUCores = types.Int32Value(2) },
		want:         []string{"poweroff", "put", "poweron"},
		wantErr:      "changing media or source",
	})
}

func TestUpdateDoesNotWriteCPUWhenPowerOffTimesOut(t *testing.T) {
	shortenPowerWaits(t)

	_, events := runOfflineSettingsApply(t, offlineSettingsCase{
		running:     true,
		stayRunning: true,
		planPower:   types.BoolValue(false),
		edit: func(m *VMResourceModel) {
			m.CPUCores = types.Int32Value(2)
			m.Timeouts = &vmTimeoutsModel{Update: types.StringValue("0s")}
		},
		want:    []string{"poweroff"},
		wantErr: "did not stop",
	})
	if eventsContain(events, "put") || eventsContain(events, "put-rejected") {
		t.Fatalf("events = %#v, want no settings write after a failed poweroff", events)
	}
}

type offlineSettingsCase struct {
	running      bool
	stayRunning  bool
	failPut      bool
	failPowerOn  bool
	bootConflict bool
	planPower    types.Bool
	edit         func(*VMResourceModel)
	want         []string
	wantErr      string
	wantErrAlso  string
}

func runOfflineSettingsApply(t *testing.T, tc offlineSettingsCase) (VMResourceModel, []string) {
	t.Helper()

	var mu sync.Mutex
	var events []string
	running := tc.running
	cpu := 1
	ram := 512
	name := "web"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read", http.StatusInternalServerError)
			return
		}

		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vms/7":
			if running && offlineSettingsBody(string(body)) {
				events = append(events, "put-rejected")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"err":"CPU cores cannot be changed while the VM is running"}`))
				return
			}
			if tc.failPut {
				events = append(events, "put-failed")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"err":"settings update failed"}`))
				return
			}
			events = append(events, "put")
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("put body: %v", err)
			}
			if v, ok := payload["cpu_cores"].(float64); ok {
				cpu = int(v)
			}
			if v, ok := payload["ram"].(float64); ok {
				ram = int(v)
			}
			if v, ok := payload["name"].(string); ok {
				name = v
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			status := "stopped"
			if running {
				status = "running"
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"$key":7,"machine":1,"name":%q,"cpu_cores":%d,"ram":%d,"enabled":true,"powerstate":%s,"running":%s,"status":%q}`, name, cpu, ram, boolJSON(running), boolJSON(running), status)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			switch {
			case strings.Contains(string(body), `"action":"poweroff"`):
				events = append(events, "poweroff")
				if !tc.stayRunning {
					running = false
				}
			case strings.Contains(string(body), `"action":"poweron"`):
				if tc.failPowerOn {
					events = append(events, "poweron-failed")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"err":"power on failed"}`))
					return
				}
				events = append(events, "poweron")
				running = true
			case strings.Contains(string(body), `"action":"kill"`):
				events = append(events, "kill")
				t.Errorf("kill was posted: %s", body)
			default:
				t.Errorf("unexpected vm action %s", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/cloudinit_files":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.RequestURI(), body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	vergeClient := vergeio.NewClient(server.URL, "user", "pass", true)
	vmResource := &VMResource{
		vmApi:     mustAPI(NewVMApi(vergeClient)),
		deviceApi: mustAPI(NewDeviceApi(vergeClient)),
	}

	schemaResp := &fwresource.SchemaResponse{}
	vmResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	stateModel := VMResourceModel{
		Id:            types.StringValue("7"),
		Machine:       types.Int32Value(1),
		Name:          types.StringValue("web"),
		CPUCores:      types.Int32Value(1),
		RAM:           types.Int32Value(512),
		PowerState:    types.BoolValue(tc.running),
		GuestAgentIPs: types.ListNull(types.StringType),
	}
	if tc.bootConflict {
		stateModel.BootDisk = &bootDiskModel{
			Key:   types.StringValue("46"),
			Name:  types.StringValue("os"),
			Media: types.StringValue("disk"),
		}
	}
	planModel := stateModel
	planModel.PowerState = tc.planPower
	if tc.bootConflict && stateModel.BootDisk != nil {
		copied := *stateModel.BootDisk
		copied.Media = types.StringValue("import")
		planModel.BootDisk = &copied
	}
	if tc.edit != nil {
		tc.edit(&planModel)
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("plan: %v", diags)
	}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("state: %v", diags)
	}

	resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	vmResource.Update(ctx, fwresource.UpdateRequest{
		Plan:   plan,
		State:  state,
		Config: tfsdk.Config(plan),
	}, resp)

	mu.Lock()
	gotEvents := append([]string(nil), events...)
	mu.Unlock()

	if tc.wantErr != "" || tc.wantErrAlso != "" {
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected update to fail")
		}
		var text strings.Builder
		for _, d := range resp.Diagnostics.Errors() {
			text.WriteString(d.Summary())
			text.WriteByte('\n')
			text.WriteString(d.Detail())
			text.WriteByte('\n')
		}
		msg := text.String()
		for _, fragment := range []string{tc.wantErr, tc.wantErrAlso} {
			if fragment == "" {
				continue
			}
			if !strings.Contains(msg, fragment) {
				t.Fatalf("errors %q missing %q", msg, fragment)
			}
		}
		if tc.wantErrAlso != "" && len(resp.Diagnostics.Errors()) < 2 {
			t.Fatalf("errors = %q, want the original error and the power restore error", msg)
		}
		if strings.Join(gotEvents, ",") != strings.Join(tc.want, ",") {
			t.Fatalf("events = %#v, want %#v", gotEvents, tc.want)
		}
		return VMResourceModel{}, gotEvents
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("update: %v\nevents: %#v", resp.Diagnostics, gotEvents)
	}
	if strings.Join(gotEvents, ",") != strings.Join(tc.want, ",") {
		t.Fatalf("events = %#v, want %#v", gotEvents, tc.want)
	}

	var got VMResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("updated state: %v", diags)
	}
	return got, gotEvents
}

func offlineSettingsBody(body string) bool {
	for _, field := range []string{
		`"cpu_cores"`,
		`"ram"`,
		`"machine_type"`,
		`"console"`,
		`"video"`,
		`"uefi"`,
		`"secure_boot"`,
		`"nested_virtualization"`,
		`"disable_hypervisor"`,
	} {
		if strings.Contains(body, field) {
			return true
		}
	}
	return false
}

func eventsContain(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}
