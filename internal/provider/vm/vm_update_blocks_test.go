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

// TestVMUpdateSyncsNilDriveNICAndDeviceLists is the apply that adds the first
// NIC and device to a VM that has none, and removes its last drive.
// An empty nested list decodes to a nil slice. Update has to sync those
// slices anyway, or the API is left unchanged and the next plan repeats.
func TestVMUpdateSyncsNilDriveNICAndDeviceLists(t *testing.T) {
	const vmJSON = `{"$key":7,"machine":1,"name":"vm","cpu_cores":1,"ram":512,"enabled":true,"powerstate":false}`
	const driveJSON = `{"$key":"46","machine":1,"name":"os","interface":"virtio","disksize":1073741824,"enabled":true,"powerstate":"offline"}`
	const nicJSON = `{"machine":1,"name":"lan","interface":"virtio","enabled":true,"vnet":6,"macaddress":"52:54:00:aa:bb:cc"}`
	const deviceJSON = `{"machine":1,"type":"node_pci_devices","name":"gpu","enabled":true}`

	var mu sync.Mutex
	var calls []string
	nicCreated := false
	driveDeleted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		created := nicCreated
		deleted := driveDeleted
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_nics":
			nicCreated = true
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/machine_drives/46":
			driveDeleted = true
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		var response []byte
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			response = []byte(`{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			response = []byte(vmJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives":
			w.WriteHeader(http.StatusOK)
			if deleted {
				response = []byte(`[]`)
			} else {
				response = []byte(`[` + driveJSON + `]`)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			response = []byte(driveJSON)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			response = []byte(`{}`)
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			response = []byte(`{"version":"26.0.0"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics":
			w.WriteHeader(http.StatusOK)
			if created {
				response = []byte(`[{"$key":98,"machine":1,"name":"lan","interface":"virtio","enabled":true,"vnet":6,"macaddress":"52:54:00:aa:bb:cc"}]`)
			} else {
				response = []byte(`[]`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_nics":
			if !strings.Contains(string(body), `"name":"lan"`) {
				t.Errorf("NIC create body = %s, want name lan", body)
			}
			w.WriteHeader(http.StatusCreated)
			response = []byte(`{"$key":"98"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
			w.WriteHeader(http.StatusOK)
			response = []byte(nicJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_devices":
			if !strings.Contains(string(body), `"name":"gpu"`) {
				t.Errorf("device create body = %s, want name gpu", body)
			}
			w.WriteHeader(http.StatusCreated)
			response = []byte(`{"$key":"12"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_devices/12":
			w.WriteHeader(http.StatusOK)
			response = []byte(deviceJSON)
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.RequestURI(), body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(response); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	client := vergeio.NewClient(server.URL, "user", "pass", true)
	vmResource := &VMResource{
		vmApi:     NewVMApi(client),
		diskApi:   NewDiskApi(client),
		nicApi:    NewNICApi(client),
		deviceApi: NewDeviceApi(client),
	}

	schemaResp := &fwresource.SchemaResponse{}
	vmResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	base := VMResourceModel{
		Id:            types.StringValue("7"),
		Machine:       types.Int32Value(1),
		Name:          types.StringValue("vm"),
		GuestAgentIPs: types.ListNull(types.StringType),
	}
	// Prior state has one drive and no NICs or devices. Empty lists are nil.
	stateModel := base
	stateModel.Disks = []*diskResourceModel{{
		Key:       types.StringValue("46"),
		Name:      types.StringValue("os"),
		Interface: types.StringValue("virtio"),
		Media:     types.StringValue("disk"),
		DiskSize:  types.Float64Value(1),
		Enabled:   types.BoolValue(true),
	}}
	// Plan removes that drive and adds the first NIC and device.
	planModel := base
	planModel.NICs = []*nicResourceModel{{
		Name:      types.StringValue("lan"),
		Interface: types.StringValue("virtio"),
		Enabled:   types.BoolValue(true),
		VNET:      types.Int32Value(6),
	}}
	planModel.Devices = []*deviceResourceModel{{
		Name:    types.StringValue("gpu"),
		Type:    types.StringValue("node_pci_devices"),
		Enabled: types.BoolValue(true),
	}}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("plan: %v", diags)
	}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("state: %v", diags)
	}

	// The values Update receives are what Plan.Get and State.Get decode.
	// An empty list is a nil slice, which used to skip the sync.
	var decodedPlan, decodedState VMResourceModel
	if diags := plan.Get(ctx, &decodedPlan); diags.HasError() {
		t.Fatalf("decode plan: %v", diags)
	}
	if diags := state.Get(ctx, &decodedState); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if decodedPlan.Disks != nil || decodedState.NICs != nil || decodedState.Devices != nil {
		t.Fatalf("empty lists decoded as non-nil: plan drives nil=%v state nics nil=%v state devices nil=%v",
			decodedPlan.Disks == nil, decodedState.NICs == nil, decodedState.Devices == nil)
	}
	if len(decodedPlan.NICs) != 1 || len(decodedState.Disks) != 1 || len(decodedPlan.Devices) != 1 {
		t.Fatalf("decoded plan nics=%d devices=%d state drives=%d", len(decodedPlan.NICs), len(decodedPlan.Devices), len(decodedState.Disks))
	}

	resp := &fwresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}
	vmResource.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update: %v", resp.Diagnostics)
	}

	var got VMResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("updated state: %v", diags)
	}
	if len(got.NICs) != 1 || got.NICs[0].Name.ValueString() != "lan" {
		t.Fatalf("NICs after adding the first = %d, want 1 named lan", len(got.NICs))
	}
	if len(got.Disks) != 0 {
		t.Fatalf("drives after removing the last = %d, want 0", len(got.Disks))
	}
	if len(got.Devices) != 1 || got.Devices[0].Key.ValueString() != "12" || got.Devices[0].Name.ValueString() != "gpu" {
		t.Fatalf("devices after adding the first = %#v", got.Devices)
	}

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(calls, "\n")
	for _, want := range []string{
		"POST /api/v4/machine_nics",
		"DELETE /api/v4/machine_drives/46",
		"POST /api/v4/machine_devices",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("calls missing %s:\n%s", want, joined)
		}
	}
}
