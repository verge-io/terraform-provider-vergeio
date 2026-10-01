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

// TestVMUpdateSyncsNilDriveNICAndDeviceLists is the apply that adds the first
// NIC and device to a VM that has none, and removes its last drive.
// An empty nested list decodes to a nil slice. Update has to sync those
// slices anyway, or the API is left unchanged and the next plan repeats.
func TestVMUpdateSyncsNilDriveNICAndDeviceLists(t *testing.T) {
	shortenPowerWaits(t)
	const vmJSON = `{"$key":7,"machine":1,"name":"vm","cpu_cores":1,"ram":512,"enabled":true,"powerstate":false}`
	const driveJSON = `{"$key":"46","machine":1,"name":"os","interface":"virtio","disksize":1073741824,"enabled":true,"powerstate":"offline"}`
	const nicJSON = `{"machine":1,"name":"lan","interface":"virtio","enabled":true,"vnet":6,"macaddress":"52:54:00:aa:bb:cc"}`
	const deviceJSON = `{"machine":1,"type":"node_pci_devices","name":"gpu","enabled":true}`

	var mu sync.Mutex
	var calls []string
	nicCreated := false
	driveDeleted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/cloudinit_files":
			w.WriteHeader(http.StatusOK)
			response = []byte(`[]`)
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
		vmApi:     mustAPI(NewVMApi(client)),
		diskApi:   mustAPI(NewDiskApi(client)),
		nicApi:    mustAPI(NewNICApi(client)),
		deviceApi: mustAPI(NewDeviceApi(client)),
	}

	schemaResp := &fwresource.SchemaResponse{}
	vmResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	base := VMResourceModel{
		Id:            types.StringValue("7"),
		Machine:       types.Int32Value(1),
		Name:          types.StringValue("vm"),
		GuestAgentIPs: types.ListNull(types.StringType),
	}
	// Prior state has a boot disk and no devices. NICs are not on the VM.
	stateModel := base
	stateModel.BootDisk = &bootDiskModel{
		Key:   types.StringValue("46"),
		Name:  types.StringValue("os"),
		Media: types.StringValue("disk"),
		Size:  types.Float64Value(1),
	}
	// Plan removes that boot disk and adds the first device.
	planModel := base
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
	if decodedPlan.BootDisk != nil || decodedState.BootDisk == nil || decodedState.Devices != nil {
		t.Fatalf("boot disk decode plan nil=%v state nil=%v devices nil=%v",
			decodedPlan.BootDisk == nil, decodedState.BootDisk == nil, decodedState.Devices == nil)
	}
	if decodedPlan.Devices == nil || len(decodedPlan.Devices) != 1 {
		t.Fatalf("decoded plan devices=%d", len(decodedPlan.Devices))
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
	if got.BootDisk != nil {
		t.Fatalf("boot disk after removal = %#v, want nil", got.BootDisk)
	}
	if len(got.Devices) != 1 || got.Devices[0].Key.ValueString() != "12" || got.Devices[0].Name.ValueString() != "gpu" {
		t.Fatalf("devices after adding the first = %#v", got.Devices)
	}

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(calls, "\n")
	for _, want := range []string{
		"DELETE /api/v4/machine_drives/46",
		"POST /api/v4/machine_devices",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("calls missing %s:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "POST /api/v4/machine_nics") {
		t.Errorf("VM update created a NIC:\n%s", joined)
	}
}

// TestVMUpdateSyncsDrivesAndNICsBeforePowerOn is the apply that both starts
// a stopped VM and adds a drive and a NIC. The devices have to be created
// before poweron, or the guest boots without them and they stay offline.
func TestVMUpdateSyncsDrivesAndNICsBeforePowerOn(t *testing.T) {
	shortenPowerWaits(t)
	const stoppedVM = `{"$key":7,"machine":1,"name":"vm","cpu_cores":1,"ram":512,"enabled":true,"powerstate":false}`
	const runningVM = `{"$key":7,"machine":1,"name":"vm","cpu_cores":1,"ram":512,"enabled":true,"powerstate":true}`
	const driveJSON = `{"$key":"99","machine":1,"name":"data","interface":"virtio","disksize":1073741824,"enabled":true}`
	const nicJSON = `{"$key":"98","machine":1,"name":"lan","interface":"virtio","enabled":true,"vnet":6,"macaddress":"52:54:00:aa:bb:cc"}`

	var mu sync.Mutex
	var calls []string
	var actionBodies []string
	powered := false
	driveCreated := false
	nicCreated := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/api/v4/vm_actions" {
			actionBodies = append(actionBodies, string(body))
			if strings.Contains(string(body), `"action":"poweron"`) {
				powered = true
			}
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives" {
			driveCreated = true
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_nics" {
			nicCreated = true
		}
		isPowered := powered
		hasDrive := driveCreated
		hasNIC := nicCreated
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		var response []byte
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			response = []byte(`{"version":"26.0.0"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			response = []byte(`{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			if isPowered {
				response = []byte(runningVM)
			} else {
				response = []byte(stoppedVM)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives":
			w.WriteHeader(http.StatusCreated)
			response = []byte(`{"$key":"99"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/99":
			w.WriteHeader(http.StatusOK)
			response = []byte(driveJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives":
			w.WriteHeader(http.StatusOK)
			if hasDrive {
				response = []byte(`[` + driveJSON + `]`)
			} else {
				response = []byte(`[]`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_nics":
			w.WriteHeader(http.StatusCreated)
			response = []byte(`{"$key":"98"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
			w.WriteHeader(http.StatusOK)
			response = []byte(nicJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics":
			w.WriteHeader(http.StatusOK)
			if hasNIC {
				response = []byte(`[` + nicJSON + `]`)
			} else {
				response = []byte(`[]`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			w.WriteHeader(http.StatusCreated)
			response = []byte(`{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/cloudinit_files":
			w.WriteHeader(http.StatusOK)
			response = []byte(`[]`)
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
		vmApi:     mustAPI(NewVMApi(client)),
		diskApi:   mustAPI(NewDiskApi(client)),
		nicApi:    mustAPI(NewNICApi(client)),
		deviceApi: mustAPI(NewDeviceApi(client)),
	}

	schemaResp := &fwresource.SchemaResponse{}
	vmResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	stateModel := VMResourceModel{
		Id:            types.StringValue("7"),
		Machine:       types.Int32Value(1),
		Name:          types.StringValue("vm"),
		PowerState:    types.BoolValue(false),
		GuestAgentIPs: types.ListNull(types.StringType),
	}
	planModel := stateModel
	planModel.PowerState = types.BoolValue(true)
	planModel.BootDisk = &bootDiskModel{
		Name:  types.StringValue("data"),
		Media: types.StringValue("disk"),
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
	vmResource.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update: %v", resp.Diagnostics)
	}

	mu.Lock()
	defer mu.Unlock()
	if !callBefore(calls, "POST /api/v4/machine_drives", "POST /api/v4/vm_actions") {
		t.Fatalf("calls = %#v, want the boot disk created before power on", calls)
	}
	if strings.Contains(strings.Join(calls, "\n"), "POST /api/v4/machine_nics") {
		t.Fatalf("calls = %#v, VM update created a NIC", calls)
	}
	if !callBefore(calls, "PUT /api/v4/vms/7", "POST /api/v4/machine_drives") {
		t.Fatalf("calls = %#v, want the VM record updated before the drive is created", calls)
	}
	poweredOn := false
	for _, body := range actionBodies {
		if strings.Contains(body, "hotplug") {
			t.Fatalf("stopped VM hotplugged a device before boot: %s", body)
		}
		if strings.Contains(body, `"action":"poweron"`) {
			poweredOn = true
		}
	}
	if !poweredOn {
		t.Fatalf("poweron action was not sent: %#v", actionBodies)
	}
}
