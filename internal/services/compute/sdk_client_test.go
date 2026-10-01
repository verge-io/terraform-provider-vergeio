// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestComputeConstructorsReturnClientError(t *testing.T) {
	bad := versionClient(t, "25.0.0")
	good := versionClient(t, "26.0.0")

	vm, err := NewVMApi(nil)
	assertNoAPI(t, vm, err)
	vm, err = NewVMApi(bad)
	assertNoAPI(t, vm, err)
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}
	vm, err = NewVMApi(good)
	if err != nil || vm == nil || vm.sdk == nil {
		t.Fatalf("vm api=%v err=%v", vm, err)
	}

	disk, err := NewDiskApi(bad)
	assertNoAPI(t, disk, err)
	disk, err = NewDiskApi(good)
	if err != nil || disk == nil || disk.sdk == nil {
		t.Fatalf("disk api=%v err=%v", disk, err)
	}

	nic, err := NewNICApi(bad)
	assertNoAPI(t, nic, err)
	nic, err = NewNICApi(good)
	if err != nil || nic == nil || nic.sdk == nil {
		t.Fatalf("nic api=%v err=%v", nic, err)
	}

	device, err := NewDeviceApi(bad)
	assertNoAPI(t, device, err)
	device, err = NewDeviceApi(good)
	if err != nil || device == nil || device.sdk == nil {
		t.Fatalf("device api=%v err=%v", device, err)
	}

	files, err := NewCloudinitFileApi(bad)
	assertNoAPI(t, files, err)
	files, err = NewCloudinitFileApi(good)
	if err != nil || files == nil || files.sdk == nil {
		t.Fatalf("cloudinit api=%v err=%v", files, err)
	}
}

func TestVMResourceConfigureReportsClientError(t *testing.T) {
	r := &VMResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(t.Context(), resource.ConfigureRequest{ProviderData: versionClient(t, "25.0.0")}, resp)
	requireClientDiagnostic(t, resp.Diagnostics)
	if r.vmApi != nil || r.diskApi != nil || r.nicApi != nil || r.deviceApi != nil {
		t.Fatal("VM APIs were stored after the client could not be created")
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
	if !strings.Contains(d.Detail(), "unsupported server version") {
		t.Fatalf("detail = %q", d.Detail())
	}
}
