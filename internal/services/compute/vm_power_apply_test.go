package compute

import (
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

// The powerstate column is not a reliable running flag. These tests return
// a column that disagrees with machine#status#running.

func TestUpdatePowersOnWhenMachineIsStopped(t *testing.T) {
	shortenPowerWaits(t)
	// Column still says on after the UI shutdown. Apply must power on.
	got, actions := runVMPowerApply(t, vmPowerApplyCase{
		plan:         types.BoolValue(true),
		column:       true,
		running:      false,
		columnAfter:  true,
		runningAfter: true,
	})
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweron"`) || strings.Contains(actions[0], `"kill"`) {
		t.Fatalf("actions = %#v, want one poweron", actions)
	}
	if got.IsNull() || !got.ValueBool() {
		t.Fatalf("powerstate after apply = %#v, want true", got)
	}
}

func TestUpdatePowersOffWhenMachineIsRunning(t *testing.T) {
	shortenPowerWaits(t)
	// Column stays true after the guest stops. State must still be false.
	got, actions := runVMPowerApply(t, vmPowerApplyCase{
		plan:         types.BoolValue(false),
		column:       true,
		running:      true,
		columnAfter:  true,
		runningAfter: false,
	})
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweroff"`) || strings.Contains(actions[0], `"kill"`) {
		t.Fatalf("actions = %#v, want one poweroff and no kill", actions)
	}
	if got.IsNull() || got.ValueBool() {
		t.Fatalf("powerstate after apply = %#v, want false", got)
	}
}

func TestUpdateLeavesARunningVMWhenPlanIsOn(t *testing.T) {
	shortenPowerWaits(t)
	// Column says off, machine is running, plan wants on. Do not power-cycle.
	got, actions := runVMPowerApply(t, vmPowerApplyCase{
		plan:         types.BoolValue(true),
		column:       false,
		running:      true,
		columnAfter:  false,
		runningAfter: true,
	})
	if len(actions) != 0 {
		t.Fatalf("actions = %#v, want no power action", actions)
	}
	if got.IsNull() || !got.ValueBool() {
		t.Fatalf("powerstate after apply = %#v, want true", got)
	}
}

func TestUpdateOmitsPowerWhenPlanIsNull(t *testing.T) {
	shortenPowerWaits(t)
	got, actions := runVMPowerApply(t, vmPowerApplyCase{
		plan:         types.BoolNull(),
		column:       true,
		running:      false,
		columnAfter:  true,
		runningAfter: false,
	})
	if len(actions) != 0 {
		t.Fatalf("actions = %#v, want no power action when powerstate is omitted", actions)
	}
	if got.IsNull() || got.ValueBool() {
		t.Fatalf("powerstate after apply = %#v, want false from the machine", got)
	}
}

func TestUpdatePowerOnFailsWhenMachineStaysStopped(t *testing.T) {
	shortenPowerWaits(t)
	orig := vmPowerWaitTimeout
	t.Cleanup(func() { vmPowerWaitTimeout = orig })
	vmPowerWaitTimeout = 0

	_, actions := runVMPowerApply(t, vmPowerApplyCase{
		plan:         types.BoolValue(true),
		column:       false,
		running:      false,
		columnAfter:  false,
		runningAfter: false,
		wantErr:      "running",
	})
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweron"`) {
		t.Fatalf("actions = %#v, want one poweron before the timeout", actions)
	}
}

type vmPowerApplyCase struct {
	plan         types.Bool
	column       bool
	running      bool
	columnAfter  bool
	runningAfter bool
	wantErr      string
}

func runVMPowerApply(t *testing.T, tc vmPowerApplyCase) (types.Bool, []string) {
	t.Helper()

	var mu sync.Mutex
	var actions []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()

		column := tc.column
		running := tc.running
		if len(actions) > 0 {
			column = tc.columnAfter
			running = tc.runningAfter
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			status := "stopped"
			if running {
				status = "running"
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"$key":7,"machine":1,"name":"web","cpu_cores":1,"ram":512,"enabled":true,"powerstate":` + boolJSON(column) + `,"running":` + boolJSON(running) + `,"status":"` + status + `"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			actions = append(actions, string(body))
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
		PowerState:    types.BoolValue(tc.column),
		GuestAgentIPs: types.ListNull(types.StringType),
	}
	planModel := stateModel
	planModel.PowerState = tc.plan

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
	gotActions := append([]string(nil), actions...)
	mu.Unlock()

	if tc.wantErr != "" {
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected update to fail")
		}
		msg := resp.Diagnostics.Errors()[0].Detail()
		if !strings.Contains(msg, tc.wantErr) {
			t.Fatalf("error %q missing %q", msg, tc.wantErr)
		}
		return types.BoolNull(), gotActions
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("update: %v", resp.Diagnostics)
	}

	var got VMResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("updated state: %v", diags)
	}
	return got.PowerState, gotActions
}
