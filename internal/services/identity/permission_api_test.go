// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestPermissionUpdateRequestSendsOnlyChangedRights(t *testing.T) {
	plan := &PermissionResourceModel{
		List:   types.BoolValue(true),
		Read:   types.BoolValue(true),
		Create: types.BoolValue(false),
		Modify: types.BoolValue(true),
		Delete: types.BoolValue(false),
	}
	state := &PermissionResourceModel{
		List:   types.BoolValue(true),
		Read:   types.BoolValue(true),
		Create: types.BoolValue(false),
		Modify: types.BoolValue(false),
		Delete: types.BoolValue(false),
	}
	req := permissionUpdateRequest(plan, state)
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"modify":true}`
	if string(raw) != want {
		t.Fatalf("update body = %s, want %s", raw, want)
	}
}

func TestMatchingIdentityKeys(t *testing.T) {
	rows, err := decodeIdentityRows([]byte(`[{"$key":7,"identity":15},{"$key":8,"identity":16}]`))
	if err != nil {
		t.Fatal(err)
	}
	got := matchingIdentityKeys(rows, 15)
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("keys = %v, want [7]", got)
	}
	one, err := decodeIdentityRows([]byte(`{"$key":"4","identity":9}`))
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := flexInt(one[0].Identity); !ok || id != 9 {
		t.Fatalf("identity = %#v", one[0].Identity)
	}
}

func TestCreateObjectPermissionUsesIdentityAndRow(t *testing.T) {
	var posted map[string]any
	server := permissionServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/permissions" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			if err := json.Unmarshal(body, &posted); err != nil {
				t.Errorf("decode body: %v", err)
			}
			_, _ = w.Write([]byte(`{"$key":3}`))
			return true
		}
		return false
	})
	api := NewPermissionApi(vergeio.NewClient(server.URL, "user", "pass", true))
	data := &PermissionResourceModel{
		UserID:   types.StringValue("7"),
		GroupID:  types.StringNull(),
		Table:    types.StringValue("vms"),
		ObjectID: types.Int64Value(100),
		List:     types.BoolValue(true),
		Read:     types.BoolValue(true),
		Create:   types.BoolValue(false),
		Modify:   types.BoolValue(false),
		Delete:   types.BoolValue(false),
	}
	if err := api.createPermission(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() != "3" || data.UserID.ValueString() != "7" {
		t.Fatalf("permission = %#v", data)
	}
	if posted["identity"] != float64(15) {
		t.Fatalf("identity = %#v, want 15", posted["identity"])
	}
	if posted["row"] != float64(100) || posted["table"] != "vms" {
		t.Fatalf("posted = %#v", posted)
	}
	if posted["read"] != true || posted["list"] != true || posted["delete"] != false {
		t.Fatalf("rights = %#v", posted)
	}
}

func TestCreateTablePermissionPostsRowZero(t *testing.T) {
	var posted map[string]any
	server := permissionServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/permissions" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &posted)
			_, _ = w.Write([]byte(`{"$key":5}`))
			return true
		}
		return false
	})
	api := NewPermissionApi(vergeio.NewClient(server.URL, "user", "pass", true))
	data := &PermissionResourceModel{
		UserID:   types.StringNull(),
		GroupID:  types.StringValue("4"),
		Table:    types.StringValue("vms"),
		ObjectID: types.Int64Null(),
		List:     types.BoolValue(true),
		Read:     types.BoolValue(true),
		Create:   types.BoolNull(),
		Modify:   types.BoolNull(),
		Delete:   types.BoolNull(),
	}
	if err := api.createPermission(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if !data.ObjectID.IsNull() {
		t.Fatalf("object_id = %#v, want null for a table grant", data.ObjectID)
	}
	if posted["row"] != float64(0) {
		t.Fatalf("row = %#v, want 0", posted["row"])
	}
	if posted["identity"] != float64(9) {
		t.Fatalf("identity = %#v, want the group identity 9", posted["identity"])
	}
	if _, ok := posted["create"]; ok {
		t.Fatal("unset create was sent")
	}
}

func TestReadPermissionKeepsConfiguredUser(t *testing.T) {
	server := permissionServer(t, nil)
	api := NewPermissionApi(vergeio.NewClient(server.URL, "user", "pass", true))
	data := &PermissionResourceModel{
		Id:       types.StringValue("3"),
		UserID:   types.StringValue("7"),
		GroupID:  types.StringNull(),
		Table:    types.StringValue("vms"),
		ObjectID: types.Int64Value(100),
	}
	if err := api.readPermission(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.UserID.ValueString() != "7" || !data.GroupID.IsNull() {
		t.Fatalf("grantee user=%s group null=%v", data.UserID.ValueString(), data.GroupID.IsNull())
	}
	if !data.Read.ValueBool() || !data.List.ValueBool() || data.Modify.ValueBool() {
		t.Fatalf("rights list=%v read=%v modify=%v", data.List.ValueBool(), data.Read.ValueBool(), data.Modify.ValueBool())
	}
}

func TestImportPermissionResolvesUserByIdentity(t *testing.T) {
	server := permissionServer(t, nil)
	api := NewPermissionApi(vergeio.NewClient(server.URL, "user", "pass", true))
	data := &PermissionResourceModel{Id: types.StringValue("3")}
	if err := api.readPermission(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.UserID.ValueString() != "7" || !data.GroupID.IsNull() {
		t.Fatalf("imported grantee user=%s group=%s", data.UserID.ValueString(), data.GroupID.ValueString())
	}
	if data.Table.ValueString() != "vms" || data.ObjectID.ValueInt64() != 100 {
		t.Fatalf("target table=%s object=%s", data.Table.ValueString(), data.ObjectID.String())
	}
}

func permissionServer(t *testing.T, extra func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if extra != nil && extra(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/7":
			_, _ = w.Write([]byte(`{"$key":7,"identity":15}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/4":
			_, _ = w.Write([]byte(`{"$key":4,"identity":9}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/users" && r.URL.Query().Get("filter") == "identity eq 15":
			_, _ = w.Write([]byte(`[{"$key":7,"identity":15,"name":"ada"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups" && r.URL.Query().Get("filter") == "identity eq 15":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/permissions/3":
			_, _ = w.Write([]byte(`{"$key":3,"identity":15,"table":"vms","row":100,"list":true,"read":true,"create":false,"modify":false,"delete":false}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/permissions/5":
			_, _ = w.Write([]byte(`{"$key":5,"identity":9,"table":"vms","row":0,"list":true,"read":true,"create":false,"modify":false,"delete":false}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
