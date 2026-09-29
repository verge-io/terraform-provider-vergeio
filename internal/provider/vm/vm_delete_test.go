package vm

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

	"terraform-provider-vergeio/internal/provider/vergeio"
)

// Delete is also the replace path. -replace, taint, and a force-new change
// all destroy the old VM through VMResource.Delete before creating the new one.

func TestVMDeletePowersOffRunningVMThenDeletes(t *testing.T) {
	actions, deleted := runVMDelete(t, vmDeleteCase{
		stopAfter: vmActionPowerOff,
	})
	if deleted != 1 {
		t.Fatalf("deletes = %d, want 1 after the guest stops", deleted)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweroff"`) || strings.Contains(actions[0], `"kill"`) {
		t.Fatalf("actions = %#v, want one poweroff and no kill", actions)
	}
}

func TestVMDeleteKillsWhenPoweroffTimesOut(t *testing.T) {
	actions, deleted := runVMDelete(t, vmDeleteCase{
		mode:      shutdownOnDestroyGracefulThenKill,
		delete:    "0s",
		stopAfter: vmActionKill,
	})
	if deleted != 1 {
		t.Fatalf("deletes = %d, want 1 after kill", deleted)
	}
	if len(actions) != 2 || !strings.Contains(actions[0], `"action":"poweroff"`) || !strings.Contains(actions[1], `"action":"kill"`) {
		t.Fatalf("actions = %#v, want poweroff then one kill", actions)
	}
}

func TestVMDeleteGracefulModeDeletesWhenGuestStops(t *testing.T) {
	actions, deleted := runVMDelete(t, vmDeleteCase{
		mode:      shutdownOnDestroyGraceful,
		stopAfter: vmActionPowerOff,
	})
	if deleted != 1 {
		t.Fatalf("deletes = %d, want 1 after the guest stops", deleted)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweroff"`) || strings.Contains(actions[0], `"kill"`) {
		t.Fatalf("actions = %#v, want one poweroff and no kill", actions)
	}
}

func TestVMDeleteGracefulModeDoesNotKillOrDelete(t *testing.T) {
	actions, deleted := runVMDelete(t, vmDeleteCase{
		mode:    shutdownOnDestroyGraceful,
		delete:  "0s",
		wantErr: true,
	})
	if deleted != 0 {
		t.Fatalf("deletes = %d, want 0 when graceful shutdown times out", deleted)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"poweroff"`) || strings.Contains(strings.Join(actions, " "), `"kill"`) {
		t.Fatalf("actions = %#v, want one poweroff and no kill", actions)
	}
}

func TestVMDeleteKillModeSkipsPoweroff(t *testing.T) {
	actions, deleted := runVMDelete(t, vmDeleteCase{
		mode:      shutdownOnDestroyKill,
		stopAfter: vmActionKill,
	})
	if deleted != 1 {
		t.Fatalf("deletes = %d, want 1 after kill", deleted)
	}
	joined := strings.Join(actions, " ")
	if strings.Contains(joined, `"poweroff"`) {
		t.Fatalf("actions = %#v, want kill only", actions)
	}
	if len(actions) != 1 || !strings.Contains(actions[0], `"action":"kill"`) {
		t.Fatalf("actions = %#v, want one kill", actions)
	}
}

func TestVMDeleteStoppedVMDeletesWithoutPowerAction(t *testing.T) {
	for _, mode := range []string{"", shutdownOnDestroyGracefulThenKill, shutdownOnDestroyGraceful, shutdownOnDestroyKill} {
		t.Run(mode, func(t *testing.T) {
			actions, deleted := runVMDelete(t, vmDeleteCase{
				mode:           mode,
				alreadyStopped: true,
			})
			if deleted != 1 {
				t.Fatalf("deletes = %d, want 1", deleted)
			}
			if len(actions) != 0 {
				t.Fatalf("actions = %#v, want no power action for a stopped VM", actions)
			}
		})
	}
}

func TestVMDeleteRejectsUnknownShutdownMode(t *testing.T) {
	actions, deleted := runVMDelete(t, vmDeleteCase{
		mode:    "reboot",
		wantErr: true,
	})
	if deleted != 0 || len(actions) != 0 {
		t.Fatalf("deleted=%d actions=%#v, want no API call", deleted, actions)
	}
}

type vmDeleteCase struct {
	mode           string
	delete         string
	alreadyStopped bool
	// stopAfter is the action after which status reads report stopped.
	// Empty means the VM stays running.
	stopAfter string
	wantErr   bool
}

func runVMDelete(t *testing.T, tc vmDeleteCase) ([]string, int) {
	t.Helper()

	var mu sync.Mutex
	var actions []string
	deleted := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()

		running := !tc.alreadyStopped
		if tc.stopAfter != "" {
			for _, action := range actions {
				if strings.Contains(action, `"action":"`+tc.stopAfter+`"`) {
					running = false
				}
			}
		}

		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			status := "stopped"
			if running {
				status = "running"
			}
			_, _ = w.Write([]byte(`{"$key":7,"name":"web","powerstate":` + boolJSON(running) + `,"running":` + boolJSON(running) + `,"status":"` + status + `"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			actions = append(actions, string(body))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vms/7":
			deleted++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.RequestURI(), body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	client := vergeio.NewClient(server.URL, "user", "pass", true)
	vmResource := &VMResource{vmApi: NewVMApi(client)}

	schemaResp := &fwresource.SchemaResponse{}
	vmResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	stateModel := VMResourceModel{
		Id:            types.StringValue("7"),
		Name:          types.StringValue("web"),
		GuestAgentIPs: types.ListNull(types.StringType),
	}
	if tc.mode != "" {
		stateModel.ShutdownOnDestroy = types.StringValue(tc.mode)
	}
	if tc.delete != "" {
		stateModel.Timeouts = &vmTimeoutsModel{Delete: types.StringValue(tc.delete)}
	}

	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("state: %v", diags)
	}

	resp := &fwresource.DeleteResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	vmResource.Delete(ctx, fwresource.DeleteRequest{State: state}, resp)
	if tc.wantErr {
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected destroy to fail")
		}
		msg := resp.Diagnostics.Errors()[0].Detail()
		if tc.mode == shutdownOnDestroyGraceful {
			for _, want := range []string{"web", "7"} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q missing %q", msg, want)
				}
			}
		}
	} else if resp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), actions...), deleted
}

func boolJSON(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
