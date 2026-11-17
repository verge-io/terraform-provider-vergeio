// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/listtest"
	"terraform-provider-vergeio/internal/services/identity"
	"terraform-provider-vergeio/internal/shared"
)

func TestAccGroupListResource(t *testing.T) {
	name := acctest.Name("grouplist")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupListDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGroupListConfig(name),
				Check:  testAccGroupListed(name),
			},
		},
	})
}

func testAccGroupListed(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["vergeio_group.test"]
		if !ok {
			return fmt.Errorf("vergeio_group.test is missing from state")
		}
		ctx := context.Background()
		lister := identity.NewGroupListResource().(list.ListResourceWithConfigure)
		resp := &fwresource.ConfigureResponse{}
		lister.Configure(ctx, fwresource.ConfigureRequest{
			ProviderData: vergeio.NewClient(
				os.Getenv("TF_ACC_VERGEIO_HOST"),
				os.Getenv("TF_ACC_VERGEIO_USERNAME"),
				os.Getenv("TF_ACC_VERGEIO_PASSWORD"),
				true,
			),
		}, resp)
		if resp.Diagnostics.HasError() {
			return fmt.Errorf("configure list resource: %v", resp.Diagnostics)
		}
		results, err := listtest.Collect(ctx, lister, identity.NewGroupResource(), shared.ListQuery{
			NamePattern: types.StringValue(name),
		}, false, 0)
		if err != nil {
			return err
		}
		if len(results) != 1 {
			return fmt.Errorf("listed %d groups named %s", len(results), name)
		}
		if results[0].DisplayName != name {
			return fmt.Errorf("display name = %s, want %s", results[0].DisplayName, name)
		}
		id, err := listtest.IdentityID(ctx, results[0])
		if err != nil {
			return err
		}
		if id != rs.Primary.ID {
			return fmt.Errorf("identity id = %s, state id = %s", id, rs.Primary.ID)
		}
		excluded, err := listtest.Collect(ctx, lister, identity.NewGroupResource(), shared.ListQuery{
			NamePattern: types.StringValue("no-such-tf-acc-*"),
		}, false, 0)
		if err != nil {
			return err
		}
		if len(excluded) != 0 {
			return fmt.Errorf("exclusion pattern listed %d groups", len(excluded))
		}
		return nil
	}
}

func testAccCheckGroupListDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_group", func(ctx context.Context, id int) error {
		_, err := client.Groups.Get(ctx, id)
		return err
	})
}

func testAccGroupListConfig(name string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_group" "test" {
  name        = %q
  description = "listed by query"
  enabled     = true
}
`, name))
}
