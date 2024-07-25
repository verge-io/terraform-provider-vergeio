package vergeio

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccVergeioHosts_basic(t *testing.T) {
	var providers []*schema.Provider

	resourceName := "data.vergeio_authsources.empty"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories(&providers),
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceVergeioAuthsourcesConfig_empty(),
				Check: resource.ComposeTestCheckFunc(
					testAccDataSourceVergeioAuthsourcesCheck(resourceName),
				),
			},
		},
	})
}

func testAccDataSourceVergeioAuthsourcesCheck(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		rs, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("root module has no resource called %s", resourceName)
		}

		authsources, authsourcesOk := rs.Primary.Attributes["authsources.#"]

		if !authsourcesOk {
			return fmt.Errorf("authsources attribute is missing.")
		}

		_, err := strconv.Atoi(authsources)

		if err != nil {
			return fmt.Errorf("error parsing size of authsources (%s) into integer: %s", authsources, err)
		}

		// if authsourcesQuantity == 0 {
		// 	return fmt.Errorf("No authsources found, this is probably a bug.")
		// }

		return nil
	}
}

func testAccDataSourceVergeioAuthsourcesConfig_empty() string {
	return `
data "vergeio_authsources" "empty" {}
`
}
