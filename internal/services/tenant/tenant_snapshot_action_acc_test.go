// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccTenantSnapshotAction snapshots a tenant from a before_update trigger,
// then deletes the snapshot so destroy can remove the tenant. The trigger is
// on terraform_data so the tenant resource is not updated. Skipped without
// TF_ACC and on OpenTofu.
func TestAccTenantSnapshotAction(t *testing.T) {
	acctest.RequireActions(t)
	tenantName := acctest.Name("tenant-snap")
	nodeName := acctest.Name("tenant-snap-node")
	snapName := acctest.Name("tenant-snap-shot")
	tier := accStorageTier(t)
	created := testAccTenantSnapshotConfig(tenantName, nodeName, snapName, tier, "created", false)
	updated := testAccTenantSnapshotConfig(tenantName, nodeName, snapName, tier, "snapshot", true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.RequireActions(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{Config: created},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccTenantHasSnapshot("vergeio_tenant.test", snapName),
					testAccDeleteTenantSnapshot("vergeio_tenant.test", snapName),
				),
			},
		},
	})
}

func TestTenantSnapshotAcceptanceConfigParses(t *testing.T) {
	for _, withAction := range []bool{false, true} {
		src := testAccTenantSnapshotConfig("tf-acc-tenant", "tf-acc-node", "tf-acc-snap", 1, "tick", withAction)
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

func testAccTenantSnapshotConfig(tenantName, nodeName, snapName string, tier int, tick string, withAction bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(nodeName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(snapName); err != nil {
		panic(err)
	}
	lifecycle := ""
	actionBlock := ""
	if withAction {
		lifecycle = `
  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_tenant_snapshot.before]
    }
  }`
		actionBlock = fmt.Sprintf(`
action "vergeio_tenant_snapshot" "before" {
  config {
    tenant_id         = vergeio_tenant.test.id
    name              = %q
    description       = "acceptance snapshot"
    retention_seconds = 3600
  }
}
`, snapName)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant" "test" {
  name        = %q
  description = "acceptance tenant"
  password    = "Tf-acc-tenant-password1"
  powerstate  = false
}

resource "vergeio_tenant_node" "test" {
  tenant_id = vergeio_tenant.test.id
  name      = %q
  cpu_cores = 2
  ram       = 2048
  enabled   = true
}

resource "vergeio_tenant_storage" "test" {
  tenant_id   = vergeio_tenant.test.id
  tier        = %d
  provisioned = 1073741824
}

resource "terraform_data" "tick" {
  input = %q
%s
}
%s
`, tenantName, nodeName, tier, tick, lifecycle, actionBlock))
}

func testAccTenantHasSnapshot(resourceName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accTenantID(s, resourceName)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		snaps, err := client.TenantSnapshots.ListByTenant(context.Background(), id)
		if err != nil {
			return err
		}
		for _, snap := range snaps {
			if snap.Name == snapName {
				return nil
			}
		}
		return fmt.Errorf("tenant %d has no snapshot %q", id, snapName)
	}
}

func testAccDeleteTenantSnapshot(resourceName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accTenantID(s, resourceName)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		snaps, err := client.TenantSnapshots.ListByTenant(context.Background(), id)
		if err != nil {
			return err
		}
		for _, snap := range snaps {
			if snap.Name != snapName && !acctest.HasPrefix(snap.Name) {
				continue
			}
			if err := client.TenantSnapshots.Delete(context.Background(), snap.Key.Int()); err != nil && !vergeos.IsNotFoundError(err) {
				return fmt.Errorf("delete tenant snapshot %d: %w", snap.Key.Int(), err)
			}
		}
		return nil
	}
}

func accTenantID(s *terraform.State, resourceName string) (int, error) {
	rs, ok := s.RootModule().Resources[resourceName]
	if !ok {
		return 0, fmt.Errorf("resource not found: %s", resourceName)
	}
	id, err := strconv.Atoi(rs.Primary.ID)
	if err != nil {
		return 0, fmt.Errorf("tenant id %q: %w", rs.Primary.ID, err)
	}
	return id, nil
}
