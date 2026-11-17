// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

func TestNetworkConstructorsReturnClientError(t *testing.T) {
	bad := versionClient(t, "25.0.0")
	good := versionClient(t, "26.0.0")

	api, err := NewNetworkApi(nil)
	assertNoAPI(t, api, err)
	api, err = NewNetworkApi(bad)
	assertNoAPI(t, api, err)
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}

	rules, err := NewRuleApi(nil)
	assertNoAPI(t, rules, err)
	rules, err = NewRuleApi(bad)
	assertNoAPI(t, rules, err)
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}

	api, err = NewNetworkApi(good)
	if err != nil || api == nil || api.sdk == nil {
		t.Fatalf("network api=%v err=%v", api, err)
	}
	rules, err = NewRuleApi(good)
	if err != nil || rules == nil || rules.sdk == nil {
		t.Fatalf("rules api=%v err=%v", rules, err)
	}
}

func TestNetworkDataSourceConfigureReportsClientError(t *testing.T) {
	d := &NetworkDataSource{}
	resp := &datasource.ConfigureResponse{}
	d.Configure(t.Context(), datasource.ConfigureRequest{ProviderData: versionClient(t, "25.0.0")}, resp)
	requireClientDiagnostic(t, resp.Diagnostics)
	if d.networkApi != nil {
		t.Fatal("network API was stored after the client could not be created")
	}
}

func TestNetworkResourceConfigureReportsClientError(t *testing.T) {
	r := &NetworkResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(t.Context(), resource.ConfigureRequest{ProviderData: versionClient(t, "25.0.0")}, resp)
	requireClientDiagnostic(t, resp.Diagnostics)
	if r.networkApi != nil {
		t.Fatal("network API was stored after the client could not be created")
	}
}

func TestNewNetworkApiRejectsUntrustedCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	api, err := NewNetworkApi(vergeio.NewClientWithConfig(vergeio.ClientConfig{
		Host:     server.URL,
		Username: "user",
		Password: "pass",
		Insecure: false,
	}))
	assertNoAPI(t, api, err)
	if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "failed to check server version") {
		t.Fatalf("error = %v", err)
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

func requireClientDiagnostic(t *testing.T, diags diag.Diagnostics) {
	t.Helper()
	if !diags.HasError() {
		t.Fatal("expected a client diagnostic")
	}
	d := diags.Errors()[0]
	if d.Summary() != "Unable to Create VergeOS API Client" {
		t.Fatalf("summary = %q", d.Summary())
	}
	if !strings.Contains(d.Detail(), "failed to create VergeOS client") || !strings.Contains(d.Detail(), "unsupported server version") {
		t.Fatalf("detail = %q", d.Detail())
	}
}
