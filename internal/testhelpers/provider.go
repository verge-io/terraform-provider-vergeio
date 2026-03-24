package testhelpers

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"terraform-provider-vergeio/internal/provider"
)

// ProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var ProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// TestAccPreCheck validates the necessary test API keys exist
// in the testing environment
func TestAccPreCheck(t *testing.T) {
	// Add any prerequisite checks here, such as:
	// - Required environment variables
	// - Test connectivity to VergeOS instance
	// - Minimum required credentials
}

// ProviderConfig returns a basic provider configuration for testing
func ProviderConfig() string {
	return `
provider "vergeio" {
  # Configuration will be added based on test environment setup
}
`
}