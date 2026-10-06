// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func mustAPI[T any](api T, err error) T {
	if err != nil {
		panic(err)
	}
	return api
}

func TestIdentityConstructorsReturnClientError(t *testing.T) {
	bad := versionClient(t, "25.0.0")
	good := versionClient(t, "26.0.0")

	group, err := NewGroupApi(nil)
	assertNoAPI(t, group, err)
	group, err = NewGroupApi(bad)
	assertNoAPI(t, group, err)
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}
	group, err = NewGroupApi(good)
	if err != nil || group == nil || group.sdk == nil {
		t.Fatalf("group api=%v err=%v", group, err)
	}

	groups, err := NewGroupsApi(bad)
	assertNoAPI(t, groups, err)
	groups, err = NewGroupsApi(good)
	if err != nil || groups == nil || groups.sdk == nil {
		t.Fatalf("groups api=%v err=%v", groups, err)
	}

	member, err := NewMemberApi(bad)
	assertNoAPI(t, member, err)
	member, err = NewMemberApi(good)
	if err != nil || member == nil || member.sdk == nil {
		t.Fatalf("member api=%v err=%v", member, err)
	}

	permission, err := NewPermissionApi(bad)
	assertNoAPI(t, permission, err)
	permission, err = NewPermissionApi(good)
	if err != nil || permission == nil || permission.sdk == nil {
		t.Fatalf("permission api=%v err=%v", permission, err)
	}

	user, err := NewUserApi(bad)
	assertNoAPI(t, user, err)
	user, err = NewUserApi(good)
	if err != nil || user == nil || user.sdk == nil {
		t.Fatalf("user api=%v err=%v", user, err)
	}

	users, err := NewUsersApi(bad)
	assertNoAPI(t, users, err)
	users, err = NewUsersApi(good)
	if err != nil || users == nil || users.sdk == nil {
		t.Fatalf("users api=%v err=%v", users, err)
	}

	apiKey, err := NewAPIKeyApi(bad)
	assertNoAPI(t, apiKey, err)
	apiKey, err = NewAPIKeyApi(good)
	if err != nil || apiKey == nil || apiKey.sdk == nil {
		t.Fatalf("api key api=%v err=%v", apiKey, err)
	}

	authSource, err := NewAuthSourceApi(bad)
	assertNoAPI(t, authSource, err)
	authSource, err = NewAuthSourceApi(good)
	if err != nil || authSource == nil || authSource.sdk == nil {
		t.Fatalf("auth source api=%v err=%v", authSource, err)
	}
}

func TestGroupResourceConfigureReportsClientError(t *testing.T) {
	r := &GroupResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(t.Context(), resource.ConfigureRequest{ProviderData: versionClient(t, "25.0.0")}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a client diagnostic")
	}
	d := resp.Diagnostics.Errors()[0]
	if d.Summary() != "Unable to Create VergeOS API Client" {
		t.Fatalf("summary = %q", d.Summary())
	}
	if !strings.Contains(d.Detail(), "unsupported server version") {
		t.Fatalf("detail = %q", d.Detail())
	}
	if r.api != nil {
		t.Fatal("group API was stored after the client could not be created")
	}
}

func versionClient(t *testing.T, version string) *vergeio.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"` + version + `"}`))
	}))
	t.Cleanup(server.Close)
	return vergeio.NewClient(server.URL, "user", "pass", true)
}

func assertNoAPI[T any](t *testing.T, api *T, err error) {
	t.Helper()
	if err == nil || api != nil {
		t.Fatalf("api=%v err=%v, want an error and no API", api, err)
	}
	if !strings.Contains(err.Error(), "failed to create VergeOS client") && !strings.Contains(err.Error(), "vergeio client is nil") {
		t.Fatalf("error = %v", err)
	}
}
