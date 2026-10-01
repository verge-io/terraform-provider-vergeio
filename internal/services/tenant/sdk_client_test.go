// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestTenantConstructorReturnsClientError(t *testing.T) {
	api, err := NewAPI(nil)
	if err == nil || api != nil {
		t.Fatalf("nil client: api=%v err=%v", api, err)
	}

	api, err = NewAPI(versionClient(t, "27.0.0"))
	if err == nil || api != nil {
		t.Fatalf("unsupported version: api=%v err=%v", api, err)
	}
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "failed to create VergeOS client") {
		t.Fatalf("error = %v", err)
	}

	api, err = NewAPI(versionClient(t, "26.0.0"))
	if err != nil || api == nil || api.sdk == nil {
		t.Fatalf("api=%v err=%v", api, err)
	}
}

func tenantTestClient(t *testing.T) *vergeio.Client {
	t.Helper()
	return versionClient(t, "26.0.0")
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
