// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccTenantCopy(t *testing.T) {
	acctest.PreCheck(t)
	name := acctest.Name("tenant-copy")
	sourceID := accTenantCopySource(t, name)
	config := testAccTenantCopyConfig(name, sourceID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantCopyDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_copy.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_tenant_copy.test", "tenant_id", strconv.Itoa(sourceID)),
					resource.TestCheckResourceAttr("vergeio_tenant_copy.test", "isolate", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_copy.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_copy.test", "created"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccTenantCopy_KeyReuseDestroyRefreshOff covers #376. Destroy with
// refresh off must leave the tenant that now holds the stored key.
func TestAccTenantCopy_KeyReuseDestroyRefreshOff(t *testing.T) {
	acctest.PreCheck(t)
	name := acctest.Name("tenant-copy-kr")
	foreignName := acctest.Name("tenant-copy-kr-f")
	sourceID := accTenantCopySource(t, name)
	config := testAccTenantCopyConfig(name, sourceID)

	var managedKey int
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		if err := deleteAccTenant(context.Background(), client, foreignKey); err != nil {
			t.Errorf("%v", err)
		}
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		AdditionalCLIOptions:     acctest.PlanWithoutRefresh(),
		CheckDestroy:             testAccCheckTenantCopyDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_copy.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_tenant_copy.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_copy.test", "created"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["vergeio_tenant_copy.test"]
						id, err := acctest.ResourceID(s, "vergeio_tenant_copy.test")
						if err != nil {
							return err
						}
						managedKey = id
						if rs.Primary.Attributes["created"] == "" || rs.Primary.Attributes["created"] == "0" {
							return fmt.Errorf("created is %q", rs.Primary.Attributes["created"])
						}
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
					if err := deleteAccTenant(ctx, client, managedKey); err != nil {
						t.Fatalf("delete managed tenant copy: %v", err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.Tenants.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("tenant %d", managedKey))
					created, err := client.Tenants.Create(ctx, &vergeos.TenantCreateRequest{
						Name:     foreignName,
						Password: "Tf-acc-tenant-password1",
					})
					if err != nil {
						t.Fatalf("create foreign tenant: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed tenant copy key=%d; foreign key=%d created=%d reused=%v", managedKey, foreignKey, created.Created, keyReused)
				},
				Config: acctest.Config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_copy.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: func(*terraform.State) error {
					if err := acctest.ForeignPresent(foreignKey, foreignName, keyReused, func(ctx context.Context) (string, error) {
						client, err := acctest.SDKClient()
						if err != nil {
							return "", err
						}
						got, err := client.Tenants.Get(ctx, foreignKey)
						if err != nil {
							return "", err
						}
						return got.Name, nil
					}); err != nil {
						return err
					}
					client, err := acctest.SDKClient()
					if err != nil {
						return err
					}
					if err := deleteAccTenant(context.Background(), client, foreignKey); err != nil {
						return err
					}
					foreignKey = 0
					return nil
				},
			},
		},
	})
}

func testAccTenantCopyConfig(name string, sourceID int) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant_copy" "test" {
  tenant_id = "%d"
  name      = %q
  isolate   = false
}
`, sourceID, name))
}

func testAccCheckTenantCopyDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeletedMatching(s, "vergeio_tenant_copy", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.Tenants.Get(ctx, id)
		if err != nil {
			return err
		}
		if created := attrs["created"]; created != "" && created != "0" && strconv.FormatInt(got.Created, 10) != created {
			return &vergeos.NotFoundError{Resource: "Tenant", ID: id}
		}
		if name := attrs["name"]; name != "" && got.Name != name {
			return &vergeos.NotFoundError{Resource: "Tenant", ID: id}
		}
		return nil
	})
}

func accTenantCopySource(t *testing.T, copyName string) int {
	t.Helper()
	if err := acctest.RequirePrefix(copyName); err != nil {
		t.Fatal(err)
	}
	client, err := acctest.SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	source, err := client.Tenants.Create(ctx, &vergeos.TenantCreateRequest{
		Name:     acctest.Name("tenant-copy-src"),
		Password: "Tf-acc-tenant-password1",
	})
	if err != nil {
		t.Fatalf("create source tenant: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		existing, err := client.Tenants.GetByName(cleanupCtx, copyName)
		if err != nil && !vergeos.IsNotFoundError(err) {
			t.Errorf("read tenant %s: %v", copyName, err)
		}
		if existing != nil {
			if err := deleteAccTenant(cleanupCtx, client, existing.Key.Int()); err != nil {
				t.Errorf("%v", err)
			}
		}
		if err := deleteAccTenant(cleanupCtx, client, source.Key.Int()); err != nil {
			t.Errorf("%v", err)
		}
	})
	return source.Key.Int()
}
