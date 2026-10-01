// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

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

func TestTagsConstructorReturnsClientError(t *testing.T) {
	api, err := NewTagsApi(nil)
	assertNoAPI(t, api, err)

	api, err = NewTagsApi(versionClient(t, "25.0.0"))
	assertNoAPI(t, api, err)
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}

	api, err = NewTagsApi(versionClient(t, "26.0.0"))
	if err != nil || api == nil || api.sdk == nil {
		t.Fatalf("api=%v err=%v", api, err)
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
