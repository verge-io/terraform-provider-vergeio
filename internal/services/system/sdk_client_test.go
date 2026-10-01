// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package system

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func mustAPI[T any](api T, err error) T {
	if err != nil {
		panic(err)
	}
	return api
}

func TestSystemConstructorsReturnClientError(t *testing.T) {
	bad := versionClient(t, "27.0.0")
	good := versionClient(t, "26.0.0")

	cluster, err := NewClusterApi(bad)
	assertNoAPI(t, cluster, err)
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}
	cluster, err = NewClusterApi(good)
	if err != nil || cluster == nil || cluster.sdk == nil {
		t.Fatalf("cluster api=%v err=%v", cluster, err)
	}

	node, err := NewNodeApi(bad)
	assertNoAPI(t, node, err)
	node, err = NewNodeApi(good)
	if err != nil || node == nil || node.sdk == nil {
		t.Fatalf("node api=%v err=%v", node, err)
	}

	groups, err := NewResourceGroupsApi(bad)
	assertNoAPI(t, groups, err)
	groups, err = NewResourceGroupsApi(good)
	if err != nil || groups == nil || groups.sdk == nil {
		t.Fatalf("resource groups api=%v err=%v", groups, err)
	}

	version, err := NewVersionApi(bad)
	assertNoAPI(t, version, err)
	version, err = NewVersionApi(good)
	if err != nil || version == nil || version.sdk == nil {
		t.Fatalf("version api=%v err=%v", version, err)
	}
}

func versionClient(t *testing.T, version string) *vergeio.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if !strings.Contains(err.Error(), "failed to create VergeOS client") {
		t.Fatalf("error = %v", err)
	}
}
