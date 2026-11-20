package compute_test

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
	"terraform-provider-vergeio/internal/services/compute"
	"terraform-provider-vergeio/internal/shared"
)

func TestAccVMListResource(t *testing.T) {
	name := acctest.Name("vmlist")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMListConfig(name),
				Check:  testAccVMListed(name),
			},
		},
	})
}

func testAccVMListed(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["vergeio_vm.test"]
		if !ok {
			return fmt.Errorf("vergeio_vm.test is missing from state")
		}
		ctx := context.Background()
		lister := compute.NewVMListResource().(list.ListResourceWithConfigure)
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
		results, err := listtest.Collect(ctx, lister, compute.NewVMResource(), shared.ListQuery{
			NamePattern: types.StringValue(name),
		}, false, 0)
		if err != nil {
			return err
		}
		if len(results) != 1 {
			return fmt.Errorf("listed %d VMs named %s", len(results), name)
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
		included, err := listtest.Collect(ctx, lister, compute.NewVMResource(), shared.ListQuery{
			NamePattern: types.StringValue(name),
		}, true, 0)
		if err != nil {
			return err
		}
		if len(included) != 1 || included[0].Resource == nil {
			return fmt.Errorf("include_resource listed %#v", included)
		}
		var model compute.VMResourceModel
		if diags := included[0].Resource.Get(ctx, &model); diags.HasError() {
			return fmt.Errorf("VM resource: %v", diags)
		}
		if model.Name.ValueString() != name || model.Id.ValueString() != rs.Primary.ID {
			return fmt.Errorf("included VM = name %s id %s", model.Name.ValueString(), model.Id.ValueString())
		}
		if !model.GuestAgentIPs.IsNull() || !model.GuestAgentIPs.ElementType(ctx).Equal(types.StringType) {
			return fmt.Errorf("guest_agent_ips = %s", model.GuestAgentIPs)
		}
		return nil
	}
}

func testAccVMListConfig(name string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm" "test" {
  name       = %q
  enabled    = true
  cpu_cores  = 1
  ram        = 1024
  powerstate = false
}
`, name))
}
