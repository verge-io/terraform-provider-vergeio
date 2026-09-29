package provider

import (
	"context"
	"testing"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestConfigureBuildsHTTPClient(t *testing.T) {
	ctx := context.Background()
	p := &vergeioProvider{version: "test"}

	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}

	raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
		"host":     tftypes.NewValue(tftypes.String, "example.test"),
		"username": tftypes.NewValue(tftypes.String, "api-user"),
		"password": tftypes.NewValue(tftypes.String, "api-pass"),
		"insecure": tftypes.NewValue(tftypes.Bool, true),
	})

	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{
		Config: tfsdk.Config{
			Raw:    raw,
			Schema: schemaResp.Schema,
		},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", resp.Diagnostics)
	}

	client, ok := resp.ResourceData.(*vergeio.Client)
	if !ok || client == nil {
		t.Fatalf("ResourceData = %#v, want *vergeio.Client", resp.ResourceData)
	}
	if resp.DataSourceData != client {
		t.Fatal("data sources and resources were given different clients")
	}
	if client.Timeout() != vergeio.DefaultTimeout || client.Timeout() <= 0 {
		t.Fatalf("Timeout() = %s, want %s", client.Timeout(), vergeio.DefaultTimeout)
	}
	if client.Host != "example.test" || client.Username != "api-user" || client.Password != "api-pass" || !client.Insecure {
		t.Fatalf("client = host %q user %q insecure %v", client.Host, client.Username, client.Insecure)
	}
	if client.FieldCache == nil {
		t.Fatal("field cache was not initialized")
	}
}
