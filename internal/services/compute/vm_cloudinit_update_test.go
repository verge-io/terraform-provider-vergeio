// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

const (
	cloudInitFirst  = "#cloud-config\nhostname: first\n"
	cloudInitSecond = "#cloud-config\nhostname: second\n"
	cloudInitMeta   = "instance-id: iid-local01\n"
	cloudInitVendor = "vendor: extra\n"
)

func TestCloudInitFilesEqualTreatsNilAndEmptyAsDifferent(t *testing.T) {
	file := cloudInitFile("/user-data", cloudInitFirst)
	if !cloudInitFilesEqual(nil, nil) {
		t.Fatal("nil lists should match")
	}
	if cloudInitFilesEqual(nil, []CloudInitFile{}) {
		t.Fatal("nil and empty lists should differ")
	}
	if cloudInitFilesEqual([]CloudInitFile{file}, []CloudInitFile{cloudInitFile("/user-data", cloudInitSecond)}) {
		t.Fatal("different contents should differ")
	}
	reordered := []CloudInitFile{cloudInitFile("/meta-data", cloudInitMeta), file}
	original := []CloudInitFile{file, cloudInitFile("/meta-data", cloudInitMeta)}
	if cloudInitFilesEqual(reordered, original) {
		t.Fatal("order should matter")
	}
}

func TestIndexCloudInitFilesRejectsBlankAndDuplicateNames(t *testing.T) {
	_, err := indexCloudInitFiles([]CloudInitFile{
		{Name: types.StringValue("  "), Contents: types.StringValue("a")},
	})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("blank name error = %v", err)
	}

	_, err = indexCloudInitFiles([]CloudInitFile{
		cloudInitFile("/user-data", "a"),
		cloudInitFile("/user-data", "b"),
	})
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate name error = %v", err)
	}

	_, err = indexCloudInitFiles([]CloudInitFile{
		{Name: types.StringUnknown(), Contents: types.StringValue("a")},
	})
	if err == nil || !strings.Contains(err.Error(), "not known") {
		t.Fatalf("unknown name error = %v", err)
	}
}

// TestUpdateCloudInitFileContents is the apply that edits /user-data on a VM
// that already has the file. The VM PUT cannot carry cloudinit_files, so the
// file row has to be updated and the saved state has to be the new body.
func TestUpdateCloudInitFileContents(t *testing.T) {
	fake := newFakeCloudInit(
		fakeCloudInitRow{key: 11, name: "/user-data", contents: cloudInitFirst},
		fakeCloudInitRow{key: 12, name: "/meta-data", contents: cloudInitMeta},
	)
	got := runCloudInitUpdate(t, fake,
		[]CloudInitFile{cloudInitFile("/user-data", cloudInitFirst), cloudInitFile("/meta-data", cloudInitMeta)},
		[]CloudInitFile{cloudInitFile("/user-data", cloudInitSecond), cloudInitFile("/meta-data", cloudInitMeta)},
	)

	puts := fake.calls(http.MethodPut, "/api/v4/cloudinit_files/11")
	if len(puts) != 1 {
		t.Fatalf("content updates = %#v, want one PUT for /user-data", fake.cloudInitCalls())
	}
	var body struct {
		Name     *string `json:"name"`
		Contents *string `json:"contents"`
		Owner    *string `json:"owner"`
	}
	if err := json.Unmarshal([]byte(puts[0].body), &body); err != nil {
		t.Fatal(err)
	}
	if body.Contents == nil || *body.Contents != cloudInitSecond {
		t.Fatalf("contents = %#v, want the new body", body.Contents)
	}
	if body.Name != nil || body.Owner != nil {
		t.Fatalf("update changed name or owner: %#v", body)
	}
	if len(fake.calls(http.MethodPut, "/api/v4/cloudinit_files/12")) != 0 {
		t.Fatal("unchanged /meta-data was updated")
	}
	if len(fake.calls(http.MethodPost, "/api/v4/cloudinit_files")) != 0 || len(fake.calls(http.MethodDelete, "")) != 0 {
		t.Fatalf("contents change also created or deleted files: %#v", fake.cloudInitCalls())
	}
	assertCloudInitFiles(t, got, []CloudInitFile{
		cloudInitFile("/user-data", cloudInitSecond),
		cloudInitFile("/meta-data", cloudInitMeta),
	})
	for _, call := range fake.calls(http.MethodPut, "/api/v4/vms/7") {
		if strings.Contains(call.body, "cloudinit_files") || strings.Contains(call.body, "hostname: second") {
			t.Fatalf("VM update carried cloud-init files: %s", call.body)
		}
	}
	if !strings.Contains(fake.listFilters(), "owner eq 'vms/7'") {
		t.Fatalf("list filter = %q, want this VM", fake.listFilters())
	}
}

func TestUpdateCloudInitFilesCreatesAndDeletesRows(t *testing.T) {
	fake := newFakeCloudInit(
		fakeCloudInitRow{key: 11, name: "/user-data", contents: cloudInitFirst},
		fakeCloudInitRow{key: 12, name: "/meta-data", contents: cloudInitMeta},
	)
	got := runCloudInitUpdate(t, fake,
		[]CloudInitFile{cloudInitFile("/user-data", cloudInitFirst), cloudInitFile("/meta-data", cloudInitMeta)},
		[]CloudInitFile{cloudInitFile("/meta-data", cloudInitMeta), cloudInitFile("/vendor-data", cloudInitVendor)},
	)

	if len(fake.calls(http.MethodDelete, "/api/v4/cloudinit_files/11")) != 1 {
		t.Fatalf("calls = %#v, want /user-data deleted", fake.cloudInitCalls())
	}
	if len(fake.calls(http.MethodDelete, "/api/v4/cloudinit_files/12")) != 0 {
		t.Fatal("kept /meta-data was deleted")
	}
	if len(fake.calls(http.MethodPut, "/api/v4/cloudinit_files/12")) != 0 {
		t.Fatal("unchanged /meta-data was updated")
	}
	posts := fake.calls(http.MethodPost, "/api/v4/cloudinit_files")
	if len(posts) != 1 {
		t.Fatalf("creates = %#v, want one", fake.cloudInitCalls())
	}
	var body struct {
		Name     string `json:"name"`
		Contents string `json:"contents"`
		Owner    string `json:"owner"`
	}
	if err := json.Unmarshal([]byte(posts[0].body), &body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "/vendor-data" || body.Contents != cloudInitVendor || body.Owner != "vms/7" {
		t.Fatalf("create body = %#v", body)
	}
	assertCloudInitFiles(t, got, []CloudInitFile{
		cloudInitFile("/meta-data", cloudInitMeta),
		cloudInitFile("/vendor-data", cloudInitVendor),
	})
}

func TestUpdateCloudInitFileRecreatesAMissingRow(t *testing.T) {
	fake := newFakeCloudInit()
	got := runCloudInitUpdate(t, fake,
		[]CloudInitFile{cloudInitFile("/user-data", cloudInitFirst)},
		[]CloudInitFile{cloudInitFile("/user-data", cloudInitSecond)},
	)
	posts := fake.calls(http.MethodPost, "/api/v4/cloudinit_files")
	if len(posts) != 1 {
		t.Fatalf("calls = %#v, want the missing file created", fake.cloudInitCalls())
	}
	for _, call := range fake.calls(http.MethodPut, "") {
		if strings.Contains(call.path, "/cloudinit_files") {
			t.Fatalf("missing file was updated instead of created: %#v", fake.cloudInitCalls())
		}
	}
	assertCloudInitFiles(t, got, []CloudInitFile{cloudInitFile("/user-data", cloudInitSecond)})
}

func TestUpdateDeletesCloudInitFilesWhenCleared(t *testing.T) {
	fake := newFakeCloudInit(fakeCloudInitRow{key: 11, name: "/user-data", contents: cloudInitFirst})
	got := runCloudInitUpdate(t, fake,
		[]CloudInitFile{cloudInitFile("/user-data", cloudInitFirst)},
		nil,
	)
	if len(fake.calls(http.MethodDelete, "/api/v4/cloudinit_files/11")) != 1 {
		t.Fatalf("calls = %#v, want the file deleted", fake.cloudInitCalls())
	}
	if got.CloudInitFiles != nil {
		t.Fatalf("state files = %#v, want none", got.CloudInitFiles)
	}
}

func TestUpdateLeavesCloudInitFilesWhenUnchanged(t *testing.T) {
	files := []CloudInitFile{cloudInitFile("/user-data", cloudInitFirst)}
	fake := newFakeCloudInit(fakeCloudInitRow{key: 11, name: "/user-data", contents: cloudInitFirst})
	got := runCloudInitUpdate(t, fake, files, files)
	if len(fake.cloudInitCalls()) != 0 {
		t.Fatalf("unchanged files were sent: %#v", fake.cloudInitCalls())
	}
	assertCloudInitFiles(t, got, files)
}

func TestUpdateCloudInitFileReorderDoesNotRewriteRows(t *testing.T) {
	fake := newFakeCloudInit(
		fakeCloudInitRow{key: 11, name: "/user-data", contents: cloudInitFirst},
		fakeCloudInitRow{key: 12, name: "/meta-data", contents: cloudInitMeta},
	)
	state := []CloudInitFile{cloudInitFile("/user-data", cloudInitFirst), cloudInitFile("/meta-data", cloudInitMeta)}
	plan := []CloudInitFile{cloudInitFile("/meta-data", cloudInitMeta), cloudInitFile("/user-data", cloudInitFirst)}
	got := runCloudInitUpdate(t, fake, state, plan)
	for _, call := range fake.cloudInitCalls() {
		if call.method != http.MethodGet {
			t.Fatalf("reorder wrote a file row: %#v", fake.cloudInitCalls())
		}
	}
	assertCloudInitFiles(t, got, plan)
}

func TestUpdateCloudInitFileReportsWriteFailure(t *testing.T) {
	fake := newFakeCloudInit(fakeCloudInitRow{key: 11, name: "/user-data", contents: cloudInitFirst})
	fake.failPut = true
	resp := runCloudInitUpdateExpectErr(t, fake,
		[]CloudInitFile{cloudInitFile("/user-data", cloudInitFirst)},
		[]CloudInitFile{cloudInitFile("/user-data", cloudInitSecond)},
	)
	msg := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(msg, "updating cloud-init file") || !strings.Contains(msg, "/user-data") {
		t.Fatalf("error = %q", msg)
	}
	if len(fake.calls(http.MethodPut, "/api/v4/cloudinit_files/11")) != 1 {
		t.Fatalf("calls = %#v, want the failed update", fake.cloudInitCalls())
	}
}

func cloudInitFile(name, contents string) CloudInitFile {
	return CloudInitFile{Name: types.StringValue(name), Contents: types.StringValue(contents)}
}

func assertCloudInitFiles(t *testing.T, got VMResourceModel, want []CloudInitFile) {
	t.Helper()
	if len(got.CloudInitFiles) != len(want) {
		t.Fatalf("state has %d files, want %d (%#v)", len(got.CloudInitFiles), len(want), got.CloudInitFiles)
	}
	for i := range want {
		if got.CloudInitFiles[i].Name.ValueString() != want[i].Name.ValueString() ||
			got.CloudInitFiles[i].Contents.ValueString() != want[i].Contents.ValueString() {
			t.Fatalf("file %d = %q %q, want %q %q", i,
				got.CloudInitFiles[i].Name.ValueString(), got.CloudInitFiles[i].Contents.ValueString(),
				want[i].Name.ValueString(), want[i].Contents.ValueString())
		}
	}
}

type fakeCloudInitRow struct {
	key      int
	name     string
	contents string
}

type fakeCloudInitCall struct {
	method string
	path   string
	filter string
	body   string
}

type fakeCloudInit struct {
	mu       sync.Mutex
	files    []fakeCloudInitRow
	nextKey  int
	recorded []fakeCloudInitCall
	failPut  bool
}

func newFakeCloudInit(rows ...fakeCloudInitRow) *fakeCloudInit {
	fake := &fakeCloudInit{files: append([]fakeCloudInitRow(nil), rows...), nextKey: 30}
	for _, row := range rows {
		if row.key >= fake.nextKey {
			fake.nextKey = row.key + 1
		}
	}
	return fake
}

func (f *fakeCloudInit) calls(method, path string) []fakeCloudInitCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeCloudInitCall
	for _, call := range f.recorded {
		if call.method != method {
			continue
		}
		if path != "" && call.path != path {
			continue
		}
		out = append(out, call)
	}
	return out
}

func (f *fakeCloudInit) cloudInitCalls() []fakeCloudInitCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeCloudInitCall
	for _, call := range f.recorded {
		if strings.Contains(call.path, "/cloudinit_files") {
			out = append(out, call)
		}
	}
	return out
}

func (f *fakeCloudInit) listFilters() string {
	var filters []string
	for _, call := range f.calls(http.MethodGet, "/api/v4/cloudinit_files") {
		filters = append(filters, call.filter)
	}
	return strings.Join(filters, ";")
}

func (f *fakeCloudInit) handler(t *testing.T) http.Handler {
	t.Helper()
	const vmJSON = `{"$key":7,"machine":1,"name":"web","cpu_cores":1,"ram":512,"enabled":true,"powerstate":false,"running":false,"status":"stopped"}`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.recorded = append(f.recorded, fakeCloudInitCall{
			method: r.Method,
			path:   r.URL.Path,
			filter: r.URL.Query().Get("filter"),
			body:   string(body),
		})

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vms/7":
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			_, _ = w.Write([]byte(vmJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/cloudinit_files":
			payload := make([]map[string]any, 0, len(f.files))
			for _, file := range f.files {
				payload = append(payload, map[string]any{"$key": file.key, "name": file.name, "owner": "vms/7"})
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Errorf("marshal files: %v", err)
				return
			}
			_, _ = w.Write(encoded)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/cloudinit_files":
			var req struct {
				Name     string `json:"name"`
				Contents string `json:"contents"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("create body: %v", err)
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			key := f.nextKey
			f.nextKey++
			f.files = append(f.files, fakeCloudInitRow{key: key, name: req.Name, contents: req.Contents})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"$key":` + strconv.Itoa(key) + `}`))
		case strings.HasPrefix(r.URL.Path, "/api/v4/cloudinit_files/"):
			f.handleCloudInitFile(t, w, r, body)
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.RequestURI(), body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})
}

func (f *fakeCloudInit) handleCloudInitFile(t *testing.T, w http.ResponseWriter, r *http.Request, body []byte) {
	t.Helper()
	idText := strings.TrimPrefix(r.URL.Path, "/api/v4/cloudinit_files/")
	id, err := strconv.Atoi(idText)
	if err != nil || id <= 0 {
		t.Errorf("file id %q: %v", idText, err)
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	index := -1
	for i := range f.files {
		if f.files[i].key == id {
			index = i
			break
		}
	}
	switch r.Method {
	case http.MethodGet:
		if index < 0 {
			http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
			return
		}
		file := f.files[index]
		encoded, err := json.Marshal(map[string]any{"$key": file.key, "name": file.name, "owner": "vms/7"})
		if err != nil {
			t.Errorf("marshal file: %v", err)
			return
		}
		_, _ = w.Write(encoded)
	case http.MethodPut:
		if f.failPut {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"err":"rejected"}`))
			return
		}
		if index < 0 {
			http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
			return
		}
		var req struct {
			Contents *string `json:"contents"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("update body: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Contents != nil {
			f.files[index].contents = *req.Contents
		}
		_, _ = w.Write([]byte(`{}`))
	case http.MethodDelete:
		if index < 0 {
			http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
			return
		}
		f.files = append(f.files[:index], f.files[index+1:]...)
		_, _ = w.Write([]byte(`{}`))
	default:
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}
}

func runCloudInitUpdate(t *testing.T, fake *fakeCloudInit, stateFiles, planFiles []CloudInitFile) VMResourceModel {
	t.Helper()
	resp := runCloudInitUpdateRaw(t, fake, stateFiles, planFiles)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update: %v", resp.Diagnostics)
	}
	var got VMResourceModel
	if diags := resp.State.Get(t.Context(), &got); diags.HasError() {
		t.Fatalf("updated state: %v", diags)
	}
	return got
}

func runCloudInitUpdateExpectErr(t *testing.T, fake *fakeCloudInit, stateFiles, planFiles []CloudInitFile) *fwresource.UpdateResponse {
	t.Helper()
	resp := runCloudInitUpdateRaw(t, fake, stateFiles, planFiles)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected update to fail")
	}
	return resp
}

func runCloudInitUpdateRaw(t *testing.T, fake *fakeCloudInit, stateFiles, planFiles []CloudInitFile) *fwresource.UpdateResponse {
	t.Helper()
	shortenPowerWaits(t)

	server := httptest.NewServer(fake.handler(t))
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
		Id:             types.StringValue("7"),
		Machine:        types.Int32Value(1),
		Name:           types.StringValue("web"),
		PowerState:     types.BoolValue(false),
		CloudInitFiles: stateFiles,
		GuestAgentIPs:  types.ListNull(types.StringType),
	}
	planModel := stateModel
	planModel.CloudInitFiles = planFiles

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
	return resp
}
