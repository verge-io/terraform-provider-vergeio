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

// TestUpdateStoresPlannedDisplay is the apply that changes display on an
// existing VM. The vms row has no display column, so the GET after update
// cannot return the new value. State has to come back as the plan.
func TestUpdateStoresPlannedDisplay(t *testing.T) {
	shortenPowerWaits(t)

	var mu sync.Mutex
	var putBody string

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

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vms/7":
			putBody = string(body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"$key":7,"machine":1,"name":"web","console":"vnc","video":"qxl","enabled":true,"powerstate":false,"running":false,"status":"stopped"}`))
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
		Display:       types.StringValue("vnc"),
		Console:       types.StringValue("vnc"),
		Video:         types.StringValue("qxl"),
		GuestAgentIPs: types.ListNull(types.StringType),
	}
	planModel := stateModel
	planModel.Display = types.StringValue("spice")

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
	if resp.Diagnostics.HasError() {
		t.Fatalf("update: %v", resp.Diagnostics)
	}

	var got VMResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("updated state: %v", diags)
	}
	if got.Display.IsNull() || got.Display.IsUnknown() || got.Display.ValueString() != "spice" {
		t.Fatalf("display = %#v, want spice", got.Display)
	}
	if got.Console.ValueString() != "vnc" || got.Video.ValueString() != "qxl" {
		t.Fatalf("console = %#v, video = %#v", got.Console, got.Video)
	}

	mu.Lock()
	body := putBody
	mu.Unlock()
	if strings.Contains(body, "display") {
		t.Fatalf("update body = %s, display must not be sent", body)
	}
}
