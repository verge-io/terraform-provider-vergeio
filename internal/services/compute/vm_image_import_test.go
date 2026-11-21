// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

const (
	testImportKey = "8f73f8bcc9c9f1aaba32f733bfc295acaf548554"
	testVolumeKey = "0123456789abcdef0123456789abcdef01234567"
)

func TestValidateImportModelSources(t *testing.T) {
	base := importModel{Name: types.StringValue("guest"), Target: types.StringValue(vmImportTargetVM)}
	if diags := validateImportModel(&base); !diags.HasError() {
		t.Fatal("expected an error when no source is set")
	}
	both := base
	both.File = types.Int64Value(4)
	both.URL = types.StringValue("https://downloads.example.com/guest.ova")
	if diags := validateImportModel(&both); !diags.HasError() {
		t.Fatal("expected an error when two sources are set")
	}
	pathOnly := base
	pathOnly.VolumePath = types.StringValue("guest.ova")
	if diags := validateImportModel(&pathOnly); !diags.HasError() {
		t.Fatal("expected volume_path to require volume")
	}
	disk := base
	disk.URL = types.StringValue("https://downloads.example.com/disk.qcow2")
	if diags := validateImportModel(&disk); !diags.HasError() {
		t.Fatal("expected a disk image URL to be rejected for a VM import")
	}
	media := importModel{
		Name:   types.StringValue("golden"),
		Target: types.StringValue(vmImportTargetMedia),
		URL:    types.StringValue("https://downloads.example.com/golden.qcow2"),
		File:   types.Int64Value(4),
	}
	if diags := validateImportModel(&media); !diags.HasError() {
		t.Fatal("expected a media import to reject file")
	}
	media.File = types.Int64Null()
	if diags := validateImportModel(&media); diags.HasError() {
		t.Fatal(diags)
	}
	ova := base
	ova.URL = types.StringValue("https://downloads.example.com/guest.ova")
	ova.PreferredTier = types.StringValue("3")
	if diags := validateImportModel(&ova); diags.HasError() {
		t.Fatal(diags)
	}
}

func TestVersionAtLeast(t *testing.T) {
	if !versionAtLeast("26.2.0", 26, 2) || !versionAtLeast("26.2.2.1", 26, 2) {
		t.Fatal("26.2 should pass")
	}
	if versionAtLeast("26.1.9", 26, 2) || versionAtLeast("26.2.0-rc1", 26, 2) || versionAtLeast("", 26, 2) {
		t.Fatal("older and prerelease versions should not pass")
	}
}

func TestFileNameFromImportURL(t *testing.T) {
	name, err := fileNameFromImportURL("https://downloads.example.com/images/guest.ova?token=1")
	if err != nil || name != "guest.ova" {
		t.Fatalf("name = %q %v", name, err)
	}
	if _, err := fileNameFromImportURL("https://downloads.example.com/images/"); err == nil {
		t.Fatal("expected a directory URL to be rejected")
	}
}

func TestVMImportCreateFromFileAndDelete(t *testing.T) {
	var mu sync.Mutex
	var posted string
	deleted := map[string]int{}
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_imports":
			body, _ := io.ReadAll(r.Body)
			posted = string(body)
			writeJSON(w, http.StatusOK, `{"$key":"`+testImportKey+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46,"file":12,"preserve_macs":false,"preferred_tier":"3"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/46":
			writeJSON(w, http.StatusOK, `{"running":false,"status":"stopped"}`)
		case r.Method == http.MethodDelete:
			deleted[r.URL.Path]++
			w.WriteHeader(http.StatusOK)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{
		Name:          types.StringValue("guest"),
		Target:        types.StringValue(vmImportTargetVM),
		File:          types.Int64Value(12),
		PreferredTier: types.StringValue("3"),
		DeleteVM:      types.BoolValue(true),
		DeleteFile:    types.BoolValue(true),
	}
	if err := api.create(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ID.ValueString() != testImportKey || data.VMID.ValueInt64() != 46 || data.Status.ValueString() != "complete" {
		t.Fatalf("import = %#v", data)
	}
	if data.Adopted.ValueBool() || data.OwnsFile.ValueBool() {
		t.Fatalf("adopted=%v owns_file=%v", data.Adopted.ValueBool(), data.OwnsFile.ValueBool())
	}
	if !strings.Contains(posted, `"file":12`) || !strings.Contains(posted, `"preferred_tier":"3"`) || strings.Contains(posted, "preserve_bios_serial") {
		t.Fatalf("posted body = %s", posted)
	}
	if err := api.delete(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if deleted["/api/v4/vms/46"] != 1 || deleted["/api/v4/vm_imports/"+testImportKey] != 1 {
		t.Fatalf("deleted = %#v", deleted)
	}
	if deleted["/api/v4/files/12"] != 0 {
		t.Fatal("a file the configuration passed in should stay")
	}
}

func TestVMImportDiskURLDoesNotPost(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Errorf("posted %s", r.URL.Path)
		}
		writeImportFixture(w, r)
	})
	err := api.create(context.Background(), &importModel{
		Name:   types.StringValue("guest"),
		Target: types.StringValue(vmImportTargetVM),
		URL:    types.StringValue("https://downloads.example.com/disk.qcow2"),
	})
	if err == nil || !strings.Contains(err.Error(), "target = \"media\"") {
		t.Fatalf("err = %v", err)
	}
}

func TestVMImportFailureCleansUp(t *testing.T) {
	deleted := map[string]int{}
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/files":
			writeJSON(w, http.StatusOK, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"guest.ova","url":"https://downloads.example.com/guest.ova","description":"Downloaded for VM import guest","filesize":10}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `{"$key":"`+testImportKey+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"error","status_info":"bad ova","vm":46,"file":9}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_import_logs":
			writeJSON(w, http.StatusOK, `[{"$key":1,"vm_import":"`+testImportKey+`","level":"error","text":"bad ova"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/46":
			writeJSON(w, http.StatusOK, `{"running":false,"status":"stopped"}`)
		case r.Method == http.MethodDelete:
			deleted[r.URL.Path]++
			w.WriteHeader(http.StatusOK)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{
		Name:       types.StringValue("guest"),
		Target:     types.StringValue(vmImportTargetVM),
		URL:        types.StringValue("https://downloads.example.com/guest.ova"),
		DeleteVM:   types.BoolValue(true),
		DeleteFile: types.BoolValue(true),
	}
	err := api.create(context.Background(), data)
	if err == nil || !strings.Contains(err.Error(), "bad ova") {
		t.Fatalf("err = %v", err)
	}
	if importIDSet(data) {
		t.Fatal("cleaned up import should not stay in state")
	}
	if deleted["/api/v4/vms/46"] != 1 || deleted["/api/v4/vm_imports/"+testImportKey] != 1 || deleted["/api/v4/files/9"] != 1 {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestVMImportAdoptedVMStays(t *testing.T) {
	deleted := map[string]int{}
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/vms") && r.URL.Query().Get("filter") != "":
			writeJSON(w, http.StatusOK, `[{"$key":46,"name":"guest"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/46":
			writeJSON(w, http.StatusOK, `{"$key":46,"name":"guest"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			if r.URL.Query().Get("filter") == "name eq 'guest'" {
				writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46}]`)
				return
			}
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46}]`)
		case r.Method == http.MethodDelete:
			deleted[r.URL.Path]++
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost:
			t.Errorf("posted %s", r.URL.Path)
			http.NotFound(w, r)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{
		Name:     types.StringValue("guest"),
		Target:   types.StringValue(vmImportTargetVM),
		File:     types.Int64Value(12),
		DeleteVM: types.BoolValue(true),
	}
	if err := api.create(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.Adopted.ValueBool() {
		t.Fatal("expected the existing VM to be adopted")
	}
	if err := api.delete(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if deleted["/api/v4/vms/46"] != 0 {
		t.Fatalf("adopted VM was deleted: %#v", deleted)
	}
	if deleted["/api/v4/vm_imports/"+testImportKey] != 1 {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestMediaImportCreate(t *testing.T) {
	var posted string
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/files":
			body, _ := io.ReadAll(r.Body)
			posted = string(body)
			writeJSON(w, http.StatusOK, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"golden.qcow2","description":"Downloaded for media import golden","filesize":4096,"preferred_tier":"4","url":"https://downloads.example.com/golden.qcow2"}`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{
		Name:          types.StringValue("golden"),
		Target:        types.StringValue(vmImportTargetMedia),
		URL:           types.StringValue("https://downloads.example.com/golden.qcow2"),
		FileName:      types.StringValue("golden.qcow2"),
		PreferredTier: types.StringValue("4"),
		DeleteVM:      types.BoolValue(true),
		DeleteFile:    types.BoolValue(true),
	}
	if err := api.create(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ID.ValueString() != "9" || data.FileID.ValueInt64() != 9 || !data.OwnsFile.ValueBool() || data.Status.ValueString() != "complete" {
		t.Fatalf("media = %#v", data)
	}
	if !strings.Contains(posted, `"name":"golden.qcow2"`) || !strings.Contains(posted, `"url":"https://downloads.example.com/golden.qcow2"`) {
		t.Fatalf("posted = %s", posted)
	}
	if strings.Contains(posted, "vm_imports") {
		t.Fatal("a disk image should not be posted as a VM import")
	}
}

func TestVMImportDataSourceWaits(t *testing.T) {
	previous := vmImportPollInterval
	vmImportPollInterval = time.Millisecond
	t.Cleanup(func() { vmImportPollInterval = previous })
	calls := 0
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/vm_imports" && r.Method == http.MethodGet {
			calls++
			status := "importing"
			if calls > 1 {
				status = "complete"
			}
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"`+status+`","vm":46}]`)
			return
		}
		writeImportFixture(w, r)
	})
	data := &importDataModel{ID: types.StringValue(testImportKey), Wait: types.BoolValue(true), Timeout: types.StringValue("1s")}
	if err := api.readData(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Status.ValueString() != "complete" || data.VMID.ValueInt64() != 46 || data.Name.ValueString() != "guest" {
		t.Fatalf("data = %#v", data)
	}
}

func TestVMImportDataSourceFailedRow(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"error","status_info":"bad ova"}]`)
		case "/api/v4/vm_import_logs":
			writeJSON(w, http.StatusOK, `[]`)
		default:
			writeImportFixture(w, r)
		}
	})
	err := api.readData(context.Background(), &importDataModel{ID: types.StringValue(testImportKey), Wait: types.BoolValue(true)})
	if err == nil || !strings.Contains(err.Error(), "bad ova") {
		t.Fatalf("err = %v", err)
	}
}

func TestVMImportDataSourceReadsCurrentRow(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/vm_imports" {
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"importing"}]`)
			return
		}
		writeImportFixture(w, r)
	})
	data := &importDataModel{Name: types.StringValue("guest"), Wait: types.BoolValue(false)}
	if err := api.readData(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Status.ValueString() != "importing" || data.ID.ValueString() != testImportKey {
		t.Fatalf("data = %#v", data)
	}
	if diags := validateImportData(&importDataModel{
		ID:   types.StringValue(testImportKey),
		Name: types.StringValue("guest"),
	}); !diags.HasError() {
		t.Fatal("expected id and name together to be rejected")
	}
}

func TestVMImportKeeps262FieldsOnOlderServer(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version.json" {
			writeJSON(w, http.StatusOK, `{"version":"26.0.8"}`)
			return
		}
		if r.URL.Path == "/api/v4/vm_imports" {
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","preserve_bios_serial":false}]`)
			return
		}
		writeImportFixture(w, r)
	})
	data := &importModel{
		ID:                 types.StringValue(testImportKey),
		Name:               types.StringValue("guest"),
		Target:             types.StringValue(vmImportTargetVM),
		PreserveBIOSSerial: types.BoolValue(true),
	}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.PreserveBIOSSerial.ValueBool() {
		t.Fatal("26.0 should keep the configured 26.2 field")
	}
}

func TestVMImportImportState(t *testing.T) {
	if !validImportID(testImportKey) || !validImportID("9") {
		t.Fatal("expected hex and file ids to be accepted")
	}
	for _, id := range []string{"", "abc", "0", "-1"} {
		if validImportID(id) {
			t.Fatalf("accepted %q", id)
		}
	}
}

func TestVMExportCreateUpdateDelete(t *testing.T) {
	var created, updated string
	api := newExportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/volume_vm_exports":
			body, _ := io.ReadAll(r.Body)
			created = string(body)
			writeJSON(w, http.StatusOK, `{"$key":4}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/volume_vm_exports/4":
			body, _ := io.ReadAll(r.Body)
			updated = string(body)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/volume_vm_exports/4":
			max := 2
			quiesced := false
			if strings.Contains(updated, `"quiesced":true`) {
				quiesced = true
				max = 3
			}
			payload := `{"$key":4,"volume":"` + testVolumeKey + `","volume_name":"files","quiesced":` + exportBoolJSON(quiesced) + `,"create_current":true,"max_exports":` + strconv.Itoa(max) + `,"status":"idle"}`
			writeJSON(w, http.StatusOK, payload)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/volume_vm_exports/4":
			w.WriteHeader(http.StatusOK)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &exportModel{
		Volume:        types.StringValue(testVolumeKey),
		Quiesced:      types.BoolValue(false),
		CreateCurrent: types.BoolValue(true),
		MaxExports:    types.Int64Value(2),
	}
	if err := api.create(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ID.ValueString() != "4" || data.VolumeName.ValueString() != "files" || data.Status.ValueString() != "idle" {
		t.Fatalf("export = %#v", data)
	}
	if !strings.Contains(created, `"volume":"`+testVolumeKey+`"`) || strings.Contains(created, "vms") {
		t.Fatalf("create body = %s", created)
	}
	plan := *data
	plan.Quiesced = types.BoolValue(true)
	plan.MaxExports = types.Int64Value(3)
	if err := api.update(context.Background(), &plan, data); err != nil {
		t.Fatal(err)
	}
	if !plan.Quiesced.ValueBool() || plan.MaxExports.ValueInt64() != 3 {
		t.Fatalf("updated = %#v body %s", plan, updated)
	}
	if err := api.delete(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
}

func TestVMExportRejectsReusedKey(t *testing.T) {
	api := newExportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/volume_vm_exports/4" {
			writeJSON(w, http.StatusOK, `{"$key":4,"volume":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":"idle"}`)
			return
		}
		writeImportFixture(w, r)
	})
	err := api.read(context.Background(), &exportModel{
		ID:     types.StringValue("4"),
		Volume: types.StringValue(testVolumeKey),
	})
	if err == nil {
		t.Fatal("expected a reused export key to be treated as missing")
	}
}

func newImportTestAPI(t *testing.T, handler http.HandlerFunc) *importAPI {
	t.Helper()
	api, err := newImportAPI(testVergeClient(t, handler))
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func newExportTestAPI(t *testing.T, handler http.HandlerFunc) *exportAPI {
	t.Helper()
	api, err := newExportAPI(testVergeClient(t, handler))
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func testVergeClient(t *testing.T, handler http.HandlerFunc) *vergeio.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return vergeio.NewClient(server.URL, "user", "pass", true)
}

func writeImportFixture(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/version.json":
		writeJSON(w, http.StatusOK, `{"version":"26.2.0"}`)
	case "/api/v4/clusters":
		writeJSON(w, http.StatusOK, `[]`)
	case "/api/v4/vms":
		writeJSON(w, http.StatusOK, `[]`)
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func exportBoolJSON(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
