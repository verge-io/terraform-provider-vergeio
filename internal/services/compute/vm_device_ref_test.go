package compute

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"terraform-provider-vergeio/internal/client"
)

func TestReplaceWhenVMChanges(t *testing.T) {
	mod := replaceWhenVMChanges()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	state := tfsdk.State{Raw: raw}
	plan := tfsdk.Plan{Raw: raw}

	changed := &planmodifier.StringResponse{PlanValue: types.StringValue("8")}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringValue("8"),
		PlanValue:   types.StringValue("8"),
		StateValue:  types.StringValue("7"),
		State:       state,
		Plan:        plan,
	}, changed)
	if !changed.RequiresReplace {
		t.Fatal("a different vm_id should replace the device")
	}

	imported := &planmodifier.StringResponse{PlanValue: types.StringValue("7")}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringValue("7"),
		PlanValue:   types.StringValue("7"),
		StateValue:  types.StringNull(),
		State:       state,
		Plan:        plan,
	}, imported)
	if imported.RequiresReplace {
		t.Fatal("filling vm_id after an import that only had the device key should not replace it")
	}
}

func TestParseVMScopedImportID(t *testing.T) {
	vmID, name, byName, err := parseVMScopedImportID("15/os")
	if err != nil || !byName || vmID != "15" || name != "os" {
		t.Fatalf("parse = %q %q %v %v", vmID, name, byName, err)
	}
	vmID, name, byName, err = parseVMScopedImportID("46")
	if err != nil || byName || vmID != "46" || name != "" {
		t.Fatalf("key parse = %q %q %v %v", vmID, name, byName, err)
	}
	vmID, name, byName, err = parseVMScopedImportID("15/disk/extra")
	if err != nil || !byName || vmID != "15" || name != "disk/extra" {
		t.Fatalf("slash name = %q %q %v %v", vmID, name, byName, err)
	}
	if _, _, _, err := parseVMScopedImportID("15/"); err == nil {
		t.Fatal("blank name was accepted")
	}
	if _, _, _, err := parseVMScopedImportID("  "); err == nil {
		t.Fatal("blank id was accepted")
	}
}

func TestMoveStateRejectsVMToDrive(t *testing.T) {
	movers := NewVMDriveResource().(*VMDriveResource).MoveState(context.Background())
	if len(movers) != 1 || movers[0].StateMover == nil {
		t.Fatal("drive resource should expose one state mover")
	}
	resp := &resource.MoveStateResponse{}
	movers[0].StateMover(context.Background(), resource.MoveStateRequest{SourceTypeName: "vergeio_vm"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("moving a VM into a drive should fail")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "vm_id/name") && !strings.Contains(detail, "<vm_id>/<name>") {
		t.Fatalf("detail should name the import id: %s", detail)
	}

	skipped := &resource.MoveStateResponse{}
	movers[0].StateMover(context.Background(), resource.MoveStateRequest{SourceTypeName: "vergeio_network"}, skipped)
	if skipped.Diagnostics.HasError() {
		t.Fatalf("unrelated source should be skipped: %v", skipped.Diagnostics)
	}
}

func TestAdoptDriveByNameDoesNotCreate(t *testing.T) {
	const driveJSON = `{"$key":"46","machine":1,"name":"os","interface":"virtio","disksize":5368709120,"enabled":true,"preferred_tier":"3"}`
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"$key":7,"machine":1,"name":"vm","cpu_cores":1,"ram":512,"enabled":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[` + driveJSON + `]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(driveJSON))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_drives":
			posts++
			http.Error(w, "should adopt", http.StatusInternalServerError)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/cloudinit_files":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	apiClient := vergeio.NewClient(server.URL, "user", "pass", true)
	drive := &VMDriveResource{vmApi: mustAPI(NewVMApi(apiClient)), diskApi: mustAPI(NewDiskApi(apiClient))}
	ctx := context.Background()
	machine, err := drive.vmApi.machineIDForVM(ctx, "7")
	if err != nil {
		t.Fatal(err)
	}
	existing, err := drive.diskApi.findDiskByName(ctx, machine.ValueInt32(), "os")
	if err != nil {
		t.Fatal(err)
	}
	if existing == nil || existing.Key.ValueString() != "46" {
		t.Fatalf("existing = %#v", existing)
	}
	if posts != 0 {
		t.Fatalf("create posts = %d, want 0", posts)
	}

	boot, err := (&VMResource{diskApi: drive.diskApi}).adoptOrCreateBootDisk(ctx, &bootDiskModel{
		Name: types.StringValue("os"),
		Size: types.Float64Value(5),
	}, machine, types.StringValue("7"))
	if err != nil {
		t.Fatal(err)
	}
	if boot == nil || boot.Key.ValueString() != "46" || boot.Name.ValueString() != "os" {
		t.Fatalf("boot disk = %#v", boot)
	}
	if posts != 0 {
		t.Fatalf("boot disk create posts = %d, want 0", posts)
	}
}
