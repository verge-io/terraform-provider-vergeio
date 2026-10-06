package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
)

func TestProvider(t *testing.T) {
	provider := New("dev")()
	if provider == nil {
		t.Fatal("provider should not be nil")
	}
}

func TestProviderRegistersAPIKeyEphemeralResource(t *testing.T) {
	p := New("dev")()
	withEphemeral, ok := p.(fwprovider.ProviderWithEphemeralResources)
	if !ok {
		t.Fatal("provider does not register ephemeral resources")
	}
	factories := withEphemeral.EphemeralResources(context.Background())
	if len(factories) != 1 {
		t.Fatalf("ephemeral resources = %d, want 1", len(factories))
	}
	resource := factories[0]()
	resp := &ephemeral.MetadataResponse{}
	resource.Metadata(context.Background(), ephemeral.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
	if resp.TypeName != "vergeio_api_key" {
		t.Fatalf("type = %s", resp.TypeName)
	}
}
