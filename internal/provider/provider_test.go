package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
)

func TestProvider(t *testing.T) {
	provider := New("dev")()
	if provider == nil {
		t.Fatal("provider should not be nil")
	}
}

func TestProviderRegistersActions(t *testing.T) {
	p := New("dev")()
	withActions, ok := p.(fwprovider.ProviderWithActions)
	if !ok {
		t.Fatal("provider does not register actions")
	}
	factories := withActions.Actions(context.Background())
	got := map[string]bool{}
	for _, factory := range factories {
		item := factory()
		resp := &action.MetadataResponse{}
		item.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
		got[resp.TypeName] = true
	}
	for _, name := range []string{
		"vergeio_vm_snapshot",
		"vergeio_network_apply",
		"vergeio_vm_power",
		"vergeio_tenant_snapshot",
		"vergeio_tenant_clone",
		"vergeio_tenant_reset",
		"vergeio_tenant_node_migrate",
		"vergeio_tenant_node_power",
	} {
		if !got[name] {
			t.Errorf("missing action %s", name)
		}
	}
	if len(got) != 8 {
		t.Fatalf("actions = %#v", got)
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
