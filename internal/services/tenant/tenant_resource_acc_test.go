package tenant_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccTenantResources(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant")
	nodeName := acctest.Name("tenant-node")
	tier := accStorageTier(t)
	basic := testAccTenantConfig(tenantName, nodeName, tier, "Customer A", 8192, 1073741824)
	updated := testAccTenantConfig(tenantName, nodeName, tier, "Customer A updated", 16384, 2147483648)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "name", tenantName),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant.test", "id"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.test", "name", nodeName),
					resource.TestCheckResourceAttr("vergeio_tenant_node.test", "cpu_cores", "2"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.test", "ram", "8192"),
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "tier", fmt.Sprintf("%d", tier)),
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "provisioned", "1073741824"),
				),
			},
			{
				Config: basic,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "description", "Customer A updated"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.test", "ram", "16384"),
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "provisioned", "2147483648"),
				),
			},
			{
				ResourceName:            "vergeio_tenant.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password", "preferred_node"},
			},
			{
				ResourceName:      "vergeio_tenant_node.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_tenant_storage.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func accStorageTier(t *testing.T) int {
	t.Helper()
	client, err := acctest.SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	tiers, err := client.StorageTiers.List(context.Background())
	if err != nil {
		t.Fatalf("list storage tiers: %v", err)
	}
	if len(tiers) == 0 {
		t.Fatal("no storage tiers on the lab")
	}
	return tiers[0].Key
}

func testAccCheckTenantDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	// VergeOS reuses keys on tenants, tenant_nodes, and tenant_storage. A
	// Get that returns another client's row must not fail CheckDestroy (#228).
	if err := acctest.CheckDeletedMatching(s, "vergeio_tenant_node", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.TenantNodes.Get(ctx, id)
		if err != nil {
			return err
		}
		if !accAttrMatchesInt(attrs, "tenant_id", got.Tenant.Int()) {
			return &vergeos.NotFoundError{Resource: "TenantNode", ID: id}
		}
		if name := attrs["name"]; name != "" && got.Name != name {
			return &vergeos.NotFoundError{Resource: "TenantNode", ID: id}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := acctest.CheckDeletedMatching(s, "vergeio_tenant_storage", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.TenantStorage.Get(ctx, id)
		if err != nil {
			return err
		}
		if !accAttrMatchesInt(attrs, "tenant_id", got.Tenant.Int()) {
			return &vergeos.NotFoundError{Resource: "TenantStorage", ID: id}
		}
		return nil
	}); err != nil {
		return err
	}
	return acctest.CheckDeletedMatching(s, "vergeio_tenant", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.Tenants.Get(ctx, id)
		if err != nil {
			return err
		}
		if name := attrs["name"]; name != "" && got.Name != name {
			return &vergeos.NotFoundError{Resource: "Tenant", ID: id}
		}
		return nil
	})
}

// accAttrMatchesInt reports whether attrs[key] is empty (no ownership hint) or
// equals the integer VergeOS returned for that field.
func accAttrMatchesInt(attrs map[string]string, key string, got int) bool {
	want := attrs[key]
	if want == "" {
		return true
	}
	return strconv.Itoa(got) == want
}

func testAccTenantConfig(tenantName, nodeName string, tier int, description string, ram int, provisioned int64) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(nodeName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant" "test" {
  name        = %q
  description = %q
  password    = "Tf-acc-tenant-password1"
  powerstate  = false
}

resource "vergeio_tenant_node" "test" {
  tenant_id = vergeio_tenant.test.id
  name      = %q
  cpu_cores = 2
  ram       = %d
  enabled   = true
}

resource "vergeio_tenant_storage" "test" {
  tenant_id   = vergeio_tenant.test.id
  tier        = %d
  provisioned = %d
}
`, tenantName, description, nodeName, ram, tier, provisioned))
}

// TestAccTenantPowerSettleAndDestroy covers #196, #195, and #205 together:
// power on waits for terminal online (no false clean plan), power off waits
// for terminal offline, and destroy of a powered-on tenant succeeds because
// the provider waits for the tenant network to stop before Tenants.Delete.
func TestAccTenantPowerSettleAndDestroy(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-pwr")
	nodeName := acctest.Name("tenant-node-pwr")
	tier := accStorageTier(t)
	offline := testAccTenantPowerConfig(tenantName, nodeName, tier, false)
	online := testAccTenantPowerConfig(tenantName, nodeName, tier, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config: offline,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "name", tenantName),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_node.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_storage.test", "id"),
				),
			},
			{
				Config: online,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
				),
			},
			{
				Config: online,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: offline,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "offline"),
				),
			},
			{
				Config: online,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
				),
			},
			// Final step leaves powerstate=true so CheckDestroy exercises
			// node delete while the tenant is running (#195).
		},
	})
}

func testAccTenantPowerConfig(tenantName, nodeName string, tier int, power bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(nodeName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant" "test" {
  name        = %q
  description = "acc power settle"
  password    = "Tf-acc-tenant-password1"
  powerstate  = %t
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
`, tenantName, power, nodeName, tier))
}

// TestAccTenantStorageProvisionedGiB covers #197: VergeOS floors provisioned
// to whole GiB. Non-aligned values must fail at plan time; whole GiB creates
// cleanly and stays empty-plan.
func TestAccTenantStorageProvisionedGiB(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-gib")
	nodeName := acctest.Name("tenant-node-gib")
	tier := accStorageTier(t)
	aligned := testAccTenantConfig(tenantName, nodeName, tier, "gib aligned", 2048, 1073741824)
	nonAligned := testAccTenantConfig(tenantName, nodeName, tier, "gib nonaligned", 2048, 1610612736)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config:      nonAligned,
				ExpectError: regexp.MustCompile(`(?i)provisioned must be a positive multiple of 1073741824|Invalid provisioned value`),
			},
			{
				Config: aligned,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "provisioned", "1073741824"),
				),
			},
			{
				Config: aligned,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: testAccTenantConfig(tenantName, nodeName, tier, "gib aligned", 2048, 2147483648),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "provisioned", "2147483648"),
				),
			},
			{
				Config: testAccTenantConfig(tenantName, nodeName, tier, "gib aligned", 2048, 2147483648),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccTenantCreatePowerstateTrueDefers covers #207: powerstate=true on
// create with nodes in the same config must not hang for two minutes or leave
// an undestroyable running tenant network. First apply defers power-on; the
// second apply powers on once the node exists.
func TestAccTenantCreatePowerstateTrueDefers(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-defer")
	nodeName := acctest.Name("tenant-node-defer")
	tier := accStorageTier(t)
	online := testAccTenantPowerConfig(tenantName, nodeName, tier, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				// First apply defers power-on (#207). Post-apply refresh stores
				// actual offline powerstate, so the plan is not empty until a
				// second apply powers the tenant on with the node present.
				Config:             online,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "name", tenantName),
					resource.TestCheckResourceAttrSet("vergeio_tenant_node.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_storage.test", "id"),
				),
			},
			{
				Config: online,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
				),
			},
		},
	})
}

// TestAccTenantMultiNodeDestroy covers #222: destroying a tenant with two
// or more vergeio_tenant_node resources must succeed without -parallelism=1.
// VergeOS only allows deleting the highest nodeid; concurrent Terraform
// deletes retry the 405 "Only the last node can be deleted" until each node
// becomes last. powerstate=false is enough to reproduce.
func TestAccTenantMultiNodeDestroy(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-mn-destroy")
	tier := accStorageTier(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTenantMultiNodeConfig(tenantName, tier, 2, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.n.0", "name", tenantName+"-n1"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.n.1", "name", tenantName+"-n2"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_storage.test", "id"),
				),
			},
		},
	})
}

// TestAccTenantNodeRemoveWhileOnline covers #206: removing one stopped node
// from a multi-node online tenant must leave the tenant and the remaining
// node running (no whole-tenant power off).
func TestAccTenantNodeRemoveWhileOnline(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-mn")
	tier := accStorageTier(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTenantMultiNodeConfig(tenantName, tier, 1, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.n.0", "name", tenantName+"-n1"),
				),
			},
			{
				Config: testAccTenantMultiNodeConfig(tenantName, tier, 1, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
				),
			},
			{
				Config: testAccTenantMultiNodeConfig(tenantName, tier, 2, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.n.0", "name", tenantName+"-n1"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.n.1", "name", tenantName+"-n2"),
				),
			},
			{
				Config: testAccTenantMultiNodeConfig(tenantName, tier, 1, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.n.0", "name", tenantName+"-n1"),
				),
			},
			{
				Config: testAccTenantMultiNodeConfig(tenantName, tier, 1, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccTenantNodeDestroyGraceful covers #220: destroying a running tenant
// node uses TenantNodes.PowerOff (graceful) before delete. Leaves the tenant
// online through empty-plan checks, then CheckDestroy removes the running
// node and tenant.
func TestAccTenantNodeDestroyGraceful(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-grace")
	nodeName := acctest.Name("tenant-node-grace")
	tier := accStorageTier(t)
	offline := testAccTenantPowerConfig(tenantName, nodeName, tier, false)
	online := testAccTenantPowerConfig(tenantName, nodeName, tier, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config: offline,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_node.test", "id"),
				),
			},
			{
				Config: online,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
				),
			},
			{
				Config: online,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Final step leaves the tenant online so CheckDestroy powers off
			// the running node gracefully (#220) before delete.
		},
	})
}

// TestAccTenantUpdatePowerstateTrueDefersNoNodes covers #219: update with
// powerstate=true and zero vergeio_tenant_node resources must defer like
// create (#207), not PowerOn + wait two minutes. Path 1 re-applies after a
// deferred create with nodes=0. Path 2 removes the last node of an online
// tenant then re-applies powerstate=true with nodes=0.
func TestAccTenantUpdatePowerstateTrueDefersNoNodes(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-upd-defer")
	nodeName := acctest.Name("tenant-node-upd-defer")
	tier := accStorageTier(t)
	noNodes := testAccTenantPowerNoNodeConfig(tenantName, tier, true)
	withNodeOff := testAccTenantPowerConfig(tenantName, nodeName, tier, false)
	withNodeOn := testAccTenantPowerConfig(tenantName, nodeName, tier, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			// Path 1: deferred create with nodes=0, then update re-apply.
			{
				Config:             noNodes,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "name", tenantName),
					resource.TestCheckResourceAttrSet("vergeio_tenant_storage.test", "id"),
				),
			},
			{
				// Update path (#219): must defer quickly, not time out.
				// Apply keeps planned powerstate=true (same as create #207);
				// post-apply refresh stores offline and leaves a non-empty plan.
				Config:             noNodes,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "name", tenantName),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "offline"),
				),
			},
			// Path 2 setup: bring a node online, then remove it while
			// powerstate=true remains configured.
			{
				Config: withNodeOff,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_node.test", "id"),
				),
			},
			{
				Config: withNodeOn,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
				),
			},
			{
				// Last node removed; tenant goes offline on refresh. Apply state
				// may still show the prior online powerstate until refresh;
				// ExpectNonEmptyPlan covers the #219 drift into update.
				Config:             noNodes,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "name", tenantName),
					resource.TestCheckResourceAttrSet("vergeio_tenant_storage.test", "id"),
				),
			},
			{
				// Update again with nodes=0 (#219 path 2): defer, no timeout.
				Config:             noNodes,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "offline"),
				),
			},
		},
	})
}

func testAccTenantPowerNoNodeConfig(tenantName string, tier int, power bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant" "test" {
  name        = %q
  description = "acc update power defer no nodes"
  password    = "Tf-acc-tenant-password1"
  powerstate  = %t
}

resource "vergeio_tenant_storage" "test" {
  tenant_id   = vergeio_tenant.test.id
  tier        = %d
  provisioned = 1073741824
}
`, tenantName, power, tier))
}

func testAccTenantMultiNodeConfig(tenantName string, tier, nodes int, power bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant" "test" {
  name        = %q
  description = "acc multi node"
  password    = "Tf-acc-tenant-password1"
  powerstate  = %t
}

resource "vergeio_tenant_node" "n" {
  count     = %d
  tenant_id = vergeio_tenant.test.id
  name      = "%s-n${count.index + 1}"
  cpu_cores = 2
  ram       = 2048
  enabled   = true
}

resource "vergeio_tenant_storage" "test" {
  tenant_id   = vergeio_tenant.test.id
  tier        = %d
  provisioned = 1073741824
}
`, tenantName, power, nodes, tenantName, tier))
}

// TestAccTenantStorageKeyReuseDoesNotAdoptForeign covers #227: when VergeOS
// reuses a tenant_storage key after an outside delete, refresh must treat the
// foreign row as gone (plan create) instead of adopting it and RequiresReplace
// deleting the other tenant's allocation.
func TestAccTenantStorageKeyReuseDoesNotAdoptForeign(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-kr")
	nodeName := acctest.Name("tenant-node-kr")
	foreignName := acctest.Name("tenant-kr-f")
	tier := accStorageTier(t)
	config := testAccTenantConfig(tenantName, nodeName, tier, "key reuse", 2048, 1073741824)

	var managedStorageID int
	var foreignTenantID int
	var foreignStorageID int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignTenantID <= 0 {
			return
		}
		ctx := context.Background()
		if foreignStorageID > 0 {
			_ = client.TenantStorage.Delete(ctx, foreignStorageID)
		}
		_ = client.Tenants.Delete(ctx, foreignTenantID)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_tenant_storage.test", "id"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_tenant_storage.test"]
						if !ok {
							return fmt.Errorf("missing vergeio_tenant_storage.test")
						}
						id, err := strconv.Atoi(rs.Primary.ID)
						if err != nil || id <= 0 {
							return fmt.Errorf("storage id %q: %v", rs.Primary.ID, err)
						}
						managedStorageID = id
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					ctx := context.Background()
					if err := client.TenantStorage.Delete(ctx, managedStorageID); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed storage %d: %v", managedStorageID, err)
					}
					created, err := client.Tenants.Create(ctx, &vergeos.TenantCreateRequest{
						Name:     foreignName,
						Password: "Tf-acc-tenant-password1",
					})
					if err != nil {
						t.Fatalf("create foreign tenant: %v", err)
					}
					foreignTenantID = created.Key.Int()
					storage, err := client.TenantStorage.Create(ctx, &vergeos.TenantStorageCreateRequest{
						Tenant:      foreignTenantID,
						Tier:        tier,
						Provisioned: 2147483648,
					})
					if err != nil {
						t.Fatalf("create foreign storage: %v", err)
					}
					foreignStorageID = storage.Key.Int()
					keyReused = foreignStorageID == managedStorageID
					t.Logf("managed storage key=%d; foreign tenant=%d storage key=%d reused=%v",
						managedStorageID, foreignTenantID, foreignStorageID, keyReused)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_storage.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "provisioned", "1073741824"),
					func(s *terraform.State) error {
						client, err := acctest.SDKClient()
						if err != nil {
							return err
						}
						ctx := context.Background()
						got, err := client.TenantStorage.Get(ctx, foreignStorageID)
						if err != nil {
							return fmt.Errorf("foreign storage %d missing after apply (key reused=%v): %w", foreignStorageID, keyReused, err)
						}
						if got.Tenant.Int() != foreignTenantID {
							return fmt.Errorf("foreign storage tenant = %d, want %d", got.Tenant.Int(), foreignTenantID)
						}
						rs := s.RootModule().Resources["vergeio_tenant_storage.test"]
						managedID, err := strconv.Atoi(rs.Primary.ID)
						if err != nil {
							return err
						}
						if managedID == foreignStorageID {
							return fmt.Errorf("managed storage adopted foreign key %d", foreignStorageID)
						}
						managedTenant := rs.Primary.Attributes["tenant_id"]
						wantTenant := s.RootModule().Resources["vergeio_tenant.test"].Primary.ID
						if managedTenant != wantTenant {
							return fmt.Errorf("managed storage tenant_id = %s, want %s", managedTenant, wantTenant)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccTenantDestroyCheckIgnoresForeignKeyReuse covers #228: after our
// tenant/storage are deleted, VergeOS may hand the same storage key to
// another client. CheckDestroy must treat that foreign row as gone.
func TestAccTenantDestroyCheckIgnoresForeignKeyReuse(t *testing.T) {
	acctest.PreCheck(t)
	client, err := acctest.SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tier := accStorageTier(t)
	managedName := acctest.Name("tenant-ds")
	foreignName := acctest.Name("tenant-ds-f")
	nodeName := acctest.Name("tenant-node-ds")

	managed, err := client.Tenants.Create(ctx, &vergeos.TenantCreateRequest{
		Name:     managedName,
		Password: "Tf-acc-tenant-password1",
	})
	if err != nil {
		t.Fatalf("create managed tenant: %v", err)
	}
	managedTenantID := managed.Key.Int()
	t.Cleanup(func() {
		_ = client.Tenants.Delete(ctx, managedTenantID)
	})

	enabled := true
	node, err := client.TenantNodes.Create(ctx, &vergeos.TenantNodeCreateRequest{
		Tenant:   managedTenantID,
		Name:     nodeName,
		CPUCores: 1,
		RAM:      2048,
		Enabled:  &enabled,
	})
	if err != nil {
		t.Fatalf("create managed node: %v", err)
	}
	managedNodeID := node.Key.Int()
	t.Cleanup(func() {
		_ = client.TenantNodes.Delete(ctx, managedNodeID)
	})

	storage, err := client.TenantStorage.Create(ctx, &vergeos.TenantStorageCreateRequest{
		Tenant:      managedTenantID,
		Tier:        tier,
		Provisioned: 1073741824,
	})
	if err != nil {
		t.Fatalf("create managed storage: %v", err)
	}
	managedStorageID := storage.Key.Int()
	t.Cleanup(func() {
		_ = client.TenantStorage.Delete(ctx, managedStorageID)
	})

	state := &terraform.State{
		Modules: []*terraform.ModuleState{
			{
				Path: []string{"root"},
				Resources: map[string]*terraform.ResourceState{
					"vergeio_tenant.test": {
						Type: "vergeio_tenant",
						Primary: &terraform.InstanceState{
							ID: strconv.Itoa(managedTenantID),
							Attributes: map[string]string{
								"id":   strconv.Itoa(managedTenantID),
								"name": managedName,
							},
						},
					},
					"vergeio_tenant_node.test": {
						Type: "vergeio_tenant_node",
						Primary: &terraform.InstanceState{
							ID: strconv.Itoa(managedNodeID),
							Attributes: map[string]string{
								"id":        strconv.Itoa(managedNodeID),
								"tenant_id": strconv.Itoa(managedTenantID),
								"name":      nodeName,
							},
						},
					},
					"vergeio_tenant_storage.test": {
						Type: "vergeio_tenant_storage",
						Primary: &terraform.InstanceState{
							ID: strconv.Itoa(managedStorageID),
							Attributes: map[string]string{
								"id":        strconv.Itoa(managedStorageID),
								"tenant_id": strconv.Itoa(managedTenantID),
							},
						},
					},
				},
			},
		},
	}

	if err := client.TenantStorage.Delete(ctx, managedStorageID); err != nil && !vergeos.IsNotFoundError(err) {
		t.Fatalf("delete managed storage: %v", err)
	}
	if err := client.TenantNodes.Delete(ctx, managedNodeID); err != nil && !vergeos.IsNotFoundError(err) {
		t.Fatalf("delete managed node: %v", err)
	}
	if err := client.Tenants.Delete(ctx, managedTenantID); err != nil && !vergeos.IsNotFoundError(err) {
		t.Fatalf("delete managed tenant: %v", err)
	}

	foreign, err := client.Tenants.Create(ctx, &vergeos.TenantCreateRequest{
		Name:     foreignName,
		Password: "Tf-acc-tenant-password1",
	})
	if err != nil {
		t.Fatalf("create foreign tenant: %v", err)
	}
	foreignTenantID := foreign.Key.Int()
	t.Cleanup(func() {
		_ = client.Tenants.Delete(ctx, foreignTenantID)
	})
	foreignStorage, err := client.TenantStorage.Create(ctx, &vergeos.TenantStorageCreateRequest{
		Tenant:      foreignTenantID,
		Tier:        tier,
		Provisioned: 2147483648,
	})
	if err != nil {
		t.Fatalf("create foreign storage: %v", err)
	}
	foreignStorageID := foreignStorage.Key.Int()
	t.Cleanup(func() {
		_ = client.TenantStorage.Delete(ctx, foreignStorageID)
	})
	keyReused := foreignStorageID == managedStorageID
	t.Logf("managed storage key=%d; foreign tenant=%d storage key=%d reused=%v",
		managedStorageID, foreignTenantID, foreignStorageID, keyReused)

	if err := testAccCheckTenantDestroy(state); err != nil {
		t.Fatalf("CheckDestroy with foreign reused key (reused=%v): %v", keyReused, err)
	}

	// Sanity: without ownership attrs, Get-by-key alone would still fail when
	// the foreign row reused the key — confirm the foreign row is present.
	if keyReused {
		got, err := client.TenantStorage.Get(ctx, managedStorageID)
		if err != nil {
			t.Fatalf("expected foreign storage at reused key %d: %v", managedStorageID, err)
		}
		if got.Tenant.Int() != foreignTenantID {
			t.Fatalf("reused key tenant = %d, want foreign %d", got.Tenant.Int(), foreignTenantID)
		}
	}
}
