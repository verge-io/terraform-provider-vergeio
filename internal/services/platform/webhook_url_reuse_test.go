// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestWebhookURLReadDropsReusedKey(t *testing.T) {
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/webhook_urls/5":
			_, _ = w.Write([]byte(`{"$key":5,"name":"other","url":"https://example.com/other"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/webhook_urls/5":
			puts++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredWebhookURL(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &webhookURLModel{
		ID:   types.StringValue("5"),
		Name: types.StringValue("mine"),
		URL:  types.StringValue("https://example.com/mine"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	item.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("a reused webhook key with a different name should leave state")
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = plan.Set(ctx, &webhookURLModel{
		ID:   types.StringValue("5"),
		Name: types.StringValue("mine"),
		URL:  types.StringValue("https://example.com/mine"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	updated := &fwresource.UpdateResponse{State: state}
	item.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state, Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}}, updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("update should refuse a reused webhook key")
	}
	if puts != 0 {
		t.Fatalf("update wrote the foreign webhook %d times", puts)
	}
	if !vergeos.IsNotFoundError(item.api.readWebhookURL(ctx, &webhookURLModel{
		ID:   types.StringValue("5"),
		Name: types.StringValue("mine"),
	})) {
		t.Fatal("read should report the reused key as not found")
	}
}

func TestWebhookURLReadKeepsSameNameAndImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case "/api/v4/webhook_urls/5":
			_, _ = w.Write([]byte(`{"$key":5,"name":"mine","url":"https://example.com/renamed-path"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredWebhookURL(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &webhookURLModel{
		ID:   types.StringValue("5"),
		Name: types.StringValue("mine"),
		URL:  types.StringValue("https://example.com/mine"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	item.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got webhookURLModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Name.ValueString() != "mine" || got.URL.ValueString() != "https://example.com/renamed-path" {
		t.Fatalf("same name should refresh in place, got name %s url %s", got.Name, got.URL)
	}

	imported := tfsdk.State{Schema: schemaResp.Schema}
	diags = imported.Set(ctx, &webhookURLModel{ID: types.StringValue("5")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	importedResp := &fwresource.ReadResponse{State: imported}
	item.Read(ctx, fwresource.ReadRequest{State: imported}, importedResp)
	if importedResp.Diagnostics.HasError() || importedResp.State.Raw.IsNull() {
		t.Fatalf("import read should keep the row: %v null=%t", importedResp.Diagnostics, importedResp.State.Raw.IsNull())
	}
}

func TestWebhookURLImportReadKeepsZero(t *testing.T) {
	const row = `{"$key":1,"name":"tf-acc-hook","url":"https://example.com/tf-acc-hook","timeout":0,"retries":0,"allow_insecure":false}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var body string
		switch r.URL.Path {
		case "/version.json":
			body = `{"version":"26.2.2"}`
		case "/api/v4/webhook_urls/1":
			if r.Method != http.MethodGet {
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			body = row
		default:
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write %s: %v", r.URL.Path, err)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredWebhookURL(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	imported := tfsdk.State{Schema: schemaResp.Schema}
	diags := imported.Set(ctx, &webhookURLModel{ID: types.StringValue("1")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: imported}
	item.Read(ctx, fwresource.ReadRequest{State: imported}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got webhookURLModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Retries.IsNull() || got.Retries.IsUnknown() || got.Retries.ValueInt64() != 0 {
		t.Fatalf("retries = %#v", got.Retries)
	}
	if got.Timeout.IsNull() || got.Timeout.IsUnknown() || got.Timeout.ValueInt64() != 0 {
		t.Fatalf("timeout = %#v", got.Timeout)
	}
	if got.AllowInsecure.IsNull() || got.AllowInsecure.IsUnknown() || got.AllowInsecure.ValueBool() {
		t.Fatalf("allow_insecure = %#v", got.AllowInsecure)
	}
}

func TestWebhookURLDeleteChecksName(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		deliveries string
		storedName string
		wantDelete bool
		wantErr    bool
	}{
		{name: "foreign", status: http.StatusOK, body: `{"$key":5,"name":"other","url":"https://example.com/other"}`, deliveries: `[{"$key":8,"webhook_url":5,"message":"{}","status":"queued"}]`, storedName: "mine"},
		{name: "same", status: http.StatusOK, body: `{"$key":5,"name":"mine","url":"https://example.com/renamed-path"}`, deliveries: `[]`, storedName: "mine", wantDelete: true},
		{name: "missing", status: http.StatusNotFound, deliveries: `[]`, storedName: "mine", wantDelete: true},
		{name: "read error", status: http.StatusInternalServerError, storedName: "mine", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var deletes int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/webhook_urls/5":
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/webhooks":
					_, _ = w.Write([]byte(tc.deliveries))
				case r.Method == http.MethodDelete:
					deletes++
					if tc.status == http.StatusNotFound && r.URL.Path == "/api/v4/webhook_urls/5" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					_, _ = w.Write([]byte(`{"response":"OK"}`))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			ctx := context.Background()
			item := configuredWebhookURL(t, server.URL)
			schemaResp := &fwresource.SchemaResponse{}
			item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, &webhookURLModel{
				ID:   types.StringValue("5"),
				Name: types.StringValue(tc.storedName),
				URL:  types.StringValue("https://example.com/mine"),
			})
			if diags.HasError() {
				t.Fatal(diags)
			}
			resp := &fwresource.DeleteResponse{}
			item.Delete(ctx, fwresource.DeleteRequest{State: state}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("error = %v, want error %t", resp.Diagnostics, tc.wantErr)
			}
			if deletes > 0 != tc.wantDelete {
				t.Fatalf("deletes = %d, want delete %t", deletes, tc.wantDelete)
			}
		})
	}
}

func configuredWebhookURL(t *testing.T, host string) *webhookURLResource {
	t.Helper()
	item := &webhookURLResource{}
	resp := &fwresource.ConfigureResponse{}
	item.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}
