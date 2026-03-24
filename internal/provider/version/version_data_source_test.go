package version

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
//	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestVersionDataSource(t *testing.T) {
	dataSource := NewVersionDataSource()
	if dataSource == nil {
		t.Fatal("data source should not be nil")
	}
}

// testProvider creates a minimal provider just for testing version data source
type testProvider struct{}

func (p *testProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vergeio"
}

func (p *testProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{Optional: true},
			"username": schema.StringAttribute{Optional: true},
			"password": schema.StringAttribute{Optional: true, Sensitive: true},
			"insecure": schema.BoolAttribute{Optional: true},
		},
	}
}

func (p *testProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Minimal configuration for testing
}

func (p *testProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewVersionDataSource,
	}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

/*
func TestAccVersionDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVersionDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_version.test", "name"),
					resource.TestCheckResourceAttrSet("data.vergeio_version.test", "version"),
					resource.TestCheckResourceAttrSet("data.vergeio_version.test", "hash"),
				),
			},
		},
	})
}
*/

func testAccVersionDataSourceConfig() string {
	return `
provider "vergeio" {
  # Provider configuration for testing
}

data "vergeio_version" "test" {}
`
}