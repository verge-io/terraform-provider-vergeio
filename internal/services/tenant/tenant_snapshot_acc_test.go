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
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccTenantSnapshot creates a snapshot, moves its expiration, imports it
// by key and by tenant/name, reads it from vergeio_tenant_snapshots, and
// deletes it. Names start with tf-acc- so the tenant snapshot sweep can
// remove a leftover. Skipped unless TF_ACC and lab credentials are set.
func TestAccTenantSnapshot(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-snap-res")
	nodeName := acctest.Name("tenant-snap-res-node")
	snapName := acctest.Name("tenant-snap-res-shot")
	tier := accStorageTier(t)
	const firstExpiry = 1893456000
	const laterExpiry = 1924992000
	created := testAccTenantSnapshotResourceConfig(tenantName, nodeName, snapName, tier, "created", firstExpiry, false)
	extended := testAccTenantSnapshotResourceConfig(tenantName, nodeName, snapName, tier, "created", laterExpiry, false)
	kept := testAccTenantSnapshotResourceConfig(tenantName, nodeName, snapName, tier, "kept", 0, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantSnapshotDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "name", snapName),
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "description", "created"),
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "type", "full"),
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "expires", strconv.Itoa(firstExpiry)),
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "never_expires", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_snapshot.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_snapshot.test", "created"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_snapshot.test", "tenant_id", "vergeio_tenant.test", "id"),
					testAccSnapshotListed("data.vergeio_tenant_snapshots.test", snapName, strconv.Itoa(firstExpiry)),
				),
			},
			{
				Config: created,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: extended,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_snapshot.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "expires", strconv.Itoa(laterExpiry)),
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "never_expires", "false"),
					testAccSnapshotListed("data.vergeio_tenant_snapshots.test", snapName, strconv.Itoa(laterExpiry)),
				),
			},
			{
				Config: kept,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_snapshot.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "description", "kept"),
					resource.TestCheckResourceAttr("vergeio_tenant_snapshot.test", "never_expires", "true"),
					resource.TestCheckNoResourceAttr("vergeio_tenant_snapshot.test", "expires"),
					testAccSnapshotListed("data.vergeio_tenant_snapshots.test", snapName, ""),
				),
			},
			{
				Config: kept,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:      "vergeio_tenant_snapshot.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_tenant_snapshot.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccTenantSnapshotImportID(snapName),
			},
		},
	})
}

func TestTenantSnapshotResourceAcceptanceConfigParses(t *testing.T) {
	for _, never := range []bool{false, true} {
		src := testAccTenantSnapshotResourceConfig("tf-acc-tenant", "tf-acc-node", "tf-acc-snap", 1, "safety", 1893456000, never)
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

func testAccTenantSnapshotResourceConfig(tenantName, nodeName, snapName string, tier int, description string, expires int64, never bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(nodeName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(snapName); err != nil {
		panic(err)
	}
	expiry := fmt.Sprintf("\n  expires       = %d", expires)
	if never {
		expiry = "\n  never_expires = true"
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

resource "vergeio_tenant_snapshot" "test" {
  tenant_id   = vergeio_tenant.test.id
  name        = %q
  description = %q
  type        = "full"%s
}

data "vergeio_tenant_snapshots" "test" {
  tenant_id = vergeio_tenant_snapshot.test.tenant_id
}
`, tenantName, nodeName, tier, snapName, description, expiry))
}

func testAccCheckTenantSnapshotDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeletedMatching(s, "vergeio_tenant_snapshot", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.TenantSnapshots.Get(ctx, id)
		if err != nil {
			return err
		}
		if !accAttrMatchesInt(attrs, "tenant_id", got.Tenant.Int()) {
			return &vergeos.NotFoundError{Resource: "TenantSnapshot", ID: id}
		}
		if name := attrs["name"]; name != "" && got.Name != name {
			return &vergeos.NotFoundError{Resource: "TenantSnapshot", ID: id}
		}
		return nil
	}); err != nil {
		return err
	}
	return testAccCheckTenantDestroy(s)
}

func testAccSnapshotListed(dataName, snapName, expires string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ds, ok := s.RootModule().Resources[dataName]
		if !ok {
			return fmt.Errorf("data source not found: %s", dataName)
		}
		count, err := strconv.Atoi(ds.Primary.Attributes["snapshots.#"])
		if err != nil {
			return fmt.Errorf("snapshots.# = %q", ds.Primary.Attributes["snapshots.#"])
		}
		for i := 0; i < count; i++ {
			prefix := fmt.Sprintf("snapshots.%d.", i)
			if ds.Primary.Attributes[prefix+"name"] != snapName {
				continue
			}
			if ds.Primary.Attributes[prefix+"type"] == "" || ds.Primary.Attributes[prefix+"created"] == "" || ds.Primary.Attributes[prefix+"id"] == "" {
				return fmt.Errorf("snapshot %s is missing type, created, or id", snapName)
			}
			if ds.Primary.Attributes[prefix+"expires"] != expires {
				return fmt.Errorf("snapshot %s expires = %q, want %q", snapName, ds.Primary.Attributes[prefix+"expires"], expires)
			}
			return nil
		}
		return fmt.Errorf("data source has no snapshot %q (%d listed)", snapName, count)
	}
}

func testAccTenantSnapshotImportID(snapName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		tenant, ok := s.RootModule().Resources["vergeio_tenant.test"]
		if !ok {
			return "", fmt.Errorf("tenant not found")
		}
		return tenant.Primary.ID + "/" + snapName, nil
	}
}
