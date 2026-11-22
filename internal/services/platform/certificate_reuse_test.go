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

func TestCertificateReadDropsReusedKey(t *testing.T) {
	var puts int
	const row = `{"$key":5,"type":"self_signed","domain":"other.example.com","domainname":"other.example.com","domainlist":"other.example.com","description":"belongs to someone else","created":1800000000}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/certificates/5":
			_, _ = w.Write([]byte(row))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/certificates/5":
			puts++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredCertificate(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &certificateModel{
		ID:          types.StringValue("5"),
		Type:        types.StringValue(vergeos.CertificateTypeSelfSigned),
		DomainName:  types.StringValue("mine.example.com"),
		Description: types.StringValue("mine"),
		Created:     types.Int64Value(1700000000),
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
		t.Fatal("a reused certificate key with a different created time should leave state")
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = plan.Set(ctx, &certificateModel{
		ID:          types.StringValue("5"),
		Type:        types.StringValue(vergeos.CertificateTypeSelfSigned),
		DomainName:  types.StringValue("mine.example.com"),
		Description: types.StringValue("mine"),
		Created:     types.Int64Value(1700000000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	updated := &fwresource.UpdateResponse{State: state}
	item.Update(ctx, fwresource.UpdateRequest{
		Plan:   plan,
		State:  state,
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw},
	}, updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("update should refuse a reused certificate key")
	}
	if puts != 0 {
		t.Fatalf("update wrote the foreign certificate %d times", puts)
	}
	if !vergeos.IsNotFoundError(item.api.readCertificate(ctx, &certificateModel{
		ID:         types.StringValue("5"),
		Type:       types.StringValue(vergeos.CertificateTypeSelfSigned),
		DomainName: types.StringValue("mine.example.com"),
		Created:    types.Int64Value(1700000000),
	})) {
		t.Fatal("read should report the reused key as not found")
	}
}

func TestCertificateReadKeepsSameCreatedAndImport(t *testing.T) {
	const row = `{"$key":5,"type":"self_signed","domain":"mine.example.com","domainname":"mine.example.com","domainlist":"mine.example.com","description":"edited outside","created":1700000000}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case "/api/v4/certificates/5":
			_, _ = w.Write([]byte(row))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredCertificate(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &certificateModel{
		ID:          types.StringValue("5"),
		Type:        types.StringValue(vergeos.CertificateTypeSelfSigned),
		DomainName:  types.StringValue("mine.example.com"),
		Description: types.StringValue("mine"),
		Created:     types.Int64Value(1700000000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	item.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got certificateModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Description.ValueString() != "edited outside" || got.Created.ValueInt64() != 1700000000 {
		t.Fatalf("same created time should refresh in place, got description %s created %s", got.Description, got.Created)
	}

	imported := tfsdk.State{Schema: schemaResp.Schema}
	diags = imported.Set(ctx, &certificateModel{ID: types.StringValue("5")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	importedResp := &fwresource.ReadResponse{State: imported}
	item.Read(ctx, fwresource.ReadRequest{State: imported}, importedResp)
	if importedResp.Diagnostics.HasError() || importedResp.State.Raw.IsNull() {
		t.Fatalf("import read should keep the row: %v null=%t", importedResp.Diagnostics, importedResp.State.Raw.IsNull())
	}
	var filled certificateModel
	importedResp.Diagnostics.Append(importedResp.State.Get(ctx, &filled)...)
	if filled.Created.ValueInt64() != 1700000000 || filled.Description.ValueString() != "edited outside" {
		t.Fatalf("import read = description %s created %s", filled.Description, filled.Created)
	}
}

func TestCertificateDeleteChecksCreated(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		created    int64
		wantDelete bool
		wantErr    bool
	}{
		{name: "foreign", status: http.StatusOK, body: `{"$key":5,"domainname":"other.example.com","type":"self_signed","created":1800000000}`, created: 1700000000},
		{name: "same", status: http.StatusOK, body: `{"$key":5,"domainname":"mine.example.com","type":"self_signed","created":1700000000}`, created: 1700000000, wantDelete: true},
		{name: "missing", status: http.StatusNotFound, created: 1700000000, wantDelete: true},
		{name: "read error", status: http.StatusInternalServerError, created: 1700000000, wantErr: true},
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
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/certificates/5":
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/certificates/5":
					deletes++
					if tc.status == http.StatusNotFound {
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
			item := configuredCertificate(t, server.URL)
			schemaResp := &fwresource.SchemaResponse{}
			item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, &certificateModel{
				ID:      types.StringValue("5"),
				Created: types.Int64Value(tc.created),
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
