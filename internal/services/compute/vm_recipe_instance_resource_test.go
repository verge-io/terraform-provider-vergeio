// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

const (
	testRecipeKey  = "dddddddddddddddddddddddddddddddddddddddd"
	testCatalogKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestVMRecipeInstanceResourceSchema(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewVMRecipeInstanceResource().Schema(t.Context(), fwresource.SchemaRequest{}, resp)

	if _, exists := resp.Schema.Attributes["simulate"]; exists {
		t.Fatal("simulate must not be an argument; a dry run is not a deploy")
	}
	name := resp.Schema.Attributes["name"]
	if name == nil || !name.IsRequired() {
		t.Fatal("name should be required")
	}
	recipe := resp.Schema.Attributes["recipe_id"]
	if recipe == nil || !recipe.IsRequired() {
		t.Fatal("recipe_id should be required")
	}
	answers, ok := resp.Schema.Attributes["answers"].(resschema.MapAttribute)
	if !ok || !answers.Optional || answers.Required || !answers.Sensitive {
		t.Fatal("answers should be an optional sensitive map")
	}
	if !strings.Contains(answers.MarkdownDescription, "byte") || !strings.Contains(answers.MarkdownDescription, "53687091200") {
		t.Fatalf("answers description = %s", answers.MarkdownDescription)
	}
	if !strings.Contains(resp.Schema.MarkdownDescription, "no simulate") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
	auto, ok := resp.Schema.Attributes["auto_update"].(resschema.BoolAttribute)
	if !ok || !auto.Optional || !auto.Computed {
		t.Fatal("auto_update should be optional and computed")
	}
	timeouts, ok := resp.Schema.Blocks["timeouts"].(resschema.SingleNestedBlock)
	if !ok {
		t.Fatal("timeouts should be a single nested block")
	}
	if _, exists := timeouts.Attributes["update"]; exists {
		t.Fatal("recipe instance timeouts have no update wait")
	}
}

func TestVMRecipeInstanceMetadata(t *testing.T) {
	resp := &fwresource.MetadataResponse{}
	NewVMRecipeInstanceResource().Metadata(t.Context(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
	if resp.TypeName != "vergeio_vm_recipe_instance" {
		t.Fatalf("type = %s", resp.TypeName)
	}
}

func TestVMRecipeInstanceConfigure(t *testing.T) {
	resource := &VMRecipeInstanceResource{}
	resp := &fwresource.ConfigureResponse{}
	resource.Configure(t.Context(), fwresource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || resource.api != nil {
		t.Fatal("nil provider data should leave the resource unconfigured")
	}

	resp = &fwresource.ConfigureResponse{}
	resource.Configure(t.Context(), fwresource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() || resource.api != nil {
		t.Fatal("invalid client should fail configure")
	}

	resource = &VMRecipeInstanceResource{}
	resp = &fwresource.ConfigureResponse{}
	resource.Configure(t.Context(), fwresource.ConfigureRequest{ProviderData: versionClient(t, "25.0.0")}, resp)
	requireClientDiagnostic(t, resp.Diagnostics)
	if resource.api != nil {
		t.Fatal("API was stored after the client could not be created")
	}
}

func TestVMRecipeInstanceImportState(t *testing.T) {
	ctx := t.Context()
	resource := &VMRecipeInstanceResource{}
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	resp := &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	resource.ImportState(ctx, fwresource.ImportStateRequest{ID: "9"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got VMRecipeInstanceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Id.ValueString() != "9" {
		t.Fatalf("id = %q", got.Id.ValueString())
	}

	for _, id := range []string{"", "recipe", "0", "-1"} {
		resp = &fwresource.ImportStateResponse{State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		}}
		resource.ImportState(ctx, fwresource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("import id %q should fail", id)
		}
	}
}

func TestVMRecipeInstanceCreateDelete(t *testing.T) {
	orig := gracefulShutdownInterval
	gracefulShutdownInterval = 0
	t.Cleanup(func() { gracefulShutdownInterval = orig })

	fake := newRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	answers := recipeStringMap(t, map[string]string{
		"HOSTNAME":           "web-01",
		"SELECT_CREATE_UEFI": "yes",
		"YB_DRIVE_OS_SIZE":   strconv.FormatInt(vergeos.RecipeDiskSize50GB, 10),
		"YB_NIC_ETH0":        "Internal",
	})
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &VMRecipeInstanceResourceModel{
		Name:       types.StringValue("web-01"),
		RecipeID:   types.StringValue(testRecipeKey),
		Answers:    answers,
		AutoUpdate: types.BoolValue(true),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	created := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resource.Create(ctx, fwresource.CreateRequest{Plan: plan}, created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	var got VMRecipeInstanceResourceModel
	created.Diagnostics.Append(created.State.Get(ctx, &got)...)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	if got.Id.ValueString() != "9" || got.VMID.ValueInt64() != 44 || got.RecipeName.ValueString() != "Windows Server" {
		t.Fatalf("instance = %+v", got)
	}
	if !got.AutoUpdate.ValueBool() || got.Version.ValueString() != "2022" || got.Build.ValueInt64() != 3 {
		t.Fatalf("recipe metadata = %+v", got)
	}
	stored := map[string]string{}
	if diags := got.Answers.ElementsAs(ctx, &stored, false); diags.HasError() {
		t.Fatal(diags)
	}
	if stored["SELECT_CREATE_UEFI"] != "yes" || stored["YB_NIC_ETH0"] != "Internal" {
		t.Fatalf("answers were rewritten from the API: %#v", stored)
	}

	body := fake.deployBody()
	if strings.Contains(body, "simulate") {
		t.Fatalf("deploy body contained simulate: %s", body)
	}
	var posted map[string]any
	if err := json.Unmarshal([]byte(body), &posted); err != nil {
		t.Fatal(err)
	}
	if posted["recipe"] != testRecipeKey || posted["name"] != "web-01" || posted["auto_update"] != true {
		t.Fatalf("body = %#v", posted)
	}
	sent := posted["answers"].(map[string]any)
	if sent["SELECT_CREATE_UEFI"] != true {
		t.Fatalf("uefi = %#v", sent["SELECT_CREATE_UEFI"])
	}
	if sent["YB_NIC_ETH0"] != float64(12) {
		t.Fatalf("network = %#v", sent["YB_NIC_ETH0"])
	}
	if sent["YB_DRIVE_OS_SIZE"] != float64(vergeos.RecipeDiskSize50GB) {
		t.Fatalf("disk = %#v", sent["YB_DRIVE_OS_SIZE"])
	}
	if fake.deployCount != 1 {
		t.Fatalf("deploys = %d", fake.deployCount)
	}

	updatePlan := tfsdk.Plan{Schema: schemaResp.Schema}
	got.Timeouts = &recipeTimeoutsModel{Delete: types.StringValue("30s")}
	diags = updatePlan.Set(ctx, &got)
	if diags.HasError() {
		t.Fatal(diags)
	}
	updated := &fwresource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resource.Update(ctx, fwresource.UpdateRequest{Plan: updatePlan, State: created.State}, updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	if fake.deployCount != 1 {
		t.Fatal("changing timeouts.delete deployed again")
	}

	fake.running = true
	deleted := &fwresource.DeleteResponse{}
	resource.Delete(ctx, fwresource.DeleteRequest{State: updated.State}, deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	if fake.vmDeletes != 1 || fake.instanceDeletes != 1 {
		t.Fatalf("vm deletes = %d instance deletes = %d", fake.vmDeletes, fake.instanceDeletes)
	}
	if len(fake.actions) != 1 || !strings.Contains(fake.actions[0], `"action":"poweroff"`) {
		t.Fatalf("actions = %#v, want one poweroff", fake.actions)
	}
	if !fake.vmDeletedBeforeInstance {
		t.Fatal("instance row was deleted before the VM")
	}
}

func TestVMRecipeInstanceRefusesBoolAndDiskTraps(t *testing.T) {
	cases := []struct {
		name    string
		answers map[string]string
		want    string
	}{
		{
			name: "unrecognized bool",
			answers: map[string]string{
				"HOSTNAME":           "uefi-test",
				"SELECT_CREATE_UEFI": "enabled",
				"YB_DRIVE_OS_SIZE":   strconv.FormatInt(vergeos.RecipeDiskSize50GB, 10),
			},
			want: "not a recognized boolean",
		},
		{
			name: "disk size in gigabytes",
			answers: map[string]string{
				"HOSTNAME":         "disk-test",
				"YB_DRIVE_OS_SIZE": "50",
			},
			want: "bytes",
		},
		{
			name: "unknown question",
			answers: map[string]string{
				"HOSTNAME": "web-01",
				"NO_SUCH":  "1",
			},
			want: "unknown answer",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newRecipeFake()
			server := httptest.NewServer(fake)
			t.Cleanup(server.Close)

			ctx := t.Context()
			resource := configuredRecipeInstance(t, server.URL)
			schemaResp := &fwresource.SchemaResponse{}
			resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
			plan := tfsdk.Plan{Schema: schemaResp.Schema}
			diags := plan.Set(ctx, &VMRecipeInstanceResourceModel{
				Name:     types.StringValue("trap"),
				RecipeID: types.StringValue(testRecipeKey),
				Answers:  recipeStringMap(t, tc.answers),
			})
			if diags.HasError() {
				t.Fatal(diags)
			}
			resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			resource.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected deploy to be refused")
			}
			if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), tc.want) {
				t.Fatalf("error = %s", resp.Diagnostics.Errors()[0].Detail())
			}
			if fake.deployCount != 0 {
				t.Fatalf("refused answer was deployed %d times", fake.deployCount)
			}
		})
	}
}

func TestVMRecipeInstanceSendsNewInternalNetwork(t *testing.T) {
	fake := newRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &VMRecipeInstanceResourceModel{
		Name:     types.StringValue("internal"),
		RecipeID: types.StringValue(testRecipeKey),
		Answers: recipeStringMap(t, map[string]string{
			"HOSTNAME":    "internal",
			"YB_NIC_ETH0": "__new_internal__",
		}),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resource.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if fake.networkLists != 0 {
		t.Fatalf("network lists = %d, __new_internal__ does not need a lookup", fake.networkLists)
	}
	var posted map[string]any
	if err := json.Unmarshal([]byte(fake.deployBody()), &posted); err != nil {
		t.Fatal(err)
	}
	answers := posted["answers"].(map[string]any)
	if answers["YB_NIC_ETH0"] != "__new_internal__" {
		t.Fatalf("network = %#v", answers["YB_NIC_ETH0"])
	}
	if _, ok := posted["simulate"]; ok {
		t.Fatal("deploy JSON included simulate")
	}
}

func TestVMRecipeInstanceReadDropsMissingVM(t *testing.T) {
	fake := newRecipeFake()
	fake.vmMissing = true
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &VMRecipeInstanceResourceModel{
		Id:       types.StringValue("9"),
		Name:     types.StringValue("web-01"),
		RecipeID: types.StringValue(testRecipeKey),
		Answers:  recipeStringMap(t, map[string]string{"HOSTNAME": "web-01"}),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	resource.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing VM should remove the recipe instance from state")
	}
}

func TestVMRecipeInstanceReadDropsReusedKey(t *testing.T) {
	fake := newRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &VMRecipeInstanceResourceModel{
		Id:       types.StringValue("9"),
		Name:     types.StringValue("someone-else"),
		RecipeID: types.StringValue(testRecipeKey),
		Answers:  types.MapNull(types.StringType),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	resource.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("a reused instance key with a different name should leave state")
	}
}

func configuredRecipeInstance(t *testing.T, host string) *VMRecipeInstanceResource {
	t.Helper()
	resource := &VMRecipeInstanceResource{}
	resp := &fwresource.ConfigureResponse{}
	resource.Configure(t.Context(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resource
}

func recipeStringMap(t *testing.T, values map[string]string) types.Map {
	t.Helper()
	elems := make(map[string]attr.Value, len(values))
	for key, value := range values {
		elems[key] = types.StringValue(value)
	}
	mapped, diags := types.MapValue(types.StringType, elems)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return mapped
}

type recipeFake struct {
	mu                      sync.Mutex
	running                 bool
	vmMissing               bool
	deployCount             int
	networkLists            int
	vmDeletes               int
	instanceDeletes         int
	vmDeletedBeforeInstance bool
	actions                 []string
	deploy                  string
}

func newRecipeFake() *recipeFake {
	return &recipeFake{}
}

func (f *recipeFake) deployBody() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deploy
}

func (f *recipeFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/version.json":
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/recipe_questions":
		_, _ = w.Write([]byte(`[
			{"name":"HOSTNAME","type":"string","required":true},
			{"name":"SELECT_CREATE_UEFI","type":"bool"},
			{"name":"YB_DRIVE_OS_SIZE","type":"disksize"},
			{"name":"YB_NIC_ETH0","type":"network"}
		]`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
		f.networkLists++
		_, _ = w.Write([]byte(`[{"$key":12,"name":"Internal"}]`))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_recipe_instances":
		f.deployCount++
		f.deploy = string(body)
		_, _ = w.Write([]byte(`{"$key":9}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_recipe_instances/9":
		_, _ = w.Write([]byte(`{"$key":9,"recipe":"` + testRecipeKey + `","recipe_name":"Windows Server","name":"web-01","vm":44,"version":"2022","build":3,"auto_update":true}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/44":
		if f.vmMissing {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
			return
		}
		running := f.running
		status := "stopped"
		if running {
			status = "running"
		}
		_, _ = w.Write([]byte(`{"$key":44,"name":"web-01","powerstate":` + boolJSON(running) + `,"running":` + boolJSON(running) + `,"status":"` + status + `"}`))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
		f.actions = append(f.actions, string(body))
		if strings.Contains(string(body), `"action":"poweroff"`) || strings.Contains(string(body), `"action":"kill"`) {
			f.running = false
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vms/44":
		f.vmDeletes++
		_, _ = w.Write([]byte(`{}`))
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vm_recipe_instances/9":
		f.instanceDeletes++
		f.vmDeletedBeforeInstance = f.vmDeletes == 1
		_, _ = w.Write([]byte(`{}`))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"err":"unexpected ` + r.Method + ` ` + r.URL.RequestURI() + `"}`))
	}
}
