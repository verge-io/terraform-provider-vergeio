// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccVMClone(t *testing.T) {
	acctest.PreCheck(t)
	name := acctest.Name("vm-clone")
	snapshotID, networkID := accVMCloneFixture(t, name)
	config := testAccVMCloneConfig(name, snapshotID, networkID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMCloneDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_clone.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_vm_clone.test", "snapshot_id", strconv.Itoa(snapshotID)),
					resource.TestCheckResourceAttr("vergeio_vm_clone.test", "network_id", strconv.Itoa(networkID)),
					resource.TestCheckResourceAttrSet("vergeio_vm_clone.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_vm_clone.test", "created"),
					resource.TestCheckResourceAttrSet("vergeio_vm_clone.test", "source_vm_id"),
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

// TestAccVMClone_KeyReuseDestroyRefreshOff covers #376. Destroy with refresh
// off must leave the VM that now holds the stored key.
func TestAccVMClone_KeyReuseDestroyRefreshOff(t *testing.T) {
	acctest.PreCheck(t)
	name := acctest.Name("vm-clone-kr")
	foreignName := acctest.Name("vm-clone-kr-f")
	snapshotID, networkID := accVMCloneFixture(t, name)
	config := testAccVMCloneConfig(name, snapshotID, networkID)

	var managedKey int
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		deleteAccVM(t, client, foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		AdditionalCLIOptions:     acctest.PlanWithoutRefresh(),
		CheckDestroy:             testAccCheckVMCloneDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_clone.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_vm_clone.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_vm_clone.test", "created"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["vergeio_vm_clone.test"]
						id, err := acctest.ResourceID(s, "vergeio_vm_clone.test")
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
					deleteAccVM(t, client, managedKey)
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.VMs.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("VM %d", managedKey))
					created, err := client.VMs.Create(ctx, &vergeos.VMCreateRequest{
						Name:     foreignName,
						CPUCores: 1,
						RAM:      1024,
					})
					if err != nil {
						t.Fatalf("create foreign VM: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed VM clone key=%d; foreign key=%d created=%d reused=%v", managedKey, foreignKey, created.Created, keyReused)
				},
				Config: acctest.Config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm_clone.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: func(*terraform.State) error {
					if err := acctest.ForeignPresent(foreignKey, foreignName, keyReused, func(ctx context.Context) (string, error) {
						client, err := acctest.SDKClient()
						if err != nil {
							return "", err
						}
						got, err := client.VMs.Get(ctx, foreignKey)
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
					deleteAccVM(t, client, foreignKey)
					foreignKey = 0
					return nil
				},
			},
		},
	})
}

func testAccVMCloneConfig(name string, snapshotID, networkID int) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm_clone" "test" {
  snapshot_id = "%d"
  name        = %q
  network_id  = "%d"
}
`, snapshotID, name, networkID))
}

func testAccCheckVMCloneDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeletedMatching(s, "vergeio_vm_clone", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.VMs.Get(ctx, id)
		if err != nil {
			return err
		}
		if created := attrs["created"]; created != "" && created != "0" && strconv.FormatInt(got.Created, 10) != created {
			return &vergeos.NotFoundError{Resource: "VM", ID: id}
		}
		if name := attrs["name"]; name != "" && got.Name != name {
			return &vergeos.NotFoundError{Resource: "VM", ID: id}
		}
		return nil
	})
}

func accVMCloneFixture(t *testing.T, cloneName string) (int, int) {
	t.Helper()
	if err := acctest.RequirePrefix(cloneName); err != nil {
		t.Fatal(err)
	}
	client, err := acctest.SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	enabled := true
	vm, err := client.VMs.Create(ctx, &vergeos.VMCreateRequest{
		Name:     acctest.Name("vm-clone-src"),
		CPUCores: 1,
		RAM:      1024,
	})
	if err != nil {
		t.Fatalf("create source VM: %v", err)
	}
	network, err := client.Networks.Create(ctx, &vergeos.NetworkCreateRequest{
		Name:    acctest.Name("vm-clone-net"),
		Enabled: &enabled,
	})
	if err != nil {
		deleteAccVM(t, client, vm.Key.Int())
		t.Fatalf("create network: %v", err)
	}
	snap, err := client.VMs.Snapshot(ctx, vm.Key.Int(), &vergeos.VMSnapshotOptions{
		Name: acctest.Name("vm-clone-snap"),
	})
	if err != nil {
		if delErr := client.Networks.Delete(ctx, network.Key.Int()); delErr != nil && !vergeos.IsNotFoundError(delErr) {
			t.Errorf("delete network %d: %v", network.Key.Int(), delErr)
		}
		deleteAccVM(t, client, vm.Key.Int())
		t.Fatalf("snapshot source VM: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		existing, err := client.VMs.GetByName(cleanupCtx, cloneName)
		if err != nil && !vergeos.IsNotFoundError(err) {
			t.Errorf("read VM %s: %v", cloneName, err)
		}
		if existing != nil {
			deleteAccVM(t, client, existing.Key.Int())
		}
		if err := client.VMSnapshots.Delete(cleanupCtx, snap.Key.Int()); err != nil && !vergeos.IsNotFoundError(err) {
			t.Errorf("delete snapshot %d: %v", snap.Key.Int(), err)
		}
		if err := client.Networks.Delete(cleanupCtx, network.Key.Int()); err != nil && !vergeos.IsNotFoundError(err) {
			t.Errorf("delete network %d: %v", network.Key.Int(), err)
		}
		deleteAccVM(t, client, vm.Key.Int())
	})
	return snap.Key.Int(), network.Key.Int()
}

func deleteAccVM(t *testing.T, client *vergeos.Client, id int) {
	t.Helper()
	if id <= 0 || client == nil {
		return
	}
	ctx := context.Background()
	powerErr := client.VMs.PowerOffWithOptions(ctx, id, &vergeos.VMPowerOffOptions{
		Timeout:           15 * time.Second,
		PollInterval:      time.Second,
		ForceAfterTimeout: true,
	})
	if err := client.VMs.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		if powerErr != nil && !vergeos.IsNotFoundError(powerErr) {
			t.Errorf("power off VM %d: %v", id, powerErr)
		}
		t.Errorf("delete VM %d: %v", id, err)
	}
}
