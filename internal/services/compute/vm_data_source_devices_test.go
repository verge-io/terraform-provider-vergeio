package compute

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

const driveListFields26 = "$key,machine,orderid,name,description,disksize,used_bytes,interface,media,media_source,preferred_tier,enabled,readonly,optimize,serial,fsync,discard,advanced,preserve_drive_format,asset"

func TestMachineIDFilter(t *testing.T) {
	if got := machineIDFilter(nil); got != "" {
		t.Fatalf("empty = %q", got)
	}
	if got := machineIDFilter([]int{0, -3}); got != "" {
		t.Fatalf("non-positive = %q", got)
	}
	if got := machineIDFilter([]int{10, 10, 0}); got != "machine eq 10" {
		t.Fatalf("one machine = %q", got)
	}
	if got := machineIDFilter([]int{11, 10, 11}); got != "(machine eq 10 or machine eq 11)" {
		t.Fatalf("two machines = %q", got)
	}
}

func TestReadVMsGroupsDrivesAndNICs(t *testing.T) {
	var mu sync.Mutex
	var driveCalls, nicCalls int
	var driveFilter, nicFilter, driveFields, nicFields string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms":
			_, _ = w.Write([]byte(`[
				{"$key":1,"machine":11,"name":"other","is_snapshot":false,"cpu_type":"host","machine_type":"pc","os_family":"linux","uefi":false},
				{"$key":2,"machine":10,"name":"web","is_snapshot":false,"cpu_type":"host","machine_type":"q35","os_family":"linux","uefi":true},
				{"$key":3,"machine":12,"name":"web-snap","is_snapshot":true}
			]`))
		case "/api/v4/machine_drives":
			mu.Lock()
			driveCalls++
			driveFilter = r.URL.Query().Get("filter")
			driveFields = r.URL.Query().Get("fields")
			mu.Unlock()
			_, _ = w.Write([]byte(`[
				{"$key":46,"machine":11,"orderid":1,"name":"data","interface":"virtio","media":"disk","description":"extra","preferred_tier":"3","media_source":90,"used_bytes":4096},
				{"$key":47,"machine":10,"orderid":2,"name":"swap","interface":"virtio-scsi","media":"disk","preferred_tier":""},
				{"$key":45,"machine":10,"orderid":0,"name":"os","interface":"virtio-scsi","media":"disk","description":"boot","preferred_tier":"2","media_source":80,"used_bytes":1024},
				{"$key":99,"machine":99,"orderid":0,"name":"foreign","preferred_tier":"1","media_source":1,"used_bytes":1}
			]`))
		case "/api/v4/machine_nics":
			mu.Lock()
			nicCalls++
			nicFilter = r.URL.Query().Get("filter")
			nicFields = r.URL.Query().Get("fields")
			mu.Unlock()
			_, _ = w.Write([]byte(`[
				{"$key":"98","machine":10,"name":"nic1","interface":"e1000","vnet":7,"macaddress":"52:54:00:00:00:02","ipaddress":"192.0.2.20","powerState":"up"},
				{"$key":97,"machine":10,"name":"nic0","interface":"virtio","macaddress":"52:54:00:00:00:01","powerState":"down"},
				{"$key":99,"machine":99,"name":"foreign","interface":"virtio","vnet":1}
			]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &VMDataSourceModel{
		FilterName: types.StringValue("web"),
		IsSnapshot: types.BoolValue(false),
	}
	if err := api.readVMs(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if driveCalls != 1 || nicCalls != 1 {
		t.Fatalf("drive calls %d, nic calls %d, want one each", driveCalls, nicCalls)
	}
	if driveFilter != "machine eq 10" || nicFilter != "machine eq 10" {
		t.Fatalf("drive filter %q nic filter %q", driveFilter, nicFilter)
	}
	if driveFields != driveListFields26 {
		t.Fatalf("drive fields = %q", driveFields)
	}
	if nicFields != vmListNICFields {
		t.Fatalf("nic fields = %q", nicFields)
	}
	if len(data.Vms) != 1 {
		t.Fatalf("vms = %d, want the VM named web", len(data.Vms))
	}
	vm := data.Vms[0]
	if vm.Name.ValueString() != "web" || vm.Key.ValueInt32() != 2 || vm.Id.ValueInt32() != 10 {
		t.Fatalf("vm = name %s key %d id %d", vm.Name.ValueString(), vm.Key.ValueInt32(), vm.Id.ValueInt32())
	}
	if vm.Drives == nil || len(vm.Drives) != 2 {
		t.Fatalf("drives = %#v", vm.Drives)
	}
	if vm.Drives[0].Name.ValueString() != "os" || vm.Drives[0].Key.ValueInt32() != 45 || vm.Drives[0].PreferredTier.ValueInt32() != 2 {
		t.Fatalf("first drive = %#v", vm.Drives[0])
	}
	if vm.Drives[0].MediaSource == nil || vm.Drives[0].MediaSource.Key.ValueInt32() != 80 || vm.Drives[0].MediaSource.UsedBytes.ValueInt64() != 1024 {
		t.Fatalf("os media source = %#v", vm.Drives[0].MediaSource)
	}
	if !vm.Drives[0].MediaSource.AllocatedBytes.IsNull() || !vm.Drives[0].MediaSource.Filesize.IsNull() {
		t.Fatalf("os media source sizes = %#v", vm.Drives[0].MediaSource)
	}
	if vm.Drives[1].Name.ValueString() != "swap" || vm.Drives[1].MediaSource != nil || !vm.Drives[1].PreferredTier.IsNull() {
		t.Fatalf("second drive = %#v", vm.Drives[1])
	}
	if vm.Nics == nil || len(vm.Nics) != 2 {
		t.Fatalf("nics = %#v", vm.Nics)
	}
	if vm.Nics[0].Name.ValueString() != "nic0" || !vm.Nics[0].Vnet.IsNull() || vm.Nics[0].Status.ValueString() != "down" || vm.Nics[0].MacAddress.ValueString() != "52:54:00:00:00:01" {
		t.Fatalf("first nic = %#v", vm.Nics[0])
	}
	if vm.Nics[1].Name.ValueString() != "nic1" || vm.Nics[1].Vnet.ValueInt32() != 7 || vm.Nics[1].Ipaddress.ValueString() != "192.0.2.20" || vm.Nics[1].Status.ValueString() != "up" {
		t.Fatalf("second nic = %#v", vm.Nics[1])
	}
}

func TestReadVMsEmptyDevicesAreEmptyLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms":
			_, _ = w.Write([]byte(`[{"$key":2,"machine":10,"name":"web"}]`))
		case "/api/v4/machine_drives", "/api/v4/machine_nics":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	source := &VMDataSource{vmApi: api}
	ctx := t.Context()
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}
	config := tfsdk.State{Schema: schemaResp.Schema}
	if diags := config.Set(ctx, &VMDataSourceModel{FilterName: types.StringValue("web")}); diags.HasError() {
		t.Fatalf("config: %v", diags)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	source.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: config.Raw, Schema: schemaResp.Schema}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	for _, name := range []string{"drives", "nics"} {
		var list types.List
		attr := path.Root("vms").AtListIndex(0).AtName(name)
		if diags := resp.State.GetAttribute(ctx, attr, &list); diags.HasError() {
			t.Fatalf("%s: %v", name, diags)
		}
		if list.IsNull() || list.IsUnknown() {
			t.Fatalf("%s is null", name)
		}
		if len(list.Elements()) != 0 {
			t.Fatalf("%s has %d elements", name, len(list.Elements()))
		}
	}
}

func TestReadVMsDeviceListErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "drives", path: "/api/v4/machine_drives", want: "listing drives"},
		{name: "nics", path: "/api/v4/machine_nics", want: "listing NICs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				case "/api/v4/vms":
					_, _ = w.Write([]byte(`[{"$key":2,"machine":10,"name":"web"}]`))
				case "/api/v4/machine_drives", "/api/v4/machine_nics":
					if r.URL.Path == tc.path {
						http.Error(w, "nope", http.StatusInternalServerError)
						return
					}
					_, _ = w.Write([]byte(`[]`))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			api := mustAPI(NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true)))
			err := api.readVMs(t.Context(), &VMDataSourceModel{FilterName: types.StringValue("web")})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReadVMsRejectsDriveTier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms":
			_, _ = w.Write([]byte(`[{"$key":2,"machine":10,"name":"web"}]`))
		case "/api/v4/machine_drives":
			_, _ = w.Write([]byte(`[{"$key":45,"machine":10,"name":"os","preferred_tier":"tier"}]`))
		case "/api/v4/machine_nics":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	err := api.readVMs(t.Context(), &VMDataSourceModel{FilterName: types.StringValue("web")})
	if err == nil || !strings.Contains(err.Error(), "preferred_tier") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadVMsListsDevicesForEveryKeptMachine(t *testing.T) {
	var mu sync.Mutex
	var driveFilter, nicFilter string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms":
			_, _ = w.Write([]byte(`[
				{"$key":1,"machine":11,"name":"db","is_snapshot":false},
				{"$key":2,"machine":10,"name":"web","is_snapshot":false},
				{"$key":3,"machine":12,"name":"web-snap","is_snapshot":true}
			]`))
		case "/api/v4/machine_drives":
			mu.Lock()
			driveFilter = r.URL.Query().Get("filter")
			mu.Unlock()
			_, _ = w.Write([]byte(`[
				{"$key":45,"machine":10,"orderid":0,"name":"os","preferred_tier":"1"},
				{"$key":46,"machine":11,"orderid":0,"name":"data","preferred_tier":"4","media_source":"91","used_bytes":8}
			]`))
		case "/api/v4/machine_nics":
			mu.Lock()
			nicFilter = r.URL.Query().Get("filter")
			mu.Unlock()
			_, _ = w.Write([]byte(`[{"$key":97,"machine":11,"name":"lan","interface":"virtio","vnet":"4","powerState":"up"}]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &VMDataSourceModel{IsSnapshot: types.BoolValue(false)}
	if err := api.readVMs(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := "(machine eq 10 or machine eq 11)"
	if driveFilter != want || nicFilter != want {
		t.Fatalf("drive filter %q nic filter %q", driveFilter, nicFilter)
	}
	if len(data.Vms) != 2 {
		t.Fatalf("vms = %d", len(data.Vms))
	}
	byName := map[string]*VMModel{}
	for _, vm := range data.Vms {
		byName[vm.Name.ValueString()] = vm
	}
	web := byName["web"]
	if web == nil || web.Drives == nil || len(web.Drives) != 1 || web.Drives[0].Name.ValueString() != "os" || web.Nics == nil || len(web.Nics) != 0 {
		t.Fatalf("web = %#v", web)
	}
	db := byName["db"]
	if db == nil || db.Drives == nil || len(db.Drives) != 1 || db.Drives[0].PreferredTier.ValueInt32() != 4 {
		t.Fatalf("db drives = %#v", db)
	}
	if db.Drives[0].MediaSource == nil || db.Drives[0].MediaSource.Key.ValueInt32() != 91 || db.Drives[0].MediaSource.UsedBytes.ValueInt64() != 8 {
		t.Fatalf("db media source = %#v", db.Drives[0].MediaSource)
	}
	if db.Nics == nil || len(db.Nics) != 1 || db.Nics[0].Vnet.ValueInt32() != 4 || db.Nics[0].Status.ValueString() != "up" {
		t.Fatalf("db nics = %#v", db.Nics)
	}
}
