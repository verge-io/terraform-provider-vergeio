package compute

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestVMNameInUse(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "sdk 409",
			err:  &vergeos.APIError{StatusCode: 409, Endpoint: "/vms", Message: "This name is already in use"},
			want: true,
		},
		{
			name: "sdk 422",
			err:  &vergeos.APIError{StatusCode: 422, Endpoint: "/vms", Message: "This name is already in use"},
			want: true,
		},
		{
			name: "wrapped sdk 409",
			err:  fmt.Errorf("create: %w", &vergeos.APIError{StatusCode: 409, Message: "This name is already in use"}),
			want: true,
		},
		{
			name: "client 422",
			err:  vergeio.Error{StatusCode: 422, Endpoint: "api/v4/vms", VergeError: "This name is already in use"},
			want: true,
		},
		{
			name: "client 409 text",
			err:  fmt.Errorf("[ API Error 409 ] @ api/v4/vms - This name is already in use"),
			want: true,
		},
		{
			name: "unrelated 422",
			err:  &vergeos.APIError{StatusCode: 422, Message: "The specified drive is already hotplugging"},
			want: false,
		},
		{
			name: "not found",
			err:  &vergeos.APIError{StatusCode: 404, Message: "not found"},
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vmNameInUse(tt.err); got != tt.want {
				t.Fatalf("vmNameInUse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVMConfigMatches(t *testing.T) {
	plan := &VMResourceModel{
		Name:        types.StringValue("web"),
		CPUCores:    types.Int32Value(2),
		RAM:         types.Int32Value(2048),
		MachineType: types.StringValue("q35"),
		UEFI:        types.BoolValue(true),
		PowerState:  types.BoolValue(true),
		Description: types.StringNull(),
	}
	existing := &vergeos.VM{
		Key:         7,
		Name:        "web",
		CPUCores:    2,
		RAM:         2048,
		MachineType: "pc-q35-10.0",
		UEFI:        true,
		PowerState:  false,
		Description: "platform default",
	}

	if ok, reason := vmConfigMatches(plan, existing); !ok {
		t.Fatalf("matching VM rejected: %s", reason)
	}

	differentRAM := *existing
	differentRAM.RAM = 512
	ok, reason := vmConfigMatches(plan, &differentRAM)
	if ok {
		t.Fatal("different ram was adopted")
	}
	if !strings.Contains(reason, "ram is 512") || !strings.Contains(reason, "2048") {
		t.Fatalf("reason %q does not describe the ram mismatch", reason)
	}

	differentType := *existing
	differentType.MachineType = "pc-i440fx-10.0"
	if ok, reason := vmConfigMatches(plan, &differentType); ok {
		t.Fatalf("q35 matched %s (%s)", differentType.MachineType, reason)
	}

	snapshot := *existing
	snapshot.IsSnapshot = true
	if ok, reason := vmConfigMatches(plan, &snapshot); ok || !strings.Contains(reason, "snapshot") {
		t.Fatalf("snapshot match = %v, reason %q", ok, reason)
	}

	running := *existing
	running.PowerState = true
	stoppedPlan := *plan
	stoppedPlan.PowerState = types.BoolValue(false)
	if ok, reason := vmConfigMatches(&stoppedPlan, &running); ok || !strings.Contains(reason, "powerstate") {
		t.Fatalf("running VM matched powerstate false: %v %q", ok, reason)
	}
}

func TestPartialVMForStateDropsUnknownNestedValues(t *testing.T) {
	ctx := t.Context()
	data := &VMResourceModel{
		Id:            types.StringValue("7"),
		Name:          types.StringValue("web"),
		CPUCores:      types.Int32Value(2),
		RAM:           types.Int32Value(2048),
		GuestAgentIPs: types.ListUnknown(types.StringType),
		MachineType:   types.StringUnknown(),
		BootDisk: &bootDiskModel{
			Name: types.StringValue("os"),
			Key:  types.StringUnknown(),
		},
	}

	partial := partialVMForState(data)
	if data.BootDisk == nil {
		t.Fatal("partial state preparation changed the model create still uses")
	}
	if partial.BootDisk != nil || len(partial.Devices) != 0 {
		t.Fatalf("partial nested blocks boot=%v devices=%d", partial.BootDisk != nil, len(partial.Devices))
	}
	if partial.GuestAgentIPs.IsUnknown() || partial.MachineType.IsUnknown() {
		t.Fatal("partial state still has unknown values")
	}

	vmResource := &VMResource{}
	schemaResp := &fwresource.SchemaResponse{}
	vmResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &partial); diags.HasError() {
		t.Fatalf("partial state: %v", diags)
	}
	var got VMResourceModel
	if diags := state.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read partial state: %v", diags)
	}
	if got.Id.ValueString() != "7" || got.Name.ValueString() != "web" {
		t.Fatalf("stored id %q name %q", got.Id.ValueString(), got.Name.ValueString())
	}
	if got.BootDisk != nil {
		t.Fatal("stored boot disk, want none until it exists")
	}
}

func TestPartialVMForStateKeepsCreatedBootDiskAndDevices(t *testing.T) {
	ctx := t.Context()
	data := &VMResourceModel{
		Id:   types.StringValue("7"),
		Name: types.StringValue("web"),
		BootDisk: &bootDiskModel{
			Key:  types.StringValue("42"),
			Name: types.StringValue("os"),
			Size: types.Float64Value(40),
		},
		Devices: []*deviceResourceModel{
			{
				Key:  types.StringUnknown(),
				Name: types.StringValue("not-created"),
			},
			{
				Key:    types.StringValue("9"),
				Name:   types.StringValue("tpm"),
				Type:   types.StringValue("tpm"),
				Status: types.Int32Unknown(),
				DeviceTPMSettingsModel: &DeviceTPMSettingsModel{
					Version: TPMVersion{StringValue: types.StringUnknown()},
				},
			},
		},
	}

	partial := partialVMForState(data)
	if partial.BootDisk == nil || partial.BootDisk.Key.ValueString() != "42" {
		t.Fatalf("boot disk = %#v, want key 42", partial.BootDisk)
	}
	if len(partial.Devices) != 1 || partial.Devices[0].Key.ValueString() != "9" {
		t.Fatalf("devices = %#v, want only the created device", partial.Devices)
	}
	if partial.Devices[0].Status.IsUnknown() || partial.Devices[0].DeviceTPMSettingsModel.Version.IsUnknown() {
		t.Fatal("created device still has unknown values")
	}
	if !data.Devices[0].Key.IsUnknown() {
		t.Fatal("partial state preparation changed the device create still uses")
	}

	vmResource := &VMResource{}
	schemaResp := &fwresource.SchemaResponse{}
	vmResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &partial); diags.HasError() {
		t.Fatalf("partial state: %v", diags)
	}
}

func TestFindVMByNameEscapesFilter(t *testing.T) {
	ctx := t.Context()
	name := "O'Brien"
	var gotFilter string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = io.WriteString(w, `{"version":"26.0.0"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms":
			gotFilter = r.URL.Query().Get("filter")
			_, _ = io.WriteString(w, `[{"$key":4,"name":"O'Brien","machine":1,"cpu_cores":1,"ram":512}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/4":
			_, _ = io.WriteString(w, `{"$key":4,"name":"O'Brien","machine":1,"cpu_cores":1,"ram":512}`)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := mustAPI(NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	vm, err := api.findVMByName(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if vm == nil || vm.Key.Int() != 4 {
		t.Fatalf("vm = %#v", vm)
	}
	if gotFilter != `name eq 'O\'Brien'` {
		t.Fatalf("filter %q", gotFilter)
	}
}

func TestCreateStoresVMIdWhenBootDiskFails(t *testing.T) {
	ctx := t.Context()
	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":2048,"enabled":true,"powerstate":false}`
	var calls []string
	server := newVMCreateServer(t, &calls, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives" {
			return http.StatusNotFound, `{"err":"error setting field"}`, true
		}
		return 0, "", false
	}, vmJSON)
	defer server.Close()

	resp := createVM(t, ctx, server.URL, plannedVMWithBootDisk())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected boot disk create to fail")
	}
	detail := diagnosticText(resp.Diagnostics)
	if !strings.Contains(detail, "Error creating boot disk") || !strings.Contains(detail, "machine_drives") {
		t.Fatalf("diagnostics = %s", detail)
	}
	if id := stateID(t, ctx, resp.State); id != "7" {
		t.Fatalf("state id = %q, want 7 so the next apply can update the VM", id)
	}
	if !strings.Contains(strings.Join(calls, "\n"), "POST /api/v4/vms") {
		t.Fatalf("VM was not created: %s", strings.Join(calls, "\n"))
	}
}

func TestCreateAdoptsMatchingOrphan(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusUnprocessableEntity} {
		t.Run(fmt.Sprintf("status %d", status), func(t *testing.T) {
			ctx := t.Context()
			const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":2048,"enabled":true,"powerstate":false,"machine_type":"pc-q35-10.0","uefi":true}`
			var calls []string
			var creates int
			server := newVMCreateServer(t, &calls, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/v4/vms" {
					creates++
					return status, `{"err":"This name is already in use"}`, true
				}
				if r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms" {
					return http.StatusOK, `[{"$key":9,"name":"web-other","cpu_cores":1,"ram":512},{"$key":7,"name":"web","machine":1,"cpu_cores":2,"ram":2048,"machine_type":"pc-q35-10.0","uefi":true}]`, true
				}
				return 0, "", false
			}, vmJSON)
			defer server.Close()

			resp := createVM(t, ctx, server.URL, plannedVM())
			if resp.Diagnostics.HasError() {
				t.Fatalf("adopt: %v", resp.Diagnostics)
			}
			if id := stateID(t, ctx, resp.State); id != "7" {
				t.Fatalf("state id = %q, want the existing VM", id)
			}
			if creates != 1 {
				t.Fatalf("create calls = %d, want 1", creates)
			}
		})
	}
}

func TestCreateRefusesMismatchedOrphan(t *testing.T) {
	ctx := t.Context()
	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":512,"enabled":true}`
	server := newVMCreateServer(t, nil, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/vms" {
			return http.StatusConflict, `{"err":"This name is already in use"}`, true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms" {
			return http.StatusOK, `[{"$key":7,"name":"web","machine":1,"cpu_cores":2,"ram":512}]`, true
		}
		return 0, "", false
	}, vmJSON)
	defer server.Close()

	resp := createVM(t, ctx, server.URL, plannedVM())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a mismatch to be rejected")
	}
	detail := diagnosticText(resp.Diagnostics)
	for _, want := range []string{"id 7", "ram is 512", "2048", "terraform import vergeio_vm.<name> 7", "delete the VM"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("diagnostics missing %q: %s", want, detail)
		}
	}
	if id := stateID(t, ctx, resp.State); id != "" {
		t.Fatalf("mismatched orphan stored as id %q", id)
	}
}

func TestCreateKeepsIDWhenReadAfterCreateFails(t *testing.T) {
	ctx := t.Context()
	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":2048,"enabled":true,"powerstate":false}`
	var gets int
	server := newVMCreateServer(t, nil, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7" {
			gets++
			if gets > 1 {
				return http.StatusInternalServerError, `{"err":"read failed"}`, true
			}
		}
		return 0, "", false
	}, vmJSON)
	defer server.Close()

	resp := createVM(t, ctx, server.URL, plannedVM())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the follow-up read to fail")
	}
	if !strings.Contains(diagnosticText(resp.Diagnostics), "Error reading the VM") {
		t.Fatalf("diagnostics = %s", diagnosticText(resp.Diagnostics))
	}
	if id := stateID(t, ctx, resp.State); id != "7" {
		t.Fatalf("state id = %q, want 7", id)
	}
}

func TestCreateStoresBootDiskKeyWhenDriveReadFails(t *testing.T) {
	ctx := t.Context()
	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":2048,"enabled":true,"powerstate":false}`
	server := newVMCreateServer(t, nil, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives":
			return http.StatusCreated, `{"$key":"42"}`, true
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/42":
			return http.StatusInternalServerError, `{"err":"read failed"}`, true
		default:
			return 0, "", false
		}
	}, vmJSON)
	defer server.Close()

	resp := createVM(t, ctx, server.URL, plannedVMWithBootDisk())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the boot disk read to fail")
	}
	if !strings.Contains(diagnosticText(resp.Diagnostics), "Error creating boot disk") {
		t.Fatalf("diagnostics = %s", diagnosticText(resp.Diagnostics))
	}
	got := mustStateVM(t, ctx, resp.State)
	if got.Id.ValueString() != "7" {
		t.Fatalf("state id = %q, want 7", got.Id.ValueString())
	}
	if got.BootDisk == nil || got.BootDisk.Key.ValueString() != "42" {
		t.Fatalf("boot disk = %#v, want key 42", got.BootDisk)
	}
}

func TestCreateStoresDeviceKeyWhenLaterDeviceFails(t *testing.T) {
	ctx := t.Context()
	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":2048,"enabled":true,"powerstate":false}`
	var devicePosts int
	server := newVMCreateServer(t, nil, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_devices":
			devicePosts++
			if devicePosts == 1 {
				return http.StatusCreated, `{"$key":"9"}`, true
			}
			return http.StatusInternalServerError, `{"err":"device rejected"}`, true
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_devices/9":
			return http.StatusOK, `{"$key":"9","machine":1,"type":"node_pci_devices","name":"gpu","enabled":true}`, true
		default:
			return 0, "", false
		}
	}, vmJSON)
	defer server.Close()

	plan := plannedVM()
	plan.Devices = []*deviceResourceModel{
		{Name: types.StringValue("gpu"), Type: types.StringValue("node_pci_devices")},
		{Name: types.StringValue("tpm"), Type: types.StringValue("tpm")},
	}
	resp := createVM(t, ctx, server.URL, plan)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the second device to fail")
	}
	if !strings.Contains(diagnosticText(resp.Diagnostics), "Error creating device") {
		t.Fatalf("diagnostics = %s", diagnosticText(resp.Diagnostics))
	}
	got := mustStateVM(t, ctx, resp.State)
	if got.Id.ValueString() != "7" {
		t.Fatalf("state id = %q, want 7", got.Id.ValueString())
	}
	if len(got.Devices) != 1 || got.Devices[0].Key.ValueString() != "9" || got.Devices[0].Name.ValueString() != "gpu" {
		t.Fatalf("devices = %#v, want the first device key 9", got.Devices)
	}
}

func TestCreateStoresDeviceKeyWhenDeviceReadFails(t *testing.T) {
	ctx := t.Context()
	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":2048,"enabled":true,"powerstate":false}`
	server := newVMCreateServer(t, nil, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_devices":
			return http.StatusCreated, `{"$key":"9"}`, true
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_devices/9":
			return http.StatusInternalServerError, `{"err":"read failed"}`, true
		default:
			return 0, "", false
		}
	}, vmJSON)
	defer server.Close()

	plan := plannedVM()
	plan.Devices = []*deviceResourceModel{
		{Name: types.StringValue("gpu"), Type: types.StringValue("node_pci_devices")},
	}
	resp := createVM(t, ctx, server.URL, plan)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the device read to fail")
	}
	if !strings.Contains(diagnosticText(resp.Diagnostics), "Error creating device") {
		t.Fatalf("diagnostics = %s", diagnosticText(resp.Diagnostics))
	}
	got := mustStateVM(t, ctx, resp.State)
	if got.Id.ValueString() != "7" {
		t.Fatalf("state id = %q, want 7", got.Id.ValueString())
	}
	if len(got.Devices) != 1 || got.Devices[0].Key.ValueString() != "9" || got.Devices[0].Name.ValueString() != "gpu" {
		t.Fatalf("devices = %#v, want key 9", got.Devices)
	}
}

func TestCreateStoresVMIdWhenPowerOnTimesOut(t *testing.T) {
	ctx := t.Context()
	withFastVMPower(t)
	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":2048,"enabled":true,"powerstate":false}`
	server := newVMCreateServer(t, nil, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives":
			return http.StatusCreated, `{"$key":"42"}`, true
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/42":
			return http.StatusOK, `{"$key":"42","machine":1,"name":"os","disksize":42949672960,"enabled":true}`, true
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			return http.StatusOK, `{}`, true
		default:
			return 0, "", false
		}
	}, vmJSON)
	defer server.Close()

	plan := plannedVMWithBootDisk()
	plan.PowerState = types.BoolValue(true)
	resp := createVM(t, ctx, server.URL, plan)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected power on to time out")
	}
	detail := diagnosticText(resp.Diagnostics)
	if !strings.Contains(detail, "Error powering on VM") || !strings.Contains(detail, "did not become running") {
		t.Fatalf("diagnostics = %s", detail)
	}
	got := mustStateVM(t, ctx, resp.State)
	if got.Id.ValueString() != "7" {
		t.Fatalf("state id = %q, want 7", got.Id.ValueString())
	}
	if got.BootDisk == nil || got.BootDisk.Key.ValueString() != "42" {
		t.Fatalf("boot disk = %#v, want key 42 kept after power-on failure", got.BootDisk)
	}
	if !got.PowerState.IsNull() && got.PowerState.ValueBool() {
		t.Fatal("state recorded the VM as running after power-on timed out")
	}
}

func TestCreateStoresVMIdWhenCloudInitDetachFails(t *testing.T) {
	ctx := t.Context()
	withFastVMPower(t)
	origDelay := cloudInitDetachDelay
	cloudInitDetachDelay = 0
	t.Cleanup(func() { cloudInitDetachDelay = origDelay })

	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":2,"ram":2048,"enabled":true,"powerstate":false}`
	var cloudInitLists int
	server := newVMCreateServer(t, nil, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			return http.StatusOK, `{}`, true
		case r.Method == http.MethodGet && strings.Contains(r.URL.Query().Get("fields"), "as running"):
			return http.StatusOK, `{"running":true,"status":"running"}`, true
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/cloudinit_files":
			cloudInitLists++
			if cloudInitLists > 1 {
				return http.StatusInternalServerError, `{"err":"list failed"}`, true
			}
			return http.StatusOK, `[]`, true
		default:
			return 0, "", false
		}
	}, vmJSON)
	defer server.Close()

	plan := plannedVM()
	plan.PowerState = types.BoolValue(true)
	plan.CloudInitFiles = []CloudInitFile{
		{Name: types.StringValue("user-data"), Contents: types.StringValue("#cloud-config\n")},
	}
	resp := createVM(t, ctx, server.URL, plan)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected cloud-init detach to fail")
	}
	if !strings.Contains(diagnosticText(resp.Diagnostics), "Error detaching cloud-init") {
		t.Fatalf("diagnostics = %s", diagnosticText(resp.Diagnostics))
	}
	got := mustStateVM(t, ctx, resp.State)
	if got.Id.ValueString() != "7" {
		t.Fatalf("state id = %q, want 7", got.Id.ValueString())
	}
	if got.PowerState.IsNull() || !got.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want true after power-on succeeded", got.PowerState)
	}
}

func TestCreateDoesNotStoreStateWhenCreateFails(t *testing.T) {
	ctx := t.Context()
	server := newVMCreateServer(t, nil, func(w http.ResponseWriter, r *http.Request) (int, string, bool) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/vms" {
			return http.StatusInternalServerError, `{"err":"nope"}`, true
		}
		return 0, "", false
	}, `{}`)
	defer server.Close()

	resp := createVM(t, ctx, server.URL, plannedVM())
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected create to fail")
	}
	detail := diagnosticText(resp.Diagnostics)
	if strings.Contains(detail, "terraform import") {
		t.Fatalf("unrelated create error offered import: %s", detail)
	}
	if id := stateID(t, ctx, resp.State); id != "" {
		t.Fatalf("failed create stored id %q", id)
	}
}

func plannedVM() VMResourceModel {
	return VMResourceModel{
		Name:          types.StringValue("web"),
		CPUCores:      types.Int32Value(2),
		RAM:           types.Int32Value(2048),
		GuestAgentIPs: types.ListNull(types.StringType),
	}
}

func plannedVMWithBootDisk() VMResourceModel {
	plan := plannedVM()
	plan.BootDisk = &bootDiskModel{
		Name: types.StringValue("os"),
		Size: types.Float64Value(40),
	}
	return plan
}

func createVM(t *testing.T, ctx context.Context, host string, planModel VMResourceModel) *fwresource.CreateResponse {
	t.Helper()
	client := vergeio.NewClient(host, "user", "pass", true)
	vmResource := &VMResource{
		vmApi:     mustAPI(NewVMApi(client)),
		diskApi:   mustAPI(NewDiskApi(client)),
		nicApi:    mustAPI(NewNICApi(client)),
		deviceApi: mustAPI(NewDeviceApi(client)),
	}
	schemaResp := &fwresource.SchemaResponse{}
	vmResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("plan: %v", diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	vmResource.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	return resp
}

// newVMCreateServer answers the VM create, read, and device-list calls.
// special handles a request when it returns handled. vmJSON is the body
// for GET /api/v4/vms/{id}.
func newVMCreateServer(t *testing.T, calls *[]string, special func(http.ResponseWriter, *http.Request) (int, string, bool), vmJSON string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		if calls != nil {
			mu.Lock()
			*calls = append(*calls, r.Method+" "+r.URL.Path)
			mu.Unlock()
		}
		if special != nil {
			if status, body, handled := special(w, r); handled {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, body)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"version":"26.0.0"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vms":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"$key":7}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/vms/"):
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, vmJSON)
		case r.Method == http.MethodGet && (r.URL.Path == "/api/v4/machine_drives" || r.URL.Path == "/api/v4/machine_nics" || r.URL.Path == "/api/v4/cloudinit_files"):
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
}

func stateID(t *testing.T, ctx context.Context, state tfsdk.State) string {
	t.Helper()
	if state.Raw.Type() == nil || state.Raw.IsNull() {
		return ""
	}
	return mustStateVM(t, ctx, state).Id.ValueString()
}

func mustStateVM(t *testing.T, ctx context.Context, state tfsdk.State) VMResourceModel {
	t.Helper()
	if state.Raw.Type() == nil || state.Raw.IsNull() {
		t.Fatal("state is empty")
	}
	var got VMResourceModel
	if diags := state.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state: %v", diags)
	}
	return got
}

func withFastVMPower(t *testing.T) {
	t.Helper()
	origWait := vmPowerWaitTimeout
	origInterval := powerOnInterval
	origSettle := powerOnSettle
	vmPowerWaitTimeout = 0
	powerOnInterval = 0
	powerOnSettle = 0
	t.Cleanup(func() {
		vmPowerWaitTimeout = origWait
		powerOnInterval = origInterval
		powerOnSettle = origSettle
	})
}

func diagnosticText(diags diag.Diagnostics) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Summary())
		b.WriteString(" ")
		b.WriteString(d.Detail())
		b.WriteString("\n")
	}
	return b.String()
}
