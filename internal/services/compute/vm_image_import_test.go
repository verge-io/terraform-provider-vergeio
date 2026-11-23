// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"

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

func TestVMImportURLWaitsForCatalogFile(t *testing.T) {
	previous := vmImportPollInterval
	vmImportPollInterval = time.Millisecond
	t.Cleanup(func() { vmImportPollInterval = previous })

	var mu sync.Mutex
	fileGets := 0
	ready := false
	var posted, filePosted string
	postedEarly := false
	deleted := map[string]int{}
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/files":
			body, _ := io.ReadAll(r.Body)
			filePosted = string(body)
			writeJSON(w, http.StatusOK, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			fileGets++
			size := 0
			if fileGets >= 3 {
				size = 48800000
				ready = true
			}
			writeJSON(w, http.StatusOK, fmt.Sprintf(`{"$key":9,"name":"alpine.ova","url":"https://downloads.example.com/alpine.ova","description":"Downloaded for VM import guest","filesize":%d}`, size))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_imports":
			body, _ := io.ReadAll(r.Body)
			posted = string(body)
			if !ready {
				postedEarly = true
			}
			writeJSON(w, http.StatusOK, `{"$key":"`+testImportKey+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46,"file":9}]`)
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
		URL:        types.StringValue("https://downloads.example.com/alpine.ova"),
		DeleteVM:   types.BoolValue(true),
		DeleteFile: types.BoolValue(true),
	}
	if err := api.create(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if postedEarly {
		t.Fatal("vm_imports was posted before the catalog file had a size")
	}
	if !strings.Contains(posted, `"file":9`) || strings.Contains(posted, `"url"`) {
		t.Fatalf("import body = %s", posted)
	}
	if !strings.Contains(filePosted, `"url":"https://downloads.example.com/alpine.ova"`) || !strings.Contains(filePosted, `"description":"Downloaded for VM import guest\nhttps://downloads.example.com/alpine.ova"`) {
		t.Fatalf("file body = %s", filePosted)
	}
	if !data.OwnsFile.ValueBool() || data.FileID.ValueInt64() != 9 || !data.Volume.IsNull() || !data.VolumePath.IsNull() {
		t.Fatalf("import = %#v", data)
	}
	if data.URL.ValueString() != "https://downloads.example.com/alpine.ova" || !data.File.IsNull() {
		t.Fatalf("url import stored the catalog file as the source: file=%v url=%v", data.File, data.URL)
	}
	if err := api.delete(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if deleted["/api/v4/files/9"] != 1 || deleted["/api/v4/vms/46"] != 1 || deleted["/api/v4/vm_imports/"+testImportKey] != 1 {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestVMImportURLGivesUpBeforeImport(t *testing.T) {
	previous := vmImportPollInterval
	vmImportPollInterval = time.Millisecond
	t.Cleanup(func() { vmImportPollInterval = previous })

	postedImport := false
	deletedFile := false
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/files":
			writeJSON(w, http.StatusOK, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"alpine.ova","filesize":0,"allocated_bytes":48800000}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_imports":
			postedImport = true
			http.Error(w, "should not import", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/files/9":
			deletedFile = true
			w.WriteHeader(http.StatusOK)
		default:
			writeImportFixture(w, r)
		}
	})
	err := api.create(context.Background(), &importModel{
		Name:   types.StringValue("guest"),
		Target: types.StringValue(vmImportTargetVM),
		URL:    types.StringValue("https://downloads.example.com/alpine.ova"),
		Timeouts: &importTimeouts{
			Create: types.StringValue("30ms"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "did not finish downloading") {
		t.Fatalf("err = %v", err)
	}
	if postedImport {
		t.Fatal("vm_imports was posted for an empty catalog file")
	}
	if !deletedFile {
		t.Fatal("the unfinished catalog file was left in place")
	}
}

func TestVMImportURLSharesCreateTimeout(t *testing.T) {
	previous := vmImportPollInterval
	vmImportPollInterval = time.Millisecond
	t.Cleanup(func() { vmImportPollInterval = previous })

	const budget = 80 * time.Millisecond
	start := time.Now()
	var firstImport, lastImport time.Time
	deletedImport := false
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/files":
			writeJSON(w, http.StatusOK, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			size := 0
			if time.Since(start) >= 50*time.Millisecond {
				size = 1024
			}
			writeJSON(w, http.StatusOK, fmt.Sprintf(`{"$key":9,"name":"alpine.ova","filesize":%d}`, size))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `{"$key":"`+testImportKey+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			now := time.Now()
			if firstImport.IsZero() {
				firstImport = now
			}
			lastImport = now
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"importing","file":9}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_import_logs":
			writeJSON(w, http.StatusOK, `[]`)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/vm_imports/"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vm_imports/"+testImportKey:
			deletedImport = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/files/9":
			w.WriteHeader(http.StatusOK)
		default:
			writeImportFixture(w, r)
		}
	})
	err := api.create(context.Background(), &importModel{
		Name:   types.StringValue("guest"),
		Target: types.StringValue(vmImportTargetVM),
		URL:    types.StringValue("https://downloads.example.com/alpine.ova"),
		Timeouts: &importTimeouts{
			Create: types.StringValue(budget.String()),
		},
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected the import to stop when the create timeout was spent")
	}
	if !deletedImport {
		t.Fatal("the import posted after the download was left in place")
	}
	importWait := lastImport.Sub(firstImport)
	if importWait > 60*time.Millisecond || elapsed > budget+70*time.Millisecond {
		t.Fatalf("create ran for %s and polled the import for %s; timeouts.create is %s for both steps", elapsed, importWait, budget)
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
	if len(deleted) != 0 {
		t.Fatalf("adopted destroy called the API: %#v", deleted)
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
	if !strings.Contains(posted, `"name":"golden.qcow2"`) || !strings.Contains(posted, `"url":"https://downloads.example.com/golden.qcow2"`) || !strings.Contains(posted, `"description":"Downloaded for media import golden\nhttps://downloads.example.com/golden.qcow2"`) {
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

func TestVMImportReadPopulatesFileSource(t *testing.T) {
	fileGets := 0
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46,"file":18,"preserve_macs":false,"preserve_drive_format":true,"preserve_bios_serial":true,"guest_agent_preinstalled":false,"replicate_tags":true,"preferred_tier":"1","no_optical_drives":true,"override_drive_interface":"virtio","override_nic_interface":"e1000"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/18":
			fileGets++
			writeJSON(w, http.StatusOK, `{"$key":18,"name":"guest.ova","description":"catalog","url":"https://downloads.example.com/guest.ova"}`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{ID: types.StringValue(testImportKey)}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if fileGets != 1 {
		t.Fatalf("file gets = %d", fileGets)
	}
	if data.Name.ValueString() != "guest" || data.File.ValueInt64() != 18 || !data.URL.IsNull() || !data.FileName.IsNull() || !data.Volume.IsNull() || !data.SharedObject.IsNull() {
		t.Fatalf("source = name %v file %v url %v file_name %v volume %v shared %v", data.Name, data.File, data.URL, data.FileName, data.Volume, data.SharedObject)
	}
	if data.PreserveMACs.ValueBool() || !data.PreserveDriveFormat.ValueBool() || !data.PreserveBIOSSerial.ValueBool() || data.GuestAgentPreinstalled.ValueBool() || !data.ReplicateTags.ValueBool() || !data.NoOpticalDrives.ValueBool() {
		t.Fatalf("flags macs=%v format=%v serial=%v agent=%v tags=%v optical=%v", data.PreserveMACs, data.PreserveDriveFormat, data.PreserveBIOSSerial, data.GuestAgentPreinstalled, data.ReplicateTags, data.NoOpticalDrives)
	}
	if data.PreferredTier.ValueString() != "1" || data.OverrideDriveInterface.ValueString() != "virtio" || data.OverrideNICInterface.ValueString() != "e1000" {
		t.Fatalf("tier=%v drive=%v nic=%v", data.PreferredTier, data.OverrideDriveInterface, data.OverrideNICInterface)
	}
	if !data.DeleteVM.ValueBool() || !data.DeleteFile.ValueBool() || data.Adopted.ValueBool() || data.OwnsFile.ValueBool() || data.FileID.ValueInt64() != 18 || data.VMID.ValueInt64() != 46 {
		t.Fatalf("state = %#v", data)
	}
}

func TestVMImportReadRestoresDownloadedURL(t *testing.T) {
	const rawURL = "https://downloads.example.com/guest.ova"
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46,"file":9,"preserve_macs":false}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"guest.ova","description":"Downloaded for VM import guest","url":"`+rawURL+`"}`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{ID: types.StringValue(testImportKey)}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.File.IsNull() || data.URL.ValueString() != rawURL || !data.OwnsFile.ValueBool() || data.FileID.ValueInt64() != 9 {
		t.Fatalf("url=%v file=%v owns=%v file_id=%v", data.URL, data.File, data.OwnsFile, data.FileID)
	}
}

func TestVMImportReadRestoresURLFromDescription(t *testing.T) {
	const rawURL = "https://downloads.example.com/images/guest.ova?token=1"
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46,"file":9,"preserve_macs":false}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"guest.ova","description":"Downloaded for VM import guest\n`+rawURL+`","filesize":10}`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{ID: types.StringValue(testImportKey)}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.File.IsNull() || data.URL.ValueString() != rawURL || !data.OwnsFile.ValueBool() || data.FileID.ValueInt64() != 9 {
		t.Fatalf("url=%v file=%v owns=%v file_id=%v", data.URL, data.File, data.OwnsFile, data.FileID)
	}
}

func TestVMImportReadWithoutStoredURLUsesFile(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46,"file":68}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/68":
			writeJSON(w, http.StatusOK, `{"$key":68,"name":"guest.ova","description":"Downloaded for VM import guest","filesize":10}`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{ID: types.StringValue(testImportKey)}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.File.ValueInt64() != 68 || !data.URL.IsNull() || data.OwnsFile.ValueBool() {
		t.Fatalf("file=%v url=%v owns=%v", data.File, data.URL, data.OwnsFile)
	}
}

func TestVMImportReadIgnoresDescriptionForAnotherName(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","file":9}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"guest.ova","description":"Downloaded for VM import other\nhttps://downloads.example.com/other.ova"}`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{ID: types.StringValue(testImportKey)}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.File.ValueInt64() != 9 || !data.URL.IsNull() || data.OwnsFile.ValueBool() {
		t.Fatalf("file=%v url=%v owns=%v", data.File, data.URL, data.OwnsFile)
	}
}

func TestVMImportReadVolumeAndSharedObject(t *testing.T) {
	const sharedKey = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	fileGets := 0
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports" && r.URL.Query().Get("filter") == "id eq '"+testImportKey+"'":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","volume":"`+testVolumeKey+`","volume_path":"images/guest.ova","file":18}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports" && r.URL.Query().Get("filter") == "id eq '"+sharedKey+"'":
			writeJSON(w, http.StatusOK, `[{"$key":"`+sharedKey+`","id":"`+sharedKey+`","name":"guest","status":"complete","shared_object":8,"file":18}]`)
		case strings.HasPrefix(r.URL.Path, "/api/v4/files/"):
			fileGets++
			http.NotFound(w, r)
		default:
			writeImportFixture(w, r)
		}
	})
	volume := &importModel{ID: types.StringValue(testImportKey)}
	if err := api.read(context.Background(), volume); err != nil {
		t.Fatal(err)
	}
	if volume.Volume.ValueString() != testVolumeKey || volume.VolumePath.ValueString() != "images/guest.ova" || !volume.File.IsNull() || !volume.URL.IsNull() || !volume.SharedObject.IsNull() {
		t.Fatalf("volume source = %#v", volume)
	}
	shared := &importModel{ID: types.StringValue(sharedKey)}
	if err := api.read(context.Background(), shared); err != nil {
		t.Fatal(err)
	}
	if shared.SharedObject.ValueInt64() != 8 || !shared.File.IsNull() || !shared.Volume.IsNull() || !shared.VolumePath.IsNull() {
		t.Fatalf("shared source = %#v", shared)
	}
	if fileGets != 0 {
		t.Fatalf("file gets = %d", fileGets)
	}
}

func TestVMImportReadMissingCatalogFileUsesFileID(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","file":18}]`)
		case r.URL.Path == "/api/v4/files/18":
			http.NotFound(w, r)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{ID: types.StringValue(testImportKey)}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.File.ValueInt64() != 18 || !data.URL.IsNull() || data.OwnsFile.ValueBool() {
		t.Fatalf("file=%v url=%v owns=%v", data.File, data.URL, data.OwnsFile)
	}
}

func TestVMImportReadFileError(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","file":18}]`)
		case r.URL.Path == "/api/v4/files/18":
			http.Error(w, "catalog down", http.StatusInternalServerError)
		default:
			writeImportFixture(w, r)
		}
	})
	err := api.read(context.Background(), &importModel{ID: types.StringValue(testImportKey)})
	if err == nil || !strings.Contains(err.Error(), "catalog down") {
		t.Fatalf("err = %v", err)
	}
}

func TestVMImportReadLeaves262FieldsNullOnOlderServer(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			writeJSON(w, http.StatusOK, `{"version":"26.0.8"}`)
		case "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","file":18,"preserve_macs":true,"preserve_bios_serial":false}]`)
		case "/api/v4/files/18":
			writeJSON(w, http.StatusOK, `{"$key":18,"name":"guest.ova","description":"catalog"}`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{ID: types.StringValue(testImportKey)}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.PreserveMACs.ValueBool() || !data.PreserveBIOSSerial.IsNull() || !data.GuestAgentPreinstalled.IsNull() || !data.ReplicateTags.IsNull() {
		t.Fatalf("macs=%v serial=%v agent=%v tags=%v", data.PreserveMACs, data.PreserveBIOSSerial, data.GuestAgentPreinstalled, data.ReplicateTags)
	}
}

func TestMediaImportReadRestoresURL(t *testing.T) {
	const rawURL = "https://downloads.example.com/golden.qcow2"
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/files/9" {
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"golden.qcow2","description":"Downloaded for media import golden","filesize":4096,"preferred_tier":"4","url":"`+rawURL+`"}`)
			return
		}
		writeImportFixture(w, r)
	})
	data := &importModel{ID: types.StringValue("9")}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.URL.ValueString() != rawURL || data.PreferredTier.ValueString() != "4" || data.Name.ValueString() != "golden.qcow2" || data.Target.ValueString() != vmImportTargetMedia {
		t.Fatalf("media = %#v", data)
	}
	if !data.DeleteVM.ValueBool() || !data.DeleteFile.ValueBool() || !data.OwnsFile.ValueBool() || !data.FileName.IsNull() {
		t.Fatalf("media flags = %#v", data)
	}
}

func TestMediaImportReadRestoresURLFromDescription(t *testing.T) {
	const rawURL = "https://downloads.example.com/images/golden.qcow2?token=1"
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/files/9" {
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"golden.qcow2","description":"Downloaded for media import golden\n`+rawURL+`","filesize":4096}`)
			return
		}
		writeImportFixture(w, r)
	})
	data := &importModel{ID: types.StringValue("9")}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.URL.ValueString() != rawURL || data.Name.ValueString() != "golden" || !data.File.IsNull() || !data.OwnsFile.ValueBool() || data.Target.ValueString() != vmImportTargetMedia {
		t.Fatalf("media = %#v", data)
	}
}

func TestMediaImportRefreshKeepsDownloadWithURLDescription(t *testing.T) {
	const rawURL = "https://downloads.example.com/images/golden.qcow2"
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/files/9" {
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"golden.qcow2","description":"Downloaded for media import golden\n`+rawURL+`","filesize":4096,"preferred_tier":"4"}`)
			return
		}
		writeImportFixture(w, r)
	})
	data := &importModel{
		ID:     types.StringValue("9"),
		Name:   types.StringValue("golden"),
		Target: types.StringValue(vmImportTargetMedia),
		URL:    types.StringValue(rawURL),
	}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.URL.ValueString() != rawURL || data.Name.ValueString() != "golden" || data.PreferredTier.ValueString() != "4" {
		t.Fatalf("media = %#v", data)
	}
}

func TestDownloadDescriptionRejectsLineBreak(t *testing.T) {
	if _, err := downloadDescription(downloadedImportDescription, "guest\nname", "https://downloads.example.com/guest.ova"); err == nil {
		t.Fatal("expected a line break in the name to be rejected")
	}
	if _, err := downloadDescription(downloadedImportDescription, "guest", "https://downloads.example.com/gue\nst.ova"); err == nil {
		t.Fatal("expected a line break in the url to be rejected")
	}
	got, err := downloadDescription(downloadedMediaDescription, "golden", "https://downloads.example.com/golden.qcow2")
	if err != nil {
		t.Fatal(err)
	}
	rec := parseDownloadDescription(got, downloadedMediaDescription)
	if !rec.marker || !rec.urlInDescription || rec.name != "golden" || rec.rawURL != "https://downloads.example.com/golden.qcow2" {
		t.Fatalf("record = %#v", rec)
	}
	plain := parseDownloadDescription("Downloaded for VM import guest", downloadedImportDescription)
	if !plain.marker || plain.urlInDescription || plain.name != "guest" {
		t.Fatalf("plain = %#v", plain)
	}
	crlf := parseDownloadDescription("Downloaded for VM import guest\r\nhttps://downloads.example.com/guest.ova\r\n", downloadedImportDescription)
	if !crlf.urlInDescription || crlf.rawURL != "https://downloads.example.com/guest.ova" || crlf.name != "guest" {
		t.Fatalf("crlf = %#v", crlf)
	}
	if downloadOwned("Downloaded for media import golden extra", downloadedMediaDescription, "golden") {
		t.Fatal("a longer name matched the shorter download")
	}
}

func TestConfiguredImportRefreshStoresOmittedOptions(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/vm_imports" {
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","preserve_macs":true,"preserve_drive_format":false,"preserve_bios_serial":false,"guest_agent_preinstalled":false,"replicate_tags":true,"no_optical_drives":false,"preferred_tier":"4","override_drive_interface":"virtio","override_nic_interface":"e1000","shared_object":8,"volume":"`+testVolumeKey+`","volume_path":"guest.ova"}]`)
			return
		}
		writeImportFixture(w, r)
	})
	data := &importModel{
		ID:     types.StringValue(testImportKey),
		Name:   types.StringValue("guest"),
		Target: types.StringValue(vmImportTargetVM),
		File:   types.Int64Value(12),
	}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !boolIs(data.PreserveMACs, true) || !boolIs(data.PreserveDriveFormat, false) || !boolIs(data.PreserveBIOSSerial, false) || !boolIs(data.GuestAgentPreinstalled, false) || !boolIs(data.ReplicateTags, true) || !boolIs(data.NoOpticalDrives, false) {
		t.Fatalf("flags macs=%v format=%v serial=%v agent=%v tags=%v optical=%v", data.PreserveMACs, data.PreserveDriveFormat, data.PreserveBIOSSerial, data.GuestAgentPreinstalled, data.ReplicateTags, data.NoOpticalDrives)
	}
	if data.PreferredTier.ValueString() != "4" || data.OverrideDriveInterface.ValueString() != "virtio" || data.OverrideNICInterface.ValueString() != "e1000" {
		t.Fatalf("tier=%v drive=%v nic=%v", data.PreferredTier, data.OverrideDriveInterface, data.OverrideNICInterface)
	}
	if !data.SharedObject.IsNull() || !data.Volume.IsNull() || !data.VolumePath.IsNull() || data.File.ValueInt64() != 12 {
		t.Fatalf("file source changed: shared=%v volume=%v path=%v file=%v", data.SharedObject, data.Volume, data.VolumePath, data.File)
	}
	object := tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}
	raw := tftypes.NewValue(object, map[string]tftypes.Value{})
	state := tfsdk.State{Raw: raw}
	plan := tfsdk.Plan{Raw: raw}
	kept := &planmodifier.BoolResponse{}
	importStoredModifier{}.PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolValue(true),
		StateValue:  data.PreserveMACs,
		State:       state,
		Plan:        plan,
	}, kept)
	same := &planmodifier.BoolResponse{PlanValue: kept.PlanValue}
	boolplanmodifier.RequiresReplace().PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolValue(true),
		PlanValue:   kept.PlanValue,
		StateValue:  data.PreserveMACs,
		State:       state,
		Plan:        plan,
	}, same)
	if !kept.PlanValue.ValueBool() || same.RequiresReplace {
		t.Fatalf("stored preserve_macs plan = %#v replace=%v", kept.PlanValue, same.RequiresReplace)
	}
	tier := &planmodifier.StringResponse{}
	importStoredModifier{}.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringValue("4"),
		StateValue:  data.PreferredTier,
		State:       state,
		Plan:        plan,
	}, tier)
	tierReplace := &planmodifier.StringResponse{PlanValue: tier.PlanValue}
	stringplanmodifier.RequiresReplace().PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringValue("4"),
		PlanValue:   tier.PlanValue,
		StateValue:  data.PreferredTier,
		State:       state,
		Plan:        plan,
	}, tierReplace)
	if tier.PlanValue.ValueString() != "4" || tierReplace.RequiresReplace {
		t.Fatalf("stored tier plan = %#v replace=%v", tier.PlanValue, tierReplace.RequiresReplace)
	}
}

func TestVMImportCreateStoresOmittedOptions(t *testing.T) {
	var posted string
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_imports":
			body, _ := io.ReadAll(r.Body)
			posted = string(body)
			writeJSON(w, http.StatusOK, `{"$key":"`+testImportKey+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","vm":46,"file":18,"preserve_macs":true,"preserve_drive_format":false,"preserve_bios_serial":true,"guest_agent_preinstalled":false,"replicate_tags":false,"preferred_tier":"4","no_optical_drives":false,"override_nic_interface":"virtio","volume_path":"guest.ova"}]`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{
		Name:                   types.StringValue("guest"),
		Target:                 types.StringValue(vmImportTargetVM),
		File:                   types.Int64Value(18),
		PreserveMACs:           types.BoolUnknown(),
		PreserveDriveFormat:    types.BoolUnknown(),
		PreserveBIOSSerial:     types.BoolUnknown(),
		GuestAgentPreinstalled: types.BoolUnknown(),
		ReplicateTags:          types.BoolUnknown(),
		NoOpticalDrives:        types.BoolUnknown(),
		PreferredTier:          types.StringUnknown(),
		OverrideDriveInterface: types.StringUnknown(),
		OverrideNICInterface:   types.StringUnknown(),
		VolumePath:             types.StringUnknown(),
		DeleteVM:               types.BoolValue(true),
		DeleteFile:             types.BoolValue(true),
	}
	if err := api.create(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(posted, "preserve_macs") || strings.Contains(posted, "preferred_tier") || strings.Contains(posted, "override_nic_interface") {
		t.Fatalf("omitted options were posted: %s", posted)
	}
	if !boolIs(data.PreserveMACs, true) || !boolIs(data.PreserveDriveFormat, false) || !boolIs(data.PreserveBIOSSerial, true) || !boolIs(data.GuestAgentPreinstalled, false) || !boolIs(data.ReplicateTags, false) || !boolIs(data.NoOpticalDrives, false) {
		t.Fatalf("flags macs=%v format=%v serial=%v agent=%v tags=%v optical=%v", data.PreserveMACs, data.PreserveDriveFormat, data.PreserveBIOSSerial, data.GuestAgentPreinstalled, data.ReplicateTags, data.NoOpticalDrives)
	}
	if data.PreferredTier.ValueString() != "4" || !data.OverrideDriveInterface.IsNull() || data.OverrideNICInterface.ValueString() != "virtio" {
		t.Fatalf("tier=%v drive=%v nic=%v", data.PreferredTier, data.OverrideDriveInterface, data.OverrideNICInterface)
	}
	if !data.VolumePath.IsNull() || !data.Volume.IsNull() || data.File.ValueInt64() != 18 {
		t.Fatalf("file source stored volume path=%v volume=%v file=%v", data.VolumePath, data.Volume, data.File)
	}
}

func TestConfiguredVolumeImportStoresOmittedPath(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/vm_imports" {
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","volume":"`+testVolumeKey+`","volume_path":"images/guest.ova","preferred_tier":"1"}]`)
			return
		}
		writeImportFixture(w, r)
	})
	data := &importModel{
		ID:     types.StringValue(testImportKey),
		Name:   types.StringValue("guest"),
		Target: types.StringValue(vmImportTargetVM),
		Volume: types.StringValue(testVolumeKey),
	}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Volume.ValueString() != testVolumeKey || data.VolumePath.ValueString() != "images/guest.ova" || data.PreferredTier.ValueString() != "1" || !data.File.IsNull() {
		t.Fatalf("volume source = volume %v path %v tier %v file %v", data.Volume, data.VolumePath, data.PreferredTier, data.File)
	}
}

func TestConfiguredImportLeaves262FieldsNullOnOlderServer(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			writeJSON(w, http.StatusOK, `{"version":"26.0.8"}`)
		case "/api/v4/vm_imports":
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","file":18,"preserve_macs":true,"preserve_bios_serial":false,"preferred_tier":"4"}]`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{
		ID:     types.StringValue(testImportKey),
		Name:   types.StringValue("guest"),
		Target: types.StringValue(vmImportTargetVM),
		File:   types.Int64Value(18),
	}
	if err := api.read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.PreserveMACs.ValueBool() || data.PreferredTier.ValueString() != "4" || !data.PreserveBIOSSerial.IsNull() || !data.GuestAgentPreinstalled.IsNull() || !data.ReplicateTags.IsNull() {
		t.Fatalf("macs=%v tier=%v serial=%v agent=%v tags=%v", data.PreserveMACs, data.PreferredTier, data.PreserveBIOSSerial, data.GuestAgentPreinstalled, data.ReplicateTags)
	}
}

func TestVMImportUpdateLeavesOmittedOptionNull(t *testing.T) {
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/vm_imports" {
			writeJSON(w, http.StatusOK, `[{"$key":"`+testImportKey+`","id":"`+testImportKey+`","name":"guest","status":"complete","preserve_macs":true,"preferred_tier":"4"}]`)
			return
		}
		writeImportFixture(w, r)
	})
	state := &importModel{
		ID:         types.StringValue(testImportKey),
		Name:       types.StringValue("guest"),
		Target:     types.StringValue(vmImportTargetVM),
		File:       types.Int64Value(18),
		DeleteVM:   types.BoolValue(true),
		DeleteFile: types.BoolValue(true),
	}
	plan := *state
	plan.DeleteVM = types.BoolValue(false)
	if err := api.update(context.Background(), &plan, state); err != nil {
		t.Fatal(err)
	}
	if !plan.PreserveMACs.IsNull() || !plan.PreferredTier.IsNull() || plan.DeleteVM.ValueBool() {
		t.Fatalf("update stored omitted options: macs=%v tier=%v delete_vm=%v", plan.PreserveMACs, plan.PreferredTier, plan.DeleteVM)
	}
}

func TestMediaImportCreateStoresOmittedTier(t *testing.T) {
	var posted string
	api := newImportTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/files":
			body, _ := io.ReadAll(r.Body)
			posted = string(body)
			writeJSON(w, http.StatusOK, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/files/9":
			writeJSON(w, http.StatusOK, `{"$key":9,"name":"golden.qcow2","description":"Downloaded for media import golden","filesize":4096,"preferred_tier":"4"}`)
		default:
			writeImportFixture(w, r)
		}
	})
	data := &importModel{
		Name:          types.StringValue("golden"),
		Target:        types.StringValue(vmImportTargetMedia),
		URL:           types.StringValue("https://downloads.example.com/golden.qcow2"),
		PreferredTier: types.StringUnknown(),
		PreserveMACs:  types.BoolUnknown(),
		DeleteVM:      types.BoolValue(true),
		DeleteFile:    types.BoolValue(true),
	}
	if err := api.create(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(posted, `"preferred_tier":"4"`) {
		t.Fatalf("omitted tier was posted: %s", posted)
	}
	if data.PreferredTier.ValueString() != "4" || !data.PreserveMACs.IsNull() || data.PreserveMACs.IsUnknown() {
		t.Fatalf("tier=%v macs=%v", data.PreferredTier, data.PreserveMACs)
	}
}

func TestFileReadyRequiresFilesize(t *testing.T) {
	if fileReady(&vergeos.File{AllocatedBytes: 4096, UsedBytes: 512}) {
		t.Fatal("reserved space is not a finished download")
	}
	if !fileReady(&vergeos.File{Filesize: 4096}) {
		t.Fatal("a logical size means the file has content")
	}
	if fileReady(nil) || fileReady(&vergeos.File{}) {
		t.Fatal("an empty file is not ready")
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

func TestImportStoredAttributesAreComputed(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewVMImportResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	for _, name := range []string{
		"volume_path",
		"preserve_macs",
		"preserve_drive_format",
		"preserve_bios_serial",
		"guest_agent_preinstalled",
		"replicate_tags",
		"preferred_tier",
		"no_optical_drives",
		"override_drive_interface",
		"override_nic_interface",
	} {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		computed := false
		switch a := attr.(type) {
		case schema.BoolAttribute:
			computed = a.Computed
		case schema.StringAttribute:
			computed = a.Computed
		default:
			t.Fatalf("%s has type %T", name, attr)
		}
		if !computed {
			t.Fatalf("%s keeps a stored value when the configuration omits it, so it must be computed", name)
		}
	}
}

func TestImportStoredModifier(t *testing.T) {
	object := tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}
	raw := tftypes.NewValue(object, map[string]tftypes.Value{})
	state := tfsdk.State{Raw: raw}
	plan := tfsdk.Plan{Raw: raw}
	kept := &planmodifier.BoolResponse{}
	importStoredModifier{}.PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolValue(false),
		State:       state,
		Plan:        plan,
	}, kept)
	if kept.PlanValue.IsNull() || kept.PlanValue.IsUnknown() || kept.PlanValue.ValueBool() {
		t.Fatalf("omitted flag plan = %#v", kept.PlanValue)
	}
	same := &planmodifier.BoolResponse{PlanValue: kept.PlanValue}
	boolplanmodifier.RequiresReplace().PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolNull(),
		PlanValue:   kept.PlanValue,
		StateValue:  types.BoolValue(false),
		State:       state,
		Plan:        plan,
	}, same)
	if same.RequiresReplace {
		t.Fatal("an omitted flag must not replace the import")
	}
	changed := &planmodifier.BoolResponse{}
	importStoredModifier{}.PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolValue(true),
		StateValue:  types.BoolValue(false),
		State:       state,
		Plan:        plan,
	}, changed)
	replace := &planmodifier.BoolResponse{PlanValue: changed.PlanValue}
	boolplanmodifier.RequiresReplace().PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolValue(true),
		PlanValue:   changed.PlanValue,
		StateValue:  types.BoolValue(false),
		State:       state,
		Plan:        plan,
	}, replace)
	if !changed.PlanValue.ValueBool() || !replace.RequiresReplace {
		t.Fatalf("changed plan = %#v replace=%v", changed.PlanValue, replace.RequiresReplace)
	}
	tier := &planmodifier.StringResponse{}
	importStoredModifier{}.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		StateValue:  types.StringValue("1"),
		State:       state,
	}, tier)
	if tier.PlanValue.ValueString() != "1" {
		t.Fatalf("omitted tier plan = %#v", tier.PlanValue)
	}
	cleared := &planmodifier.StringResponse{}
	importStoredModifier{}.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		StateValue:  types.StringNull(),
		State:       state,
	}, cleared)
	if !cleared.PlanValue.IsNull() {
		t.Fatalf("unset tier plan = %#v", cleared.PlanValue)
	}
	nullState := tfsdk.State{Raw: tftypes.NewValue(object, nil)}
	created := &planmodifier.StringResponse{}
	importStoredModifier{}.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		StateValue:  types.StringNull(),
		State:       nullState,
	}, created)
	if !created.PlanValue.IsUnknown() {
		t.Fatalf("create tier plan = %#v", created.PlanValue)
	}
	createReplace := &planmodifier.StringResponse{PlanValue: created.PlanValue}
	stringplanmodifier.RequiresReplace().PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		PlanValue:   created.PlanValue,
		StateValue:  types.StringNull(),
		State:       nullState,
		Plan:        tfsdk.Plan{Raw: tftypes.NewValue(object, nil)},
	}, createReplace)
	if createReplace.RequiresReplace {
		t.Fatal("create must not replace the import")
	}
	sameBool := &planmodifier.BoolResponse{}
	importStoredModifier{}.PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolValue(false),
		StateValue:  types.BoolValue(false),
		State:       state,
		Plan:        plan,
	}, sameBool)
	sameReplace := &planmodifier.BoolResponse{PlanValue: sameBool.PlanValue}
	boolplanmodifier.RequiresReplace().PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolValue(false),
		PlanValue:   sameBool.PlanValue,
		StateValue:  types.BoolValue(false),
		State:       state,
		Plan:        plan,
	}, sameReplace)
	if sameBool.PlanValue.ValueBool() || sameReplace.RequiresReplace {
		t.Fatalf("stored false plan = %#v replace=%v", sameBool.PlanValue, sameReplace.RequiresReplace)
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

func boolIs(v types.Bool, want bool) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueBool() == want
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
