// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"net/http"
	"net/http/httptest"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestReadVMKeepsNullCloudInitFilesWhenVMHasNone(t *testing.T) {
	fake := newFakeCloudInit()
	got := runCloudInitRead(t, fake, nil)
	if got.CloudInitFiles != nil {
		t.Fatalf("files = %#v, want null", got.CloudInitFiles)
	}
	if len(fake.calls(http.MethodGet, "/api/v4/cloudinit_files/11")) != 0 {
		t.Fatal("downloaded a file the VM does not have")
	}
}

func TestReadVMStoresCloudInitFileEditedOutsideTerraform(t *testing.T) {
	const changed = "#cloud-config\nhostname: CHANGED\n"
	fake := newFakeCloudInit(fakeCloudInitRow{key: 11, name: "/user-data", contents: changed})
	got := runCloudInitRead(t, fake, []CloudInitFile{cloudInitFile("/user-data", cloudInitFirst)})
	assertCloudInitFiles(t, got, []CloudInitFile{cloudInitFile("/user-data", changed)})

	var downloads int
	for _, call := range fake.calls(http.MethodGet, "/api/v4/cloudinit_files/11") {
		if call.download == "1" {
			downloads++
		}
	}
	if downloads != 1 {
		t.Fatalf("download calls = %d, want GET cloudinit_files/11?download=1", downloads)
	}
}

func TestReadVMDropsCloudInitFileDeletedOutsideTerraform(t *testing.T) {
	fake := newFakeCloudInit()
	got := runCloudInitRead(t, fake, []CloudInitFile{
		cloudInitFile("/user-data", cloudInitFirst),
		cloudInitFile("/meta-data", cloudInitMeta),
	})
	if len(got.CloudInitFiles) != 0 {
		t.Fatalf("files = %#v, want none after the rows were deleted", got.CloudInitFiles)
	}
	for _, call := range fake.cloudInitCalls() {
		if call.download == "1" {
			t.Fatalf("downloaded a deleted file: %#v", fake.cloudInitCalls())
		}
	}
}

func TestReadVMKeepsCloudInitOrderWhenVergeOSReturnsAnotherOrder(t *testing.T) {
	const changed = "#cloud-config\nhostname: CHANGED\n"
	fake := newFakeCloudInit(
		fakeCloudInitRow{key: 12, name: "/meta-data", contents: cloudInitMeta},
		fakeCloudInitRow{key: 11, name: "/user-data", contents: changed},
	)
	got := runCloudInitRead(t, fake, []CloudInitFile{
		cloudInitFile("/user-data", cloudInitFirst),
		cloudInitFile("/meta-data", cloudInitMeta),
	})
	assertCloudInitFiles(t, got, []CloudInitFile{
		cloudInitFile("/user-data", changed),
		cloudInitFile("/meta-data", cloudInitMeta),
	})
}

func runCloudInitRead(t *testing.T, fake *fakeCloudInit, stateFiles []CloudInitFile) VMResourceModel {
	t.Helper()

	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	ctx := t.Context()
	vergeClient := vergeio.NewClient(server.URL, "user", "pass", true)
	vmResource := &VMResource{vmApi: mustAPI(NewVMApi(vergeClient))}

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
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("state: %v", diags)
	}

	resp := &fwresource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	vmResource.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	var got VMResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("refreshed state: %v", diags)
	}
	return got
}
