// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestNASResourceNames(t *testing.T) {
	ctx := t.Context()
	cases := []struct {
		resource resource.Resource
		name     string
	}{
		{NewServiceResource(), "vergeio_nas_service"},
		{NewVolumeResource(), "vergeio_nas_volume"},
		{NewCIFSShareResource(), "vergeio_nas_cifs_share"},
		{NewNFSShareResource(), "vergeio_nas_nfs_share"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &resource.MetadataResponse{}
			tc.resource.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
			if resp.TypeName != tc.name {
				t.Fatalf("type = %s", resp.TypeName)
			}
			schemaResp := &resource.SchemaResponse{}
			tc.resource.Schema(ctx, resource.SchemaRequest{}, schemaResp)
			if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatal(diags)
			}
		})
	}
}

func TestNASModifiedIsNotKeptFromState(t *testing.T) {
	keepPrior := useStateForUnknownDescription(t.Context())
	volume := schemaOf(t, NewVolumeResource())
	assertModifier(t, volume.Attributes["modified"], keepPrior, false)
	assertModifier(t, volume.Attributes["created"], keepPrior, false)
	for _, item := range []resource.Resource{NewCIFSShareResource(), NewNFSShareResource()} {
		resp := schemaOf(t, item)
		assertModifier(t, resp.Attributes["modified"], keepPrior, false)
		assertModifier(t, resp.Attributes["status"], keepPrior, true)
		assertModifier(t, resp.Attributes["created"], keepPrior, false)
	}
	service := schemaOf(t, NewServiceResource())
	block := service.Blocks["user"].(schema.ListNestedBlock)
	assertModifier(t, block.NestedObject.Attributes["created"], keepPrior, false)
}

func TestNASSchemaProseHasNoHyphen(t *testing.T) {
	for _, item := range []resource.Resource{
		NewServiceResource(),
		NewVolumeResource(),
		NewCIFSShareResource(),
		NewNFSShareResource(),
	} {
		resp := schemaOf(t, item)
		assertNoDashProse(t, "description", resp.MarkdownDescription)
		for name, attr := range resp.Attributes {
			assertNoDashProse(t, name, attributeDescription(attr))
		}
		for name, block := range resp.Blocks {
			nested, ok := block.(schema.ListNestedBlock)
			if !ok {
				t.Fatalf("%s block type %T", name, block)
			}
			assertNoDashProse(t, name, nested.MarkdownDescription)
			for attrName, attr := range nested.NestedObject.Attributes {
				assertNoDashProse(t, name+"."+attrName, attributeDescription(attr))
			}
		}
	}
}

func TestNASTemplateProseHasNoHyphen(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	names := []string{
		"nas_service.md.tmpl",
		"nas_volume.md.tmpl",
		"nas_cifs_share.md.tmpl",
		"nas_nfs_share.md.tmpl",
	}
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(root, "templates", "resources", name))
		if err != nil {
			t.Fatal(err)
		}
		assertNoDashProse(t, name, stripTemplateActions(stripFrontMatter(string(body))))
	}
}

func TestKeepNamedUsersCopiesByName(t *testing.T) {
	state := []userModel{
		{Name: types.StringValue("alice"), DisplayName: types.StringValue("Alice"), Created: types.Int64Value(10)},
		{Name: types.StringValue("bob"), DisplayName: types.StringValue("Bob"), Created: types.Int64Value(20)},
	}
	plan := []userModel{
		{Name: types.StringValue("bob"), DisplayName: types.StringUnknown(), Created: types.Int64Unknown()},
		{Name: types.StringValue("alice"), DisplayName: types.StringUnknown(), Created: types.Int64Unknown()},
	}
	got := keepNamedUsers(plan, state)
	if got[0].DisplayName.ValueString() != "Bob" || got[0].Created.ValueInt64() != 20 {
		t.Fatalf("bob = %#v", got[0])
	}
	if got[1].DisplayName.ValueString() != "Alice" || got[1].Created.ValueInt64() != 10 {
		t.Fatalf("alice = %#v", got[1])
	}
}

func TestVolumeSecretsFollowVersion(t *testing.T) {
	config := &volumeModel{
		EncryptionKeyWO:        types.StringValue("phrase"),
		EncryptionKeyWOVersion: types.Int64Value(1),
		CIFSPasswordWO:         types.StringValue("secret"),
		CIFSPasswordWOVersion:  types.Int64Value(1),
	}
	created := volumeSecretsFrom(config, nil, true)
	if !created.SendEncryption || created.EncryptionKey != "phrase" || !created.SendCIFS {
		t.Fatalf("create secrets = %#v", created)
	}
	same := volumeSecretsFrom(config, &volumeModel{CIFSPasswordWOVersion: types.Int64Value(1)}, false)
	if same.SendEncryption || same.SendCIFS {
		t.Fatalf("unchanged secrets = %#v", same)
	}
	config.CIFSPasswordWOVersion = types.Int64Value(2)
	changed := volumeSecretsFrom(config, &volumeModel{CIFSPasswordWOVersion: types.Int64Value(1)}, false)
	if changed.SendEncryption || !changed.SendCIFS || changed.CIFSPassword != "secret" {
		t.Fatalf("changed secrets = %#v", changed)
	}
}

func TestCreateServiceRemovesRowWhenUserCreateFails(t *testing.T) {
	fix := newNASFixture(t)
	fix.failUserCreates = 1
	api := newNASTestAPI(t, fix)
	service := &serviceModel{
		Name:      types.StringValue("filesvc"),
		NetworkID: types.StringValue("3"),
		Users: []userModel{{
			Name:              types.StringValue("files"),
			PasswordWO:        types.StringValue("secret"),
			PasswordWOVersion: types.Int64Value(1),
			Enabled:           types.BoolValue(true),
		}},
	}
	err := api.createService(t.Context(), service, userSecrets(service.Users, nil, true))
	if err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("err = %v", err)
	}
	if len(fix.snapshot("service")) != 0 || len(fix.snapshot("user")) != 0 {
		t.Fatalf("services=%v users=%v", fix.snapshot("service"), fix.snapshot("user"))
	}
	if fix.countExact("POST /api/v4/vm_services") != 0 {
		t.Fatalf("direct service create was attempted: %v", fix.callLog())
	}
	if fix.countExact("POST /api/v4/vm_recipe_instances") != 1 {
		t.Fatalf("recipe deploys = %d, calls = %v", fix.countExact("POST /api/v4/vm_recipe_instances"), fix.callLog())
	}
}

func TestCreateServiceSendsPasswordOnce(t *testing.T) {
	fix := newNASFixture(t)
	api := newNASTestAPI(t, fix)
	service := &serviceModel{
		Name:       types.StringValue("filesvc"),
		NetworkID:  types.StringValue("3"),
		MaxImports: types.Int64Value(4),
		Users: []userModel{{
			Name:              types.StringValue("files"),
			PasswordWO:        types.StringValue("secret"),
			PasswordWOVersion: types.Int64Value(1),
			DisplayName:       types.StringValue("Files"),
			Enabled:           types.BoolValue(true),
		}},
	}
	if err := api.createService(t.Context(), service, userSecrets(service.Users, nil, true)); err != nil {
		t.Fatal(err)
	}
	if service.VMID.ValueString() == "" || service.VMID.ValueString() == "0" {
		t.Fatalf("vm id = %#v", service.VMID)
	}
	deploys := fix.bodies(http.MethodPost, "/api/v4/vm_recipe_instances")
	if len(deploys) != 1 {
		t.Fatalf("deploys = %#v", deploys)
	}
	answers, _ := deploys[0]["answers"].(map[string]any)
	if answers["YB_NIC_1"] != float64(3) || answers["YB_CPU_CORES"] != float64(4) || answers["YB_RAM"] != float64(4096) {
		t.Fatalf("answers = %#v", answers)
	}
	if _, ok := answers["YB_DOMAINNAME"]; ok {
		t.Fatalf("empty domain was sent: %#v", answers)
	}
	if fix.countExact("POST /api/v4/vm_services") != 0 {
		t.Fatalf("direct service create was attempted: %v", fix.callLog())
	}
	if service.Users[0].PasswordWO.ValueString() != "" {
		t.Fatal("password was stored on the user")
	}
	posts := fix.bodies(http.MethodPost, "/api/v4/vm_service_users")
	if len(posts) != 1 || posts[0]["password"] != "secret" {
		t.Fatalf("create bodies = %#v", posts)
	}
	state := *service
	state.Users = append([]userModel(nil), service.Users...)
	state.Users[0].PasswordWO = types.StringNull()
	plan := state
	plan.Users = append([]userModel(nil), state.Users...)
	plan.MaxImports = types.Int64Value(6)
	plan.Users[0].DisplayName = types.StringValue("Shared files")
	secrets := userSecrets([]userModel{{
		Name:              types.StringValue("files"),
		PasswordWO:        types.StringValue("secret"),
		PasswordWOVersion: types.Int64Value(1),
	}}, state.Users, false)
	if err := api.updateService(t.Context(), &plan, &state, secrets); err != nil {
		t.Fatal(err)
	}
	for _, body := range fix.bodies(http.MethodPut, "/api/v4/vm_service_users/"+service.Users[0].ID.ValueString()) {
		if _, ok := body["password"]; ok {
			t.Fatalf("password resent: %#v", body)
		}
	}
	if plan.Users[0].DisplayName.ValueString() != "Shared files" {
		t.Fatalf("display name = %#v", plan.Users[0].DisplayName)
	}
}

func TestNewUserRequiresPassword(t *testing.T) {
	fix := newNASFixture(t)
	api := newNASTestAPI(t, fix)
	service := &serviceModel{Name: types.StringValue("filesvc"), NetworkID: types.StringValue("3")}
	if err := api.createService(t.Context(), service, nil); err != nil {
		t.Fatal(err)
	}
	state := *service
	plan := state
	plan.Users = []userModel{{
		Name:    types.StringValue("files"),
		Enabled: types.BoolValue(true),
	}}
	err := api.updateService(t.Context(), &plan, &state, userSecrets(plan.Users, state.Users, false))
	if err == nil || !strings.Contains(err.Error(), "password_wo") {
		t.Fatalf("err = %v", err)
	}
	if len(fix.snapshot("user")) != 0 {
		t.Fatalf("user left in place: %#v", fix.snapshot("user"))
	}
}

func TestDeleteVolumeDisablesBeforeDelete(t *testing.T) {
	withPoll(t, 5)
	fix := newNASFixture(t)
	fix.put("service", "7", map[string]any{"$key": 7, "vm": 4, "name": "nas"})
	fix.put("volume", "vol1", map[string]any{
		"$key": "vol1", "id": "vol1", "name": "data", "service": 7, "enabled": true,
	})
	fix.put("cifs", "cifs1", map[string]any{
		"$key": "cifs1", "id": "cifs1", "name": "files", "volume": "vol1", "enabled": true,
	})
	fix.put("nfs", "nfs1", map[string]any{
		"$key": "nfs1", "id": "nfs1", "name": "exports", "volume": "vol1", "enabled": true,
	})
	api := newNASTestAPI(t, fix)
	if err := api.DeleteVolume(t.Context(), "vol1"); err != nil {
		t.Fatal(err)
	}
	if fix.deletedWhileEnabled {
		t.Fatalf("delete ran while the volume was enabled: %v", fix.callLog())
	}
	if len(fix.snapshot("volume")) != 0 || len(fix.snapshot("cifs")) != 0 || len(fix.snapshot("nfs")) != 0 {
		t.Fatalf("volumes=%v cifs=%v nfs=%v", fix.snapshot("volume"), fix.snapshot("cifs"), fix.snapshot("nfs"))
	}
	if !callBefore(fix.callLog(), "POST /api/v4/volume_actions", "DELETE /api/v4/volumes/vol1") {
		t.Fatalf("disable did not precede delete: %v", fix.callLog())
	}
	if !callBefore(fix.callLog(), "DELETE /api/v4/volume_cifs_shares/cifs1", "DELETE /api/v4/volumes/vol1") {
		t.Fatalf("CIFS share was not removed before the volume delete: %v", fix.callLog())
	}
}

func TestDeleteVolumeWaitsUntilDisabled(t *testing.T) {
	withPoll(t, 3)
	fix := newNASFixture(t)
	fix.suppressDisable = true
	fix.put("volume", "vol1", map[string]any{
		"$key": "vol1", "id": "vol1", "name": "data", "service": 7, "enabled": true,
	})
	api := newNASTestAPI(t, fix)
	err := api.DeleteVolume(t.Context(), "vol1")
	if err == nil || !strings.Contains(err.Error(), "still enabled") {
		t.Fatalf("err = %v", err)
	}
	if fix.deletedWhileEnabled || fix.countPrefix("DELETE /api/v4/volumes/") != 0 {
		t.Fatalf("delete was issued while enabled: %v", fix.callLog())
	}
	if len(fix.snapshot("volume")) != 1 {
		t.Fatal("volume was removed while still enabled")
	}
}

func TestCreateVolumeSendsMinimumSize(t *testing.T) {
	fix := newNASFixture(t)
	api := newNASTestAPI(t, fix)
	volume := &volumeModel{
		Name:      types.StringValue("data"),
		ServiceID: types.StringValue("7"),
		Enabled:   types.BoolValue(true),
	}
	if err := api.createVolume(t.Context(), volume, volumeSecrets{}); err != nil {
		t.Fatal(err)
	}
	posts := fix.bodies(http.MethodPost, "/api/v4/volumes")
	if len(posts) != 1 || posts[0]["maxsize"] != float64(volumeMinBytes) {
		t.Fatalf("create bodies = %#v", posts)
	}
	if volume.MaxSize.ValueInt64() != volumeMinBytes {
		t.Fatalf("max size = %#v", volume.MaxSize)
	}
	explicit := &volumeModel{
		Name:      types.StringValue("larger"),
		ServiceID: types.StringValue("7"),
		Enabled:   types.BoolValue(true),
		MaxSize:   types.Int64Value(volumeMinBytes * 2),
	}
	if err := api.createVolume(t.Context(), explicit, volumeSecrets{}); err != nil {
		t.Fatal(err)
	}
	posts = fix.bodies(http.MethodPost, "/api/v4/volumes")
	if len(posts) != 2 || posts[1]["maxsize"] != float64(volumeMinBytes*2) {
		t.Fatalf("create bodies = %#v", posts)
	}
}

func TestCreateVolumeRemovesRowWhenReadFails(t *testing.T) {
	withPoll(t, 3)
	fix := newNASFixture(t)
	fix.failVolumeGetAt = 2
	api := newNASTestAPI(t, fix)
	volume := &volumeModel{
		Name:      types.StringValue("data"),
		ServiceID: types.StringValue("7"),
		Enabled:   types.BoolValue(true),
	}
	err := api.createVolume(t.Context(), volume, volumeSecrets{})
	if err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("err = %v", err)
	}
	if len(fix.snapshot("volume")) != 0 {
		t.Fatalf("volume left in place: %#v", fix.snapshot("volume"))
	}
}

func TestCreateShareRemovesRowWhenReadFails(t *testing.T) {
	fix := newNASFixture(t)
	fix.failCIFSGetAt = 2
	api := newNASTestAPI(t, fix)
	share := &cifsModel{
		Name:     types.StringValue("files"),
		VolumeID: types.StringValue("vol1"),
		Enabled:  types.BoolValue(true),
		Comment:  types.StringValue("docs"),
	}
	err := api.createCIFS(t.Context(), share)
	if err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("err = %v", err)
	}
	if len(fix.snapshot("cifs")) != 0 {
		t.Fatalf("share left in place: %#v", fix.snapshot("cifs"))
	}
}

func TestUpdateNFSShare(t *testing.T) {
	fix := newNASFixture(t)
	api := newNASTestAPI(t, fix)
	share := &nfsModel{
		Name:         types.StringValue("exports"),
		VolumeID:     types.StringValue("vol1"),
		Enabled:      types.BoolValue(true),
		AllowedHosts: types.StringValue("10.0.0.0/8"),
		Squash:       types.StringValue("root_squash"),
		DataAccess:   types.StringValue("rw"),
	}
	if err := api.createNFS(t.Context(), share); err != nil {
		t.Fatal(err)
	}
	state := *share
	plan := state
	plan.AllowedHosts = types.StringValue("192.0.2.0/24")
	if err := api.updateNFS(t.Context(), &plan, &state); err != nil {
		t.Fatal(err)
	}
	if plan.AllowedHosts.ValueString() != "192.0.2.0/24" {
		t.Fatalf("allowed hosts = %#v", plan.AllowedHosts)
	}
	if plan.Modified.ValueInt64() == state.Modified.ValueInt64() {
		t.Fatal("modified did not advance")
	}
}

func TestDeleteServiceLeavesSnapshotVolume(t *testing.T) {
	withPoll(t, 5)
	fix := newNASFixture(t)
	fix.put("service", "7", map[string]any{"$key": 7, "vm": 4, "name": "nas"})
	fix.put("volume", "vol1", map[string]any{
		"$key": "vol1", "id": "vol1", "name": "data", "service": 7, "enabled": true,
	})
	fix.put("volume", "snap1", map[string]any{
		"$key": "snap1", "id": "snap1", "name": "data snap", "service": 7, "enabled": true, "is_snapshot": true,
	})
	api := newNASTestAPI(t, fix)
	err := api.DeleteService(t.Context(), 7)
	if err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("err = %v", err)
	}
	left := fix.snapshot("volume")
	if len(left) != 1 || fieldString(left[0]["id"]) != "snap1" {
		t.Fatalf("volumes=%v", left)
	}
	if len(fix.snapshot("service")) != 1 {
		t.Fatalf("service was removed while a snapshot volume remained: %v", fix.snapshot("service"))
	}
	if fix.countExact("DELETE /api/v4/volumes/snap1") != 0 {
		t.Fatalf("snapshot volume was deleted: %v", fix.callLog())
	}
	if fix.countExact("DELETE /api/v4/volumes/vol1") == 0 {
		t.Fatalf("data volume was not removed: %v", fix.callLog())
	}
}

func TestDeleteServiceRemovesVolumeWhenRefused(t *testing.T) {
	withPoll(t, 5)
	fix := newNASFixture(t)
	fix.put("service", "7", map[string]any{"$key": 7, "vm": 4, "name": "nas"})
	fix.put("volume", "vol1", map[string]any{
		"$key": "vol1", "id": "vol1", "name": "data", "service": 7, "enabled": true,
	})
	api := newNASTestAPI(t, fix)
	if err := api.DeleteService(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	if len(fix.snapshot("service")) != 0 || len(fix.snapshot("volume")) != 0 {
		t.Fatalf("services=%v volumes=%v", fix.snapshot("service"), fix.snapshot("volume"))
	}
	if fix.deletedWhileEnabled {
		t.Fatalf("volume delete ran while enabled: %v", fix.callLog())
	}
	if fix.countExact("DELETE /api/v4/vm_services/7") != 2 {
		t.Fatalf("service deletes = %d, calls = %v", fix.countExact("DELETE /api/v4/vm_services/7"), fix.callLog())
	}
	if fix.countPrefix("DELETE /api/v4/vms/") != 1 {
		t.Fatalf("virtual machine was not removed: %v", fix.callLog())
	}
}

func withPoll(t *testing.T, attempts int) {
	t.Helper()
	oldInterval, oldAttempts := nasPollInterval, nasPollAttempts
	nasPollInterval = 0
	nasPollAttempts = attempts
	t.Cleanup(func() {
		nasPollInterval = oldInterval
		nasPollAttempts = oldAttempts
	})
}

func schemaOf(t *testing.T, item resource.Resource) schema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	item.Schema(t.Context(), resource.SchemaRequest{}, resp)
	return resp.Schema
}

func assertModifier(t *testing.T, attr schema.Attribute, keep string, want bool) {
	t.Helper()
	var found bool
	switch item := attr.(type) {
	case schema.Int64Attribute:
		if !item.Computed || item.Optional || item.Required {
			t.Fatalf("attribute should be computed only: %#v", item)
		}
		for _, mod := range item.PlanModifiers {
			if mod.Description(t.Context()) == keep {
				found = true
			}
		}
	default:
		t.Fatalf("attribute type %T", attr)
	}
	if found != want {
		t.Fatalf("UseStateForUnknown = %v, want %v", found, want)
	}
}

func newNASTestAPI(t *testing.T, fix *nasFixture) *API {
	t.Helper()
	prevInterval, prevAttempts := nasServiceWaitInterval, nasServiceWaitAttempts
	nasServiceWaitInterval = 0
	nasServiceWaitAttempts = 2
	t.Cleanup(func() {
		nasServiceWaitInterval = prevInterval
		nasServiceWaitAttempts = prevAttempts
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		if r.URL.Path == "/version.json" {
			writeJSON(t, w, http.StatusOK, map[string]any{"version": "26.1.8"})
			return
		}
		fix.serve(w, r)
	}))
	t.Cleanup(server.Close)
	sdk, err := vergeos.NewClient(
		vergeos.WithBaseURL(server.URL),
		vergeos.WithCredentials("user", "pass"),
		vergeos.WithInsecureTLS(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &API{sdk: sdk}
}

type nasFixture struct {
	mu                  sync.Mutex
	t                   *testing.T
	next                int
	services            map[string]map[string]any
	instances           map[string]map[string]any
	vms                 map[string]map[string]any
	users               map[string]map[string]any
	volumes             map[string]map[string]any
	cifs                map[string]map[string]any
	nfs                 map[string]map[string]any
	calls               []string
	posted              []nasCall
	failUserCreates     int
	failVolumeGetAt     int
	failCIFSGetAt       int
	volumeGets          int
	cifsGets            int
	suppressDisable     bool
	deletedWhileEnabled bool
}

type nasCall struct {
	method string
	path   string
	body   map[string]any
}

func newNASFixture(t *testing.T) *nasFixture {
	t.Helper()
	return &nasFixture{
		t:         t,
		next:      20,
		services:  map[string]map[string]any{},
		instances: map[string]map[string]any{},
		vms:       map[string]map[string]any{},
		users:     map[string]map[string]any{},
		volumes:   map[string]map[string]any{},
		cifs:      map[string]map[string]any{},
		nfs:       map[string]map[string]any{},
	}
}

func (f *nasFixture) put(kind, key string, row map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.table(kind)[key] = row
}

func (f *nasFixture) snapshot(kind string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := f.table(kind)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func (f *nasFixture) table(kind string) map[string]map[string]any {
	switch kind {
	case "service":
		return f.services
	case "instance":
		return f.instances
	case "vm":
		return f.vms
	case "user":
		return f.users
	case "volume":
		return f.volumes
	case "cifs":
		return f.cifs
	case "nfs":
		return f.nfs
	default:
		f.t.Fatalf("unknown kind %s", kind)
		return nil
	}
}

func (f *nasFixture) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *nasFixture) countExact(call string) int {
	n := 0
	for _, got := range f.callLog() {
		if got == call {
			n++
		}
	}
	return n
}

func (f *nasFixture) countPrefix(prefix string) int {
	n := 0
	for _, got := range f.callLog() {
		if strings.HasPrefix(got, prefix) {
			n++
		}
	}
	return n
}

func (f *nasFixture) bodies(method, path string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, call := range f.posted {
		if call.method == method && call.path == path {
			out = append(out, call.body)
		}
	}
	return out
}

func (f *nasFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := readMap(f.t, r)
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.posted = append(f.posted, nasCall{method: r.Method, path: r.URL.Path, body: body})
	switch {
	case r.URL.Path == "/api/v4/vm_recipes" && r.Method == http.MethodGet:
		f.listRecipes(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v4/vm_recipes/"):
		f.getRecipe(w, r)
	case r.URL.Path == "/api/v4/recipe_questions" && r.Method == http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, nasRecipeQuestions())
	case r.URL.Path == "/api/v4/vm_recipe_instances" && r.Method == http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, filterRows(f.instances, r.URL.Query().Get("filter")))
	case r.URL.Path == "/api/v4/vm_recipe_instances" && r.Method == http.MethodPost:
		f.deployRecipe(w, body)
	case strings.HasPrefix(r.URL.Path, "/api/v4/vm_recipe_instances/"):
		f.item(f.instances, w, r, body, false, false)
	case strings.HasPrefix(r.URL.Path, "/api/v4/vms/"):
		f.vmItem(w, r)
	case r.URL.Path == "/api/v4/vm_services" && r.Method == http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, filterRows(f.services, r.URL.Query().Get("filter")))
	case r.URL.Path == "/api/v4/vm_services" && r.Method == http.MethodPost:
		writeJSON(f.t, w, http.StatusMethodNotAllowed, map[string]any{"err": "VM services cannot be directly created"})
	case strings.HasPrefix(r.URL.Path, "/api/v4/vm_services/"):
		f.item(f.services, w, r, body, false, true)
	case r.URL.Path == "/api/v4/vm_service_users" && r.Method == http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, filterRows(f.users, r.URL.Query().Get("filter")))
	case r.URL.Path == "/api/v4/vm_service_users" && r.Method == http.MethodPost:
		f.createUser(w, body)
	case strings.HasPrefix(r.URL.Path, "/api/v4/vm_service_users/"):
		f.item(f.users, w, r, body, false, false)
	case r.URL.Path == "/api/v4/volumes" && r.Method == http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, filterRows(f.volumes, r.URL.Query().Get("filter")))
	case r.URL.Path == "/api/v4/volumes" && r.Method == http.MethodPost:
		f.createObject(f.volumes, "vol", w, body)
	case strings.HasPrefix(r.URL.Path, "/api/v4/volumes/"):
		f.volumeItem(w, r, body)
	case r.URL.Path == "/api/v4/volume_actions" && r.Method == http.MethodPost:
		f.volumeAction(w, body)
	case r.URL.Path == "/api/v4/volume_cifs_shares" && r.Method == http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, filterRows(f.cifs, r.URL.Query().Get("filter")))
	case r.URL.Path == "/api/v4/volume_cifs_shares" && r.Method == http.MethodPost:
		f.createObject(f.cifs, "cifs", w, body)
	case strings.HasPrefix(r.URL.Path, "/api/v4/volume_cifs_shares/"):
		f.shareItem(f.cifs, &f.cifsGets, f.failCIFSGetAt, w, r, body)
	case r.URL.Path == "/api/v4/volume_nfs_shares" && r.Method == http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, filterRows(f.nfs, r.URL.Query().Get("filter")))
	case r.URL.Path == "/api/v4/volume_nfs_shares" && r.Method == http.MethodPost:
		f.createObject(f.nfs, "nfs", w, body)
	case strings.HasPrefix(r.URL.Path, "/api/v4/volume_nfs_shares/"):
		f.item(f.nfs, w, r, body, true, false)
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}
}

const nasTestRecipeKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func nasRecipeQuestions() []map[string]any {
	names := []struct {
		name string
		kind string
	}{
		{"HOSTNAME", "hostname"},
		{"YB_HOSTNAME", "string"},
		{"YB_CPU_CORES", "num"},
		{"YB_RAM", "ram"},
		{"YB_NIC_1", "network"},
		{"YB_NIC_1_IP_TYPE", "string"},
		{"YB_TIMEZONE", "string"},
		{"YB_NTP", "string"},
		{"YB_DOMAINNAME", "string"},
	}
	rows := make([]map[string]any, 0, len(names))
	for _, question := range names {
		rows = append(rows, map[string]any{
			"name":     question.name,
			"type":     question.kind,
			"enabled":  true,
			"required": question.name == "YB_NIC_1" || question.name == "YB_HOSTNAME",
			"recipe":   "vm_recipes/" + nasTestRecipeKey,
		})
	}
	return rows
}

func (f *nasFixture) listRecipes(w http.ResponseWriter, r *http.Request) {
	row := nasTestRecipeRow()
	if !rowMatches(row, r.URL.Query().Get("filter")) {
		writeJSON(f.t, w, http.StatusOK, []map[string]any{})
		return
	}
	writeJSON(f.t, w, http.StatusOK, []map[string]any{row})
}

func (f *nasFixture) getRecipe(w http.ResponseWriter, r *http.Request) {
	if pathID(r.URL.Path) != nasTestRecipeKey {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	writeJSON(f.t, w, http.StatusOK, nasTestRecipeRow())
}

func nasTestRecipeRow() map[string]any {
	return map[string]any{
		"$key":       nasTestRecipeKey,
		"id":         nasTestRecipeKey,
		"name":       "Services",
		"downloaded": true,
	}
}

func (f *nasFixture) deployRecipe(w http.ResponseWriter, body map[string]any) {
	name, _ := body["name"].(string)
	f.next++
	vmID := f.next
	f.next++
	instanceID := f.next
	f.next++
	serviceID := f.next
	f.vms[strconv.Itoa(vmID)] = map[string]any{"$key": vmID, "name": name, "powerstate": false}
	f.instances[strconv.Itoa(instanceID)] = map[string]any{
		"$key": instanceID, "name": name, "recipe": body["recipe"], "vm": vmID,
	}
	f.services[strconv.Itoa(serviceID)] = map[string]any{
		"$key": serviceID, "vm": vmID, "name": name,
		"max_imports": 4, "max_syncs": 0, "disable_swap": false,
		"read_ahead_kb_default": "0", "cifs": 1, "nfs": 2, "antivirus": 3,
	}
	writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": instanceID})
}

func (f *nasFixture) vmItem(w http.ResponseWriter, r *http.Request) {
	id := pathID(r.URL.Path)
	row, ok := f.vms[id]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, row)
	case http.MethodDelete:
		delete(f.vms, id)
		w.WriteHeader(http.StatusOK)
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}
}

func (f *nasFixture) createUser(w http.ResponseWriter, body map[string]any) {
	if f.failUserCreates > 0 {
		f.failUserCreates--
		writeJSON(f.t, w, http.StatusInternalServerError, map[string]any{"err": "user create failed"})
		return
	}
	f.next++
	id := fmt.Sprintf("user%d", f.next)
	row := cloneMap(body)
	delete(row, "password")
	row["$key"] = id
	row["id"] = id
	if _, ok := row["enabled"]; !ok {
		row["enabled"] = true
	}
	row["created"] = 1700000000
	f.users[id] = row
	writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
}

func (f *nasFixture) createObject(rows map[string]map[string]any, prefix string, w http.ResponseWriter, body map[string]any) {
	f.next++
	id := fmt.Sprintf("%s%d", prefix, f.next)
	row := cloneMap(body)
	delete(row, "encryption_key")
	delete(row, "cifs_password")
	row["$key"] = id
	row["id"] = id
	if _, ok := row["enabled"]; !ok {
		row["enabled"] = true
	}
	row["created"] = 1700000000
	row["modified"] = 1700000000
	row["status"] = 1
	rows[id] = row
	writeJSON(f.t, w, http.StatusOK, map[string]any{"$key": id})
}

func (f *nasFixture) volumeItem(w http.ResponseWriter, r *http.Request, body map[string]any) {
	if r.Method == http.MethodGet {
		f.volumeGets++
		if f.failVolumeGetAt > 0 && f.volumeGets == f.failVolumeGetAt {
			writeJSON(f.t, w, http.StatusInternalServerError, map[string]any{"err": "volume read failed"})
			return
		}
	}
	if r.Method == http.MethodDelete {
		f.deleteVolume(w, pathID(r.URL.Path))
		return
	}
	f.item(f.volumes, w, r, body, true, false)
}

func (f *nasFixture) deleteVolume(w http.ResponseWriter, id string) {
	row, ok := f.volumes[id]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	if enabled, _ := row["enabled"].(bool); enabled {
		f.deletedWhileEnabled = true
		writeJSON(f.t, w, http.StatusConflict, map[string]any{"err": "enabled"})
		return
	}
	if f.sharesOn(id) {
		writeJSON(f.t, w, http.StatusConflict, map[string]any{"err": "shares remain"})
		return
	}
	delete(f.volumes, id)
	w.WriteHeader(http.StatusOK)
}

func (f *nasFixture) shareItem(rows map[string]map[string]any, gets *int, failAt int, w http.ResponseWriter, r *http.Request, body map[string]any) {
	if r.Method == http.MethodGet {
		*gets++
		if failAt > 0 && *gets == failAt {
			writeJSON(f.t, w, http.StatusInternalServerError, map[string]any{"err": "share read failed"})
			return
		}
	}
	f.item(rows, w, r, body, true, false)
}

func (f *nasFixture) item(rows map[string]map[string]any, w http.ResponseWriter, r *http.Request, body map[string]any, bumpModified, refuseChildren bool) {
	id := pathID(r.URL.Path)
	row, ok := rows[id]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(f.t, w, http.StatusOK, row)
	case http.MethodPut:
		for key, value := range body {
			row[key] = value
		}
		if bumpModified {
			row["modified"] = 1700000001
		}
		writeJSON(f.t, w, http.StatusOK, map[string]any{})
	case http.MethodDelete:
		if refuseChildren && (f.hasChild(f.users, "service", id) || f.hasChild(f.volumes, "service", id)) {
			writeJSON(f.t, w, http.StatusConflict, map[string]any{"err": "in use"})
			return
		}
		delete(rows, id)
		w.WriteHeader(http.StatusOK)
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}
}

func (f *nasFixture) volumeAction(w http.ResponseWriter, body map[string]any) {
	id, _ := body["volume"].(string)
	row, ok := f.volumes[id]
	if !ok {
		writeJSON(f.t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		return
	}
	if body["action"] != "disable" {
		writeJSON(f.t, w, http.StatusInternalServerError, map[string]any{"err": "unexpected action"})
		return
	}
	if !f.suppressDisable {
		row["enabled"] = false
	}
	writeJSON(f.t, w, http.StatusOK, map[string]any{})
}

func (f *nasFixture) hasChild(rows map[string]map[string]any, field, id string) bool {
	for _, row := range rows {
		if fieldString(row[field]) == id {
			return true
		}
	}
	return false
}

func (f *nasFixture) sharesOn(volumeID string) bool {
	return f.hasChild(f.cifs, "volume", volumeID) || f.hasChild(f.nfs, "volume", volumeID)
}

func filterRows(rows map[string]map[string]any, filter string) []map[string]any {
	out := make([]map[string]any, 0)
	for _, row := range rows {
		if rowMatches(row, filter) {
			out = append(out, row)
		}
	}
	return out
}

func rowMatches(row map[string]any, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	for _, part := range strings.Split(filter, " and ") {
		field, value, ok := strings.Cut(strings.TrimSpace(part), " eq ")
		if !ok {
			return true
		}
		value = strings.Trim(strings.TrimSpace(value), "'")
		if fieldString(row[strings.TrimSpace(field)]) != value {
			return false
		}
	}
	return true
}

func fieldString(value any) string {
	switch n := value.(type) {
	case string:
		return n
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case json.Number:
		return n.String()
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func pathID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return parts[len(parts)-1]
}

func readMap(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read body: %v", err)
		return map[string]any{}
	}
	if len(body) == 0 {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Errorf("decode %s: %v", body, err)
		return map[string]any{}
	}
	return decoded
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode: %v", err)
	}
}

func callBefore(calls []string, earlier, later string) bool {
	early, late := -1, -1
	for i, call := range calls {
		if early < 0 && call == earlier {
			early = i
		}
		if call == later {
			late = i
		}
	}
	return early >= 0 && late >= 0 && early < late
}

func attributeDescription(attr schema.Attribute) string {
	switch item := attr.(type) {
	case schema.StringAttribute:
		return item.MarkdownDescription
	case schema.BoolAttribute:
		return item.MarkdownDescription
	case schema.Int64Attribute:
		return item.MarkdownDescription
	default:
		return ""
	}
}

func assertNoDashProse(t *testing.T, name, text string) {
	t.Helper()
	stripped := stripBackticks(text)
	if strings.ContainsRune(stripped, '-') || strings.ContainsRune(stripped, '\u2014') || strings.ContainsRune(stripped, '\u2013') {
		t.Errorf("%s prose has a dash outside backticks: %s", name, text)
	}
}

func stripFrontMatter(text string) string {
	if start := strings.Index(text, "---"); start >= 0 {
		rest := text[start+3:]
		if end := strings.Index(rest, "---"); end >= 0 {
			return rest[end+3:]
		}
	}
	return text
}

func stripTemplateActions(text string) string {
	var b strings.Builder
	for {
		start := strings.Index(text, "{{")
		if start < 0 {
			b.WriteString(text)
			break
		}
		b.WriteString(text[:start])
		rest := text[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			b.WriteString(rest)
			break
		}
		text = rest[end+2:]
	}
	return b.String()
}

func stripBackticks(text string) string {
	var b strings.Builder
	in := false
	for _, r := range text {
		if r == '`' {
			in = !in
			continue
		}
		if !in {
			b.WriteRune(r)
		}
	}
	return b.String()
}
