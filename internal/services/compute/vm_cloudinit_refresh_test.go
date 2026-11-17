// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestReadVMKeepsCloudInitFilesRemovedFromVergeOS(t *testing.T) {
	// Power-on detach leaves cloudInitExpectLive unset. Refresh keeps the
	// prior list when VergeOS has none, or the next plan creates the rows
	// again.
	fake := newFakeCloudInit()
	prior := []CloudInitFile{
		cloudInitFile("/user-data", cloudInitFirst),
		cloudInitFile("/meta-data", cloudInitMeta),
	}
	got := runCloudInitRead(t, fake, prior)
	assertCloudInitFiles(t, got, prior)
	for _, call := range fake.cloudInitCalls() {
		if call.download == "1" {
			t.Fatalf("downloaded a missing file: %#v", fake.cloudInitCalls())
		}
	}
}

func TestReadVMDropsAllCloudInitFilesWhenExpectLive(t *testing.T) {
	// Create with powerstate=false sets expect-live. An external wipe of
	// every file must clear state so the next plan recreates them.
	fake := newFakeCloudInit()
	prior := []CloudInitFile{
		cloudInitFile("/user-data", cloudInitFirst),
		cloudInitFile("/meta-data", cloudInitMeta),
	}
	got := runCloudInitReadExpectLive(t, fake, prior, true)
	assertCloudInitFiles(t, got, []CloudInitFile{})
}

func TestReadVMDropsSingleCloudInitFileWhenExpectLiveAndOnlyFileDeleted(t *testing.T) {
	fake := newFakeCloudInit()
	got := runCloudInitReadExpectLive(t, fake, []CloudInitFile{
		cloudInitFile("/user-data", cloudInitFirst),
	}, true)
	assertCloudInitFiles(t, got, []CloudInitFile{})
}

func TestReadVMReportsCloudInitLivePresentWhenFilesAttached(t *testing.T) {
	// Read seeds expect-live when live files are present and the private
	// marker is absent (#192). readVM must report that presence.
	fake := newFakeCloudInit(fakeCloudInitRow{key: 11, name: "/user-data", contents: cloudInitFirst})
	_, live := runCloudInitReadExpectLivePresent(t, fake, []CloudInitFile{
		cloudInitFile("/user-data", cloudInitFirst),
	}, false)
	if !live {
		t.Fatal("livePresent = false, want true when VergeOS still has files")
	}
}

func TestReadVMReportsCloudInitLiveAbsentWhenFilesWiped(t *testing.T) {
	fake := newFakeCloudInit()
	prior := []CloudInitFile{cloudInitFile("/user-data", cloudInitFirst)}
	got, live := runCloudInitReadExpectLivePresent(t, fake, prior, false)
	if live {
		t.Fatal("livePresent = true, want false when VergeOS has no files")
	}
	// Power-on detach / pre-seed upgrade wipe: expectLive false keeps prior.
	assertCloudInitFiles(t, got, prior)
}

func TestShouldSeedCloudInitExpectLive(t *testing.T) {
	cases := []struct {
		expectLive, livePresent, want bool
	}{
		{false, true, true},   // upgraded/imported VM with files still attached
		{false, false, false}, // power-on detach (#185): do not arm wipe detection
		{true, true, false},   // already marked
		{true, false, false},  // already marked; wipe handled by expectLive
	}
	for _, tc := range cases {
		got := shouldSeedCloudInitExpectLive(tc.expectLive, tc.livePresent)
		if got != tc.want {
			t.Fatalf("shouldSeed(expectLive=%v, livePresent=%v) = %v, want %v",
				tc.expectLive, tc.livePresent, got, tc.want)
		}
	}
}

func TestCloudInitFilesForStateEmptyLive(t *testing.T) {
	prior := []CloudInitFile{
		cloudInitFile("/user-data", cloudInitFirst),
		cloudInitFile("/meta-data", cloudInitMeta),
	}
	kept := cloudInitFilesForState(prior, nil, false)
	assertCloudInitFilesModel(t, kept, prior)
	cleared := cloudInitFilesForState(prior, nil, true)
	assertCloudInitFilesModel(t, cleared, []CloudInitFile{})
	partialLive := []CloudInitFile{cloudInitFile("/meta-data", cloudInitMeta)}
	partial := cloudInitFilesForState(prior, partialLive, true)
	assertCloudInitFilesModel(t, partial, []CloudInitFile{cloudInitFile("/meta-data", cloudInitMeta)})
}

func TestReadVMDropsCloudInitFileDeletedWhileAnotherRemains(t *testing.T) {
	fake := newFakeCloudInit(fakeCloudInitRow{key: 12, name: "/meta-data", contents: cloudInitMeta})
	got := runCloudInitRead(t, fake, []CloudInitFile{
		cloudInitFile("/user-data", cloudInitFirst),
		cloudInitFile("/meta-data", cloudInitMeta),
	})
	assertCloudInitFiles(t, got, []CloudInitFile{cloudInitFile("/meta-data", cloudInitMeta)})
}

func TestReadVMKeepsConfiguredCloudInitNameWithoutLeadingSlash(t *testing.T) {
	const changed = "#cloud-config\nhostname: CHANGED\n"
	fake := newFakeCloudInit(fakeCloudInitRow{key: 11, name: "/user-data", contents: changed})
	got := runCloudInitRead(t, fake, []CloudInitFile{cloudInitFile("user-data", cloudInitFirst)})
	assertCloudInitFiles(t, got, []CloudInitFile{cloudInitFile("user-data", changed)})
}

func TestReadVMDoesNotAdoptDefaultCloudInitFiles(t *testing.T) {
	fake := newFakeCloudInit(
		fakeCloudInitRow{key: 11, name: "/user-data", contents: "#cloud-config\n"},
		fakeCloudInitRow{key: 12, name: "/meta-data", contents: "instance-id: ${YB_UUID}\n"},
	)
	got := runCloudInitRead(t, fake, nil)
	if got.CloudInitFiles != nil {
		t.Fatalf("files = %#v, want null when configuration sets none", got.CloudInitFiles)
	}
}

func TestReadVMDoesNotAppendUnconfiguredCloudInitFile(t *testing.T) {
	fake := newFakeCloudInit(
		fakeCloudInitRow{key: 11, name: "/user-data", contents: cloudInitFirst},
		fakeCloudInitRow{key: 12, name: "/meta-data", contents: cloudInitMeta},
	)
	got := runCloudInitRead(t, fake, []CloudInitFile{cloudInitFile("user-data", cloudInitFirst)})
	assertCloudInitFiles(t, got, []CloudInitFile{cloudInitFile("user-data", cloudInitFirst)})
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
	return runCloudInitReadExpectLive(t, fake, stateFiles, false)
}

func runCloudInitReadExpectLive(t *testing.T, fake *fakeCloudInit, stateFiles []CloudInitFile, expectLive bool) VMResourceModel {
	t.Helper()
	data, _ := runCloudInitReadExpectLivePresent(t, fake, stateFiles, expectLive)
	return data
}

func runCloudInitReadExpectLivePresent(t *testing.T, fake *fakeCloudInit, stateFiles []CloudInitFile, expectLive bool) (VMResourceModel, bool) {
	t.Helper()

	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	ctx := t.Context()
	vergeClient := vergeio.NewClient(server.URL, "user", "pass", true)
	api := mustAPI(NewVMApi(vergeClient))

	data := VMResourceModel{
		Id:             types.StringValue("7"),
		Machine:        types.Int32Value(1),
		Name:           types.StringValue("web"),
		PowerState:     types.BoolValue(false),
		CloudInitFiles: stateFiles,
		GuestAgentIPs:  types.ListNull(types.StringType),
	}
	livePresent, err := api.readVM(ctx, &data, expectLive)
	if err != nil {
		t.Fatalf("readVM: %v", err)
	}
	return data, livePresent
}

func assertCloudInitFilesModel(t *testing.T, got, want []CloudInitFile) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("files len=%d want %d; got %#v want %#v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Name.ValueString() != want[i].Name.ValueString() || got[i].Contents.ValueString() != want[i].Contents.ValueString() {
			t.Fatalf("files[%d]=%#v want %#v", i, got[i], want[i])
		}
	}
}
