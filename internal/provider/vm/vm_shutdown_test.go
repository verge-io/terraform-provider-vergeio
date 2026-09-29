package vm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"
	vergeos "github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

func TestGracefulShutdownPowersOffOnceAndWaits(t *testing.T) {
	t.Parallel()

	var actions []string
	reads := 0
	err := gracefulShutdown(t.Context(), "7", "web", time.Second, 0, false,
		func(_ context.Context, action string) error {
			actions = append(actions, action)
			return nil
		},
		func(context.Context) (bool, string, error) {
			reads++
			if reads == 1 {
				return true, "running", nil
			}
			if reads < 4 {
				return true, "stopping", nil
			}
			return false, "stopped", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if reads != 4 {
		t.Fatalf("reads = %d, want polls until stopped", reads)
	}
	if len(actions) != 1 || actions[0] != vmActionPowerOff {
		t.Fatalf("actions = %#v, want one poweroff", actions)
	}
}

func TestGracefulShutdownLeavesAStoppedVMAlone(t *testing.T) {
	t.Parallel()

	err := gracefulShutdown(t.Context(), "7", "web", 0, time.Hour, true,
		func(context.Context, string) error {
			t.Fatal("stopped VM should not be sent an action")
			return nil
		},
		func(context.Context) (bool, string, error) {
			return false, "stopped", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGracefulShutdownTimeoutNamesVMAndStatus(t *testing.T) {
	t.Parallel()

	var actions []string
	err := gracefulShutdown(t.Context(), "7", "web", 0, time.Hour, false,
		func(_ context.Context, action string) error {
			actions = append(actions, action)
			return nil
		},
		func(context.Context) (bool, string, error) {
			return true, "stopping", nil
		},
	)
	if err == nil {
		t.Fatal("expected a timeout")
	}
	msg := err.Error()
	for _, want := range []string{"web", "7", "stopping"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if strings.Contains(strings.ToLower(msg), "kill") {
		t.Errorf("timeout error should not claim a kill: %s", msg)
	}
	if len(actions) != 1 || actions[0] != vmActionPowerOff {
		t.Fatalf("actions = %#v, want one poweroff and no kill", actions)
	}
}

func TestGracefulShutdownForceKillsAfterTimeout(t *testing.T) {
	t.Parallel()

	var actions []string
	reads := 0
	err := gracefulShutdown(t.Context(), "7", "web", 0, time.Hour, true,
		func(_ context.Context, action string) error {
			actions = append(actions, action)
			return nil
		},
		func(context.Context) (bool, string, error) {
			reads++
			// Pre-read, the poweroff poll, then the kill poll.
			if reads < 3 {
				return true, "running", nil
			}
			return false, "stopped", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0] != vmActionPowerOff || actions[1] != vmActionKill {
		t.Fatalf("actions = %#v, want poweroff then one kill", actions)
	}
}

func TestGracefulShutdownForceTimeoutNamesVM(t *testing.T) {
	t.Parallel()

	var actions []string
	err := gracefulShutdown(t.Context(), "7", "db", 0, time.Hour, true,
		func(_ context.Context, action string) error {
			actions = append(actions, action)
			return nil
		},
		func(context.Context) (bool, string, error) {
			return true, "running", nil
		},
	)
	if err == nil {
		t.Fatal("expected an error after kill")
	}
	msg := err.Error()
	for _, want := range []string{"db", "7", "running", "kill"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if len(actions) != 2 || actions[1] != vmActionKill {
		t.Fatalf("actions = %#v, want one kill after poweroff", actions)
	}
}

func TestGracefulShutdownDoesNotKillWhenPoweroffFails(t *testing.T) {
	t.Parallel()

	var actions []string
	err := gracefulShutdown(t.Context(), "7", "web", time.Minute, 0, true,
		func(_ context.Context, action string) error {
			actions = append(actions, action)
			return errors.New("power action refused")
		},
		func(context.Context) (bool, string, error) {
			return true, "running", nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "poweroff") {
		t.Fatalf("err = %v, want a poweroff failure", err)
	}
	if len(actions) != 1 || actions[0] != vmActionPowerOff {
		t.Fatalf("actions = %#v, want poweroff only", actions)
	}
}

func TestGracefulShutdownCancelDoesNotKill(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var actions []string
	err := gracefulShutdown(ctx, "7", "web", time.Minute, time.Hour, true,
		func(_ context.Context, action string) error {
			actions = append(actions, action)
			return nil
		},
		func(context.Context) (bool, string, error) {
			return true, "running", nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context canceled", err)
	}
	if len(actions) != 0 {
		t.Fatalf("actions = %#v, want no action after cancel", actions)
	}
}

func TestPowerStatusAcceptsAliasAndFullVM(t *testing.T) {
	t.Parallel()

	running := true
	stopped := false
	gotRun, gotStatus, ok := (vmPowerStatusBody{Status: "stopping", Running: &running}).powerStatus()
	if !ok || !gotRun || gotStatus != "stopping" {
		t.Fatalf("alias status = %v %q ok=%v", gotRun, gotStatus, ok)
	}
	gotRun, gotStatus, ok = (vmPowerStatusBody{PowerState: &stopped, Machine: []byte(`1`)}).powerStatus()
	if !ok || gotRun || gotStatus != "stopped" {
		t.Fatalf("full VM powerstate = %v %q ok=%v", gotRun, gotStatus, ok)
	}
	gotRun, gotStatus, ok = (vmPowerStatusBody{Machine: []byte(`{"status":{"status":"running","running":true}}`)}).powerStatus()
	if !ok || !gotRun || gotStatus != "running" {
		t.Fatalf("nested status = %v %q ok=%v", gotRun, gotStatus, ok)
	}
	if _, _, ok = (vmPowerStatusBody{}).powerStatus(); ok {
		t.Fatal("empty payload should not report a power state")
	}
}

func TestShutdownTimeoutFromPlan(t *testing.T) {
	t.Parallel()

	got, err := shutdownTimeoutFromPlan(&VMResourceModel{})
	if err != nil {
		t.Fatal(err)
	}
	if got != gracefulShutdownTimeout {
		t.Fatalf("default timeout = %s, want %s", got, gracefulShutdownTimeout)
	}

	got, err = shutdownTimeoutFromPlan(&VMResourceModel{
		Timeouts: &vmTimeoutsModel{Update: types.StringValue("90s")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 90*time.Second {
		t.Fatalf("configured timeout = %s, want 90s", got)
	}
}

func TestDeleteTimeoutFromState(t *testing.T) {
	t.Parallel()

	got, err := deleteTimeoutFromState(&VMResourceModel{})
	if err != nil {
		t.Fatal(err)
	}
	if got != gracefulShutdownTimeout {
		t.Fatalf("default delete timeout = %s, want %s", got, gracefulShutdownTimeout)
	}

	got, err = deleteTimeoutFromState(&VMResourceModel{
		Timeouts: &vmTimeoutsModel{Delete: types.StringValue("45s")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 45*time.Second {
		t.Fatalf("configured delete timeout = %s, want 45s", got)
	}

	if _, err = deleteTimeoutFromState(&VMResourceModel{
		Timeouts: &vmTimeoutsModel{Delete: types.StringValue("-1s")},
	}); err == nil || !strings.Contains(err.Error(), "timeouts.delete") {
		t.Fatalf("err = %v, want a timeouts.delete error", err)
	}
}

func TestShutdownOnDestroyMode(t *testing.T) {
	t.Parallel()

	got, err := shutdownOnDestroyMode(&VMResourceModel{})
	if err != nil {
		t.Fatal(err)
	}
	if got != shutdownOnDestroyGracefulThenKill {
		t.Fatalf("unset mode = %q, want %s", got, shutdownOnDestroyGracefulThenKill)
	}

	for _, mode := range []string{shutdownOnDestroyGracefulThenKill, shutdownOnDestroyGraceful, shutdownOnDestroyKill} {
		got, err = shutdownOnDestroyMode(&VMResourceModel{ShutdownOnDestroy: types.StringValue(mode)})
		if err != nil {
			t.Fatal(err)
		}
		if got != mode {
			t.Fatalf("mode = %q, want %q", got, mode)
		}
	}

	if _, err = shutdownOnDestroyMode(&VMResourceModel{ShutdownOnDestroy: types.StringValue("reboot")}); err == nil {
		t.Fatal("expected an error for an unknown shutdown_on_destroy")
	}
}

func TestApplyVMDefaultsForcePowerOff(t *testing.T) {
	t.Parallel()

	data := &VMResourceModel{}
	applyVM(data, &vergeos.VM{Name: "web"})
	if data.ForcePowerOff.IsNull() || data.ForcePowerOff.IsUnknown() || data.ForcePowerOff.ValueBool() {
		t.Fatalf("force_power_off = %#v, want false after import", data.ForcePowerOff)
	}

	data.ForcePowerOff = types.BoolValue(true)
	applyVM(data, &vergeos.VM{Name: "web"})
	if data.ForcePowerOff.IsNull() || !data.ForcePowerOff.ValueBool() {
		t.Fatalf("force_power_off = %#v, want an explicit true kept", data.ForcePowerOff)
	}
}

func TestApplyVMDefaultsShutdownOnDestroy(t *testing.T) {
	t.Parallel()

	data := &VMResourceModel{}
	applyVM(data, &vergeos.VM{Name: "web"})
	if data.ShutdownOnDestroy.ValueString() != shutdownOnDestroyGracefulThenKill {
		t.Fatalf("shutdown_on_destroy = %#v, want %s after import", data.ShutdownOnDestroy, shutdownOnDestroyGracefulThenKill)
	}

	data.ShutdownOnDestroy = types.StringValue(shutdownOnDestroyKill)
	applyVM(data, &vergeos.VM{Name: "web"})
	if data.ShutdownOnDestroy.ValueString() != shutdownOnDestroyKill {
		t.Fatalf("shutdown_on_destroy = %#v, want an explicit kill kept", data.ShutdownOnDestroy)
	}
}

func TestForcePowerOffAndTimeoutsSchema(t *testing.T) {
	vmResource := NewVMResource()
	resp := &fwresource.SchemaResponse{}
	vmResource.Schema(t.Context(), fwresource.SchemaRequest{}, resp)

	powerAttr, ok := resp.Schema.Attributes["powerstate"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("powerstate should be a bool attribute")
	}
	if !strings.Contains(strings.ToLower(powerAttr.MarkdownDescription), "poweroff") {
		t.Fatalf("powerstate description %q should say it sends poweroff", powerAttr.MarkdownDescription)
	}

	forceAttr, ok := resp.Schema.Attributes["force_power_off"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("force_power_off should be a bool attribute")
	}
	if !forceAttr.Optional || !forceAttr.Computed {
		t.Fatal("force_power_off should be optional and computed")
	}
	if forceAttr.Default == nil {
		t.Fatal("force_power_off should default to false")
	}
	defaultResp := &defaults.BoolResponse{}
	forceAttr.Default.DefaultBool(t.Context(), defaults.BoolRequest{}, defaultResp)
	if defaultResp.Diagnostics.HasError() || defaultResp.PlanValue.IsNull() || defaultResp.PlanValue.ValueBool() {
		t.Fatalf("force_power_off default = %#v, diagnostics %v", defaultResp.PlanValue, defaultResp.Diagnostics)
	}

	timeoutsBlock, ok := resp.Schema.Blocks["timeouts"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("timeouts should be a single nested block")
	}
	updateAttr, ok := timeoutsBlock.Attributes["update"].(schema.StringAttribute)
	if !ok || !updateAttr.Optional {
		t.Fatal("timeouts.update should be an optional string")
	}
	deleteAttr, ok := timeoutsBlock.Attributes["delete"].(schema.StringAttribute)
	if !ok || !deleteAttr.Optional {
		t.Fatal("timeouts.delete should be an optional string")
	}
	deleteDesc := strings.ToLower(deleteAttr.MarkdownDescription)
	if !strings.Contains(deleteDesc, "poweroff") || strings.Contains(deleteDesc, "does not use") {
		t.Fatalf("timeouts.delete description %q should say destroy waits for poweroff", deleteAttr.MarkdownDescription)
	}

	shutdownAttr, ok := resp.Schema.Attributes["shutdown_on_destroy"].(schema.StringAttribute)
	if !ok {
		t.Fatal("shutdown_on_destroy should be a string attribute")
	}
	if !shutdownAttr.Optional || !shutdownAttr.Computed || shutdownAttr.Default == nil {
		t.Fatal("shutdown_on_destroy should be optional, computed, and defaulted")
	}
	stringDefault := &defaults.StringResponse{}
	shutdownAttr.Default.DefaultString(t.Context(), defaults.StringRequest{}, stringDefault)
	if stringDefault.Diagnostics.HasError() || stringDefault.PlanValue.ValueString() != shutdownOnDestroyGracefulThenKill {
		t.Fatalf("shutdown_on_destroy default = %#v, diagnostics %v", stringDefault.PlanValue, stringDefault.Diagnostics)
	}
	if !strings.Contains(strings.ToLower(shutdownAttr.MarkdownDescription), "acpi") {
		t.Fatalf("shutdown_on_destroy description %q should mention ACPI guests waiting", shutdownAttr.MarkdownDescription)
	}
	if len(shutdownAttr.Validators) == 0 {
		t.Fatal("shutdown_on_destroy should validate its values")
	}
}

func TestGracefulPowerOffPostsPoweroffAndPolls(t *testing.T) {
	shortenShutdownPoll(t)

	var mu sync.Mutex
	var actions []string
	statusReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			statusReads++
			payload := `{"status":"running","running":true}`
			if statusReads > 1 {
				payload = `{"status":"stopped","running":false}`
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(payload))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			actions = append(actions, string(body))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	api := &VMApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	if err := api.gracefulPowerOff(t.Context(), "7", "web", time.Second, false); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweroff"`) || strings.Contains(actions[0], `"kill"`) {
		t.Fatalf("actions = %#v, want one poweroff", actions)
	}
	if statusReads < 2 {
		t.Fatalf("status reads = %d, want a poll after poweroff", statusReads)
	}
}

func TestGracefulPowerOffTimeoutDoesNotKill(t *testing.T) {
	shortenShutdownPoll(t)

	var actions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"running","running":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			actions = append(actions, string(body))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	api := &VMApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	err := api.gracefulPowerOff(t.Context(), "7", "web", 0, false)
	if err == nil {
		t.Fatal("expected a timeout")
	}
	msg := err.Error()
	for _, want := range []string{"web", "7", "running"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweroff"`) {
		t.Fatalf("actions = %#v, want one poweroff and no kill", actions)
	}
}

func TestGracefulPowerOffForcePostsKill(t *testing.T) {
	shortenShutdownPoll(t)

	var mu sync.Mutex
	var actions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		kills := 0
		for _, action := range actions {
			if strings.Contains(action, `"action":"kill"`) {
				kills++
			}
		}
		if r.Method == http.MethodPost {
			actions = append(actions, string(body))
		}
		mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			payload := `{"status":"running","running":true}`
			if kills > 0 {
				payload = `{"status":"stopped","running":false}`
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(payload))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	api := &VMApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	if err := api.gracefulPowerOff(t.Context(), "7", "web", 0, true); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(actions) != 2 || !strings.Contains(actions[0], `"action":"poweroff"`) || !strings.Contains(actions[1], `"action":"kill"`) {
		t.Fatalf("actions = %#v, want poweroff then kill", actions)
	}
}

func shortenShutdownPoll(t *testing.T) {
	t.Helper()
	orig := gracefulShutdownInterval
	t.Cleanup(func() { gracefulShutdownInterval = orig })
	gracefulShutdownInterval = 0
}
