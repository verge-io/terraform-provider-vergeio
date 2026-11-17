// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestSnapshotProfileConstructorReturnsClientError(t *testing.T) {
	api, err := NewSnapshotProfileApi(nil)
	if err == nil || api != nil {
		t.Fatalf("nil client: api=%v err=%v", api, err)
	}

	api, err = NewSnapshotProfileApi(versionClient(t, "25.0.0"))
	if err == nil || api != nil {
		t.Fatalf("unsupported version: api=%v err=%v", api, err)
	}
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "failed to create VergeOS client") {
		t.Fatalf("error = %v", err)
	}

	api, err = NewSnapshotProfileApi(versionClient(t, "26.0.0"))
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
