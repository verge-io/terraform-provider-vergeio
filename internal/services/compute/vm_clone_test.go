// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestVMCloneSchema(t *testing.T) {
	item := NewVMCloneResource()
	meta := &resource.MetadataResponse{}
	item.Metadata(t.Context(), resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_vm_clone" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &resource.SchemaResponse{}
	item.Schema(t.Context(), resource.SchemaRequest{}, resp)
	for _, name := range []string{"snapshot_id", "name", "network_id"} {
		attr := resp.Schema.Attributes[name]
		if attr == nil || !attr.IsRequired() {
			t.Fatalf("%s should be required", name)
		}
	}
	source, ok := resp.Schema.Attributes["source_vm_id"].(schema.StringAttribute)
	if !ok || !source.IsComputed() || len(source.PlanModifiers) != 1 {
		t.Fatal("source_vm_id should be computed and keep its state value")
	}
	if !strings.Contains(source.PlanModifiers[0].Description(t.Context()), "will not change") {
		t.Fatalf("source_vm_id plan modifier = %s", source.PlanModifiers[0].Description(t.Context()))
	}
	if !strings.Contains(resp.Schema.MarkdownDescription, "snap_machine") || !strings.Contains(resp.Schema.MarkdownDescription, "preserve_macs") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestVMSnapshotsDataSourceReadsWithoutWriting(t *testing.T) {
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
			http.Error(w, "write", http.StatusMethodNotAllowed)
			return
		}
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms/15":
			_, _ = w.Write([]byte(`{"$key":15,"machine":40,"is_snapshot":false}`))
		case "/api/v4/machine_snapshots":
			if !strings.Contains(r.URL.Query().Get("filter"), "machine eq 40") {
				t.Errorf("filter = %s", r.URL.Query().Get("filter"))
			}
			_, _ = w.Write([]byte(`[
				{"$key":2,"machine":40,"snap_machine":90,"name":"older","created":10,"expires":20},
				{"$key":9,"machine":40,"snap_machine":91,"name":"newer","created":30,"expires":0}
			]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testVMAPI(t, server.URL)
	data := &VMSnapshotsDataSourceModel{VMID: types.StringValue("15")}
	if err := api.readVMSnapshots(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatalf("writes = %d", writes)
	}
	if len(data.Snapshots) != 2 || data.Snapshots[0].Id.ValueString() != "2" || data.Snapshots[1].Name.ValueString() != "newer" {
		t.Fatalf("snapshots = %#v", data.Snapshots)
	}
	if !data.Snapshots[1].Expires.IsNull() || data.Snapshots[1].SnapMachine.ValueInt64() != 91 {
		t.Fatalf("newer = %#v", data.Snapshots[1])
	}

	ds := NewVMSnapshotsDataSource()
	meta := &datasource.MetadataResponse{}
	ds.Metadata(t.Context(), datasource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_vm_snapshots" {
		t.Fatalf("type = %s", meta.TypeName)
	}
}

func TestCreateVMCloneUsesSnapshotVMAndMovesNICs(t *testing.T) {
	poll, timeout := vmClonePoll, vmCloneTimeout
	vmClonePoll = time.Millisecond
	vmCloneTimeout = 2 * time.Second
	t.Cleanup(func() {
		vmClonePoll = poll
		vmCloneTimeout = timeout
	})

	var actions []map[string]any
	var nicPuts []map[string]any
	var deleted []string
	nics := []map[string]any{{"$key": 5, "machine": 80, "name": "lan", "vnet": 9}}
	cloned := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.URL.Path == "/api/v4/machine_snapshots/4" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":4,"machine":40,"snap_machine":90,"name":"nightly"}`))
		case r.URL.Path == "/api/v4/vms" && r.Method == http.MethodGet:
			filter := r.URL.Query().Get("filter")
			switch {
			case strings.Contains(filter, "machine eq 90") && strings.Contains(filter, "is_snapshot eq true"):
				_, _ = w.Write([]byte(`[{"$key":70,"machine":90,"name":"src-snap","is_snapshot":true}]`))
			case strings.Contains(filter, "machine eq 40") && strings.Contains(filter, "is_snapshot eq false"):
				_, _ = w.Write([]byte(`[{"$key":15,"machine":40,"name":"src","is_snapshot":false}]`))
			case strings.Contains(filter, "name eq 'copy'"):
				if !cloned {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(`[{"$key":81,"machine":80,"name":"copy","is_snapshot":false,"powerstate":false}]`))
			default:
				t.Errorf("vm filter %s", filter)
				http.NotFound(w, r)
			}
		case r.URL.Path == "/api/v4/vms/81" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":81,"machine":80,"name":"copy","is_snapshot":false,"powerstate":false}`))
		case r.URL.Path == "/api/v4/vms/15" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":15,"machine":40,"name":"src","is_snapshot":false}`))
		case r.URL.Path == "/api/v4/vm_actions" && r.Method == http.MethodPost:
			body := readJSON(t, r)
			actions = append(actions, body)
			if body["action"] == "clone" {
				cloned = true
			}
			_, _ = w.Write([]byte(`{"$key":1}`))
		case r.URL.Path == "/api/v4/machine_nics" && r.Method == http.MethodGet:
			encoded, err := json.Marshal(nics)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(encoded)
		case r.URL.Path == "/api/v4/machine_nics/5" && r.Method == http.MethodPut:
			body := readJSON(t, r)
			nicPuts = append(nicPuts, body)
			nics[0]["vnet"] = body["vnet"]
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/api/v4/machine_nics/5" && r.Method == http.MethodGet:
			encoded, err := json.Marshal(nics[0])
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(encoded)
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testVMAPI(t, server.URL)
	data := &VMCloneResourceModel{
		SnapshotID: types.StringValue("4"),
		Name:       types.StringValue("copy"),
		NetworkID:  types.StringValue("6"),
	}
	if err := api.createVMClone(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() != "81" || data.SourceVMID.ValueString() != "15" {
		t.Fatalf("clone = %#v", data)
	}
	if len(actions) != 1 || actions[0]["action"] != "clone" || actions[0]["vm"] != float64(70) {
		t.Fatalf("actions = %#v", actions)
	}
	params, _ := actions[0]["params"].(map[string]any)
	if params["name"] != "copy" || params["preserve_macs"] != false {
		t.Fatalf("params = %#v", params)
	}
	if _, ok := params["preserve_bios_serial"]; ok {
		t.Fatalf("params = %#v", params)
	}
	if len(nicPuts) != 1 || nicPuts[0]["vnet"] != float64(6) {
		t.Fatalf("nic puts = %#v", nicPuts)
	}
	if len(deleted) != 0 {
		t.Fatalf("deleted during create = %#v", deleted)
	}

	if err := api.deleteVMClone(t.Context(), 81, 15); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "/api/v4/vms/81" {
		t.Fatalf("deleted = %#v", deleted)
	}
	if err := api.deleteVMClone(t.Context(), 15, 15); err == nil {
		t.Fatal("deleted the source VM")
	}
}

func TestCreateVMCloneRollsBackWhenNICStaysOnSourceNetwork(t *testing.T) {
	poll, timeout := vmClonePoll, vmCloneTimeout
	vmClonePoll = time.Millisecond
	vmCloneTimeout = 2 * time.Second
	t.Cleanup(func() {
		vmClonePoll = poll
		vmCloneTimeout = timeout
	})

	var deleted []string
	cloned := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.URL.Path == "/api/v4/machine_snapshots/4":
			_, _ = w.Write([]byte(`{"$key":4,"machine":40,"snap_machine":90,"name":"nightly"}`))
		case r.URL.Path == "/api/v4/vms" && r.Method == http.MethodGet:
			filter := r.URL.Query().Get("filter")
			switch {
			case strings.Contains(filter, "machine eq 90"):
				_, _ = w.Write([]byte(`[{"$key":70,"machine":90,"name":"src-snap","is_snapshot":true}]`))
			case strings.Contains(filter, "machine eq 40"):
				_, _ = w.Write([]byte(`[{"$key":15,"machine":40,"name":"src","is_snapshot":false}]`))
			case strings.Contains(filter, "name eq 'copy'"):
				if !cloned {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(`[{"$key":81,"machine":80,"name":"copy","is_snapshot":false,"powerstate":false}]`))
			default:
				http.NotFound(w, r)
			}
		case r.URL.Path == "/api/v4/vms/81" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":81,"machine":80,"name":"copy","is_snapshot":false,"powerstate":false}`))
		case r.URL.Path == "/api/v4/vm_actions":
			cloned = true
			_, _ = w.Write([]byte(`{"$key":1}`))
		case r.URL.Path == "/api/v4/machine_nics" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[{"$key":5,"machine":80,"name":"lan","vnet":9}]`))
		case r.URL.Path == "/api/v4/machine_nics/5" && r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/api/v4/machine_nics/5" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":5,"machine":80,"name":"lan","vnet":9}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vms/81":
			deleted = append(deleted, r.URL.Path)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testVMAPI(t, server.URL)
	err := api.createVMClone(t.Context(), &VMCloneResourceModel{
		SnapshotID: types.StringValue("4"),
		Name:       types.StringValue("copy"),
		NetworkID:  types.StringValue("6"),
	})
	if err == nil {
		t.Fatal("create succeeded while the NIC stayed on the source network")
	}
	if len(deleted) != 1 {
		t.Fatalf("deleted = %#v err = %v", deleted, err)
	}
}

func TestCreateVMCloneDeletesCopyWhenNICMoveFailsPartway(t *testing.T) {
	poll, timeout := vmClonePoll, vmCloneTimeout
	vmClonePoll = time.Millisecond
	vmCloneTimeout = 2 * time.Second
	t.Cleanup(func() {
		vmClonePoll = poll
		vmCloneTimeout = timeout
	})

	var deleted []string
	cloned := false
	vnets := map[int]int{5: 9, 6: 9}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.URL.Path == "/api/v4/machine_snapshots/4":
			_, _ = w.Write([]byte(`{"$key":4,"machine":40,"snap_machine":90,"name":"nightly"}`))
		case r.URL.Path == "/api/v4/vms" && r.Method == http.MethodGet:
			filter := r.URL.Query().Get("filter")
			switch {
			case strings.Contains(filter, "machine eq 90"):
				_, _ = w.Write([]byte(`[{"$key":70,"machine":90,"name":"src-snap","is_snapshot":true}]`))
			case strings.Contains(filter, "machine eq 40"):
				_, _ = w.Write([]byte(`[{"$key":15,"machine":40,"name":"src","is_snapshot":false}]`))
			case strings.Contains(filter, "name eq 'copy'"):
				if !cloned {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(`[{"$key":81,"machine":80,"name":"copy","is_snapshot":false,"powerstate":false}]`))
			default:
				http.NotFound(w, r)
			}
		case r.URL.Path == "/api/v4/vms/81" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":81,"machine":80,"name":"copy","is_snapshot":false,"powerstate":false}`))
		case r.URL.Path == "/api/v4/vms/15":
			t.Error("clone cleanup touched the source VM")
			http.Error(w, "source", http.StatusForbidden)
		case r.URL.Path == "/api/v4/vm_actions":
			cloned = true
			_, _ = w.Write([]byte(`{"$key":1}`))
		case r.URL.Path == "/api/v4/machine_nics" && r.Method == http.MethodGet:
			_, _ = fmt.Fprintf(w, `[{"$key":5,"machine":80,"name":"lan","vnet":%d},{"$key":6,"machine":80,"name":"wan","vnet":%d}]`, vnets[5], vnets[6])
		case r.URL.Path == "/api/v4/machine_nics/5" && r.Method == http.MethodPut:
			body := readJSON(t, r)
			vnets[5] = int(body["vnet"].(float64))
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/api/v4/machine_nics/5" && r.Method == http.MethodGet:
			_, _ = fmt.Fprintf(w, `{"$key":5,"machine":80,"name":"lan","vnet":%d}`, vnets[5])
		case r.URL.Path == "/api/v4/machine_nics/6" && r.Method == http.MethodPut:
			http.Error(w, "nic update refused", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vms/81":
			deleted = append(deleted, r.URL.Path)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testVMAPI(t, server.URL)
	err := api.createVMClone(t.Context(), &VMCloneResourceModel{
		SnapshotID: types.StringValue("4"),
		Name:       types.StringValue("copy"),
		NetworkID:  types.StringValue("6"),
	})
	if err == nil || !strings.Contains(err.Error(), "NIC 6") {
		t.Fatalf("err = %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "/api/v4/vms/81" {
		t.Fatalf("deleted = %#v", deleted)
	}
	if vnets[5] != 6 || vnets[6] != 9 {
		t.Fatalf("vnets after partial move = %#v", vnets)
	}
}

func TestReadAndDeleteVMCloneWithoutSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.URL.Path == "/api/v4/vms/81" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":81,"machine":80,"name":"copy","is_snapshot":false,"powerstate":false}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vms/81":
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testVMAPI(t, server.URL)
	data := &VMCloneResourceModel{Id: types.StringValue("81")}
	if err := api.readVMClone(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Name.ValueString() != "copy" || !data.SourceVMID.IsNull() {
		t.Fatalf("read = %#v", data)
	}
	if err := api.deleteVMClone(t.Context(), 81, 0); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteVMCloneKillsWhenGuestStaysUp(t *testing.T) {
	timeout, interval := gracefulShutdownTimeout, gracefulShutdownInterval
	gracefulShutdownTimeout = 0
	gracefulShutdownInterval = 0
	t.Cleanup(func() {
		gracefulShutdownTimeout = timeout
		gracefulShutdownInterval = interval
	})

	running := true
	var actions []string
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.URL.Path == "/api/v4/vms/81" && r.Method == http.MethodGet:
			_, _ = fmt.Fprintf(w, `{"$key":81,"name":"copy","is_snapshot":false,"powerstate":%t,"running":%t}`, running, running)
		case r.URL.Path == "/api/v4/vm_actions" && r.Method == http.MethodPost:
			body := readJSON(t, r)
			action, _ := body["action"].(string)
			actions = append(actions, action)
			if action == vmActionKill {
				running = false
			}
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vms/81":
			deleted = true
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testVMAPI(t, server.URL)
	if err := api.deleteVMClone(t.Context(), 81, 0); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("clone was not deleted")
	}
	if len(actions) != 2 || actions[0] != vmActionPowerOff || actions[1] != vmActionKill {
		t.Fatalf("actions = %#v, want poweroff then kill", actions)
	}
}

func testVMAPI(t *testing.T, host string) *VMApi {
	t.Helper()
	api, err := NewVMApi(vergeio.NewClient(host, "user", "pass", true))
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func readJSON(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	return decoded
}
