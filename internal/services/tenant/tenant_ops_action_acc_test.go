// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccTenantCloneAction copies a disposable tenant into another disposable
// name and skips the network, storage, and nodes. The copy is deleted in the
// check. The sweeper also removes any leftover whose name starts with tf-acc-.
// Skipped without TF_ACC and on OpenTofu.
func TestAccTenantCloneAction(t *testing.T) {
	acctest.RequireActions(t)
	tenantName := acctest.Name("tenant-clone")
	nodeName := acctest.Name("tenant-clone-node")
	cloneName := acctest.Name("tenant-clone-copy")
	tier := accStorageTier(t)
	created := testAccTenantOpsConfig(tenantName, nodeName, tier, false, "created", "", "")
	cloned := testAccTenantOpsConfig(tenantName, nodeName, tier, false, "clone", tenantCloneTrigger, fmt.Sprintf(`
action "vergeio_tenant_clone" "copy" {
  config {
    tenant_id  = vergeio_tenant.test.id
    name       = %q
    no_vnet    = true
    no_storage = true
    no_nodes   = true
  }
}
`, cloneName))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.RequireActions(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{Config: created},
			{
				Config: cloned,
				Check:  testAccDeleteClonedTenant(cloneName),
			},
		},
	})
}

// TestAccTenantResetAction restarts a disposable tenant after it is online.
// The trigger is on terraform_data so vergeio_tenant is not updated in the
// same apply.
func TestAccTenantResetAction(t *testing.T) {
	acctest.RequireActions(t)
	tenantName := acctest.Name("tenant-reset")
	nodeName := acctest.Name("tenant-reset-node")
	tier := accStorageTier(t)
	offline := testAccTenantOpsConfig(tenantName, nodeName, tier, false, "created", "", "")
	online := testAccTenantOpsConfig(tenantName, nodeName, tier, true, "online", "", "")
	reset := testAccTenantOpsConfig(tenantName, nodeName, tier, true, "reset", tenantResetTrigger, `
action "vergeio_tenant_reset" "restart" {
  config {
    tenant_id = vergeio_tenant.test.id
  }
}
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.RequireActions(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{Config: offline},
			{
				Config: online,
				Check:  resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
			},
			{
				Config: reset,
				Check:  testAccTenantOnline("vergeio_tenant.test"),
			},
		},
	})
}

// TestAccTenantNodePowerAction resets, then kills, the node of a disposable
// tenant. The tenant is powered on first so the node is running. Kill of the
// only node takes the tenant offline while powerstate stays true, so that
// step's refresh plan is not empty. The last step declares powerstate false.
func TestAccTenantNodePowerAction(t *testing.T) {
	acctest.RequireActions(t)
	tenantName := acctest.Name("tenant-node-power")
	nodeName := acctest.Name("tenant-node-power-node")
	tier := accStorageTier(t)
	offline := testAccTenantNodePowerConfig(tenantName, nodeName, tier, false, "created", "")
	online := testAccTenantNodePowerConfig(tenantName, nodeName, tier, true, "online", "")
	reset := testAccTenantNodePowerConfig(tenantName, nodeName, tier, true, "reset", "reset")
	killed := testAccTenantNodePowerConfig(tenantName, nodeName, tier, true, "kill", "kill")
	stopped := testAccTenantNodePowerConfig(tenantName, nodeName, tier, false, "stopped", "")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.RequireActions(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{Config: offline},
			{
				Config: online,
				Check:  resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
			},
			{
				Config: reset,
				Check:  testAccTenantNodePresent("vergeio_tenant_node.test"),
			},
			{
				Config:             killed,
				ExpectNonEmptyPlan: true,
				Check:              testAccTenantNodeStopped("vergeio_tenant_node.test"),
			},
			{Config: stopped},
		},
	})
}

// TestAccTenantNodeMigrateAction moves a disposable tenant node onto the host
// named by TF_ACC_VERGEIO_TARGET_NODE. It skips when that variable is unset,
// because a migrate without a chosen host has nowhere to go.
func TestAccTenantNodeMigrateAction(t *testing.T) {
	acctest.RequireActions(t)
	target := accTargetNode(t)
	tenantName := acctest.Name("tenant-node-move")
	nodeName := acctest.Name("tenant-node-move-node")
	tier := accStorageTier(t)
	offline := testAccTenantOpsConfig(tenantName, nodeName, tier, false, "created", "", "")
	online := testAccTenantOpsConfig(tenantName, nodeName, tier, true, "online", "", "")
	moved := testAccTenantOpsConfig(tenantName, nodeName, tier, true, "move", tenantNodeMigrateTrigger, fmt.Sprintf(`
action "vergeio_tenant_node_migrate" "host" {
  config {
    tenant_node_id = vergeio_tenant_node.test.id
    target_node    = %q
  }
}
`, strconv.Itoa(target)))

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.RequireActions(t)
			accTargetNode(t)
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{Config: offline},
			{
				Config: online,
				Check:  resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
			},
			{
				Config: moved,
				Check:  testAccTenantNodePresent("vergeio_tenant_node.test"),
			},
		},
	})
}

func TestTenantOpsAcceptanceConfigParses(t *testing.T) {
	sources := []string{
		testAccTenantOpsConfig("tf-acc-tenant", "tf-acc-node", 1, false, "tick", "", ""),
		testAccTenantOpsConfig("tf-acc-tenant", "tf-acc-node", 1, false, "tick", tenantCloneTrigger, `
action "vergeio_tenant_clone" "copy" {
  config {
    tenant_id  = vergeio_tenant.test.id
    name       = "tf-acc-tenant-copy"
    no_vnet    = true
    no_storage = true
    no_nodes   = true
  }
}
`),
		testAccTenantNodePowerConfig("tf-acc-tenant", "tf-acc-node", 1, true, "tick", "kill"),
	}
	for _, src := range sources {
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

const tenantCloneTrigger = `
  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_tenant_clone.copy]
    }
  }`

const tenantResetTrigger = `
  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_tenant_reset.restart]
    }
  }`

const tenantNodePowerTrigger = `
  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_tenant_node_power.op]
    }
  }`

const tenantNodeMigrateTrigger = `
  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_tenant_node_migrate.host]
    }
  }`

func testAccTenantNodePowerConfig(tenantName, nodeName string, tier int, power bool, tick, operation string) string {
	trigger := ""
	action := ""
	if operation != "" {
		trigger = tenantNodePowerTrigger
		action = fmt.Sprintf(`
action "vergeio_tenant_node_power" "op" {
  config {
    tenant_node_id = vergeio_tenant_node.test.id
    operation      = %q
  }
}
`, operation)
	}
	return testAccTenantOpsConfig(tenantName, nodeName, tier, power, tick, trigger, action)
}

func testAccTenantOpsConfig(tenantName, nodeName string, tier int, power bool, tick, lifecycle, action string) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(nodeName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant" "test" {
  name        = %q
  description = "acceptance tenant"
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

resource "terraform_data" "tick" {
  input = %q
%s
}
%s
`, tenantName, power, nodeName, tier, tick, lifecycle, action))
}

func accTargetNode(t *testing.T) int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("TF_ACC_VERGEIO_TARGET_NODE"))
	if raw == "" {
		t.Skip("set TF_ACC_VERGEIO_TARGET_NODE to a host node key to migrate an acceptance tenant node")
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		t.Fatalf("TF_ACC_VERGEIO_TARGET_NODE = %q, want a positive integer", raw)
	}
	return id
}

func testAccTenantOnline(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accTenantID(s, resourceName)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		ctx := context.Background()
		if _, err := client.Tenants.Get(ctx, id); err != nil {
			return fmt.Errorf("tenant %d: %w", id, err)
		}
		return waitTenantStatus(ctx, client, id, true)
	}
}

func testAccTenantNodePresent(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accStateID(s, resourceName)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		if _, err := client.TenantNodes.Get(context.Background(), id); err != nil {
			return fmt.Errorf("tenant node %d: %w", id, err)
		}
		return nil
	}
}

func testAccTenantNodeStopped(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accStateID(s, resourceName)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		ctx := context.Background()
		node, err := client.TenantNodes.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("tenant node %d: %w", id, err)
		}
		if err := waitTenantNodeMachineStopped(ctx, client, id, node.Machine.Int()); err != nil {
			return err
		}
		// The refresh plan after this check reads tenant status. Wait until
		// that status is offline so powerstate drifts from the declared true.
		return waitTenantStatus(ctx, client, node.Tenant.Int(), false)
	}
}

func waitTenantNodeMachineStopped(ctx context.Context, client *vergeos.Client, nodeID, machineID int) error {
	if machineID <= 0 {
		return nil
	}
	deadline := time.Now().Add(2 * time.Minute)
	var last error
	for {
		status, err := client.MachineStatus.Get(ctx, machineID)
		if err != nil {
			last = err
		} else if !status.Running {
			return nil
		} else {
			last = fmt.Errorf("tenant node %d machine %d still running", nodeID, machineID)
		}
		if !time.Now().Before(deadline) {
			return last
		}
		time.Sleep(2 * time.Second)
	}
}

func waitTenantStatus(ctx context.Context, client *vergeos.Client, tenantID int, wantOn bool) error {
	deadline := time.Now().Add(2 * time.Minute)
	var last error
	for {
		status, err := client.TenantStatus.Get(ctx, tenantID)
		if err != nil {
			last = err
		} else if tenantStatusOn(status) == wantOn {
			return nil
		} else if wantOn {
			last = fmt.Errorf("tenant %d status %q is not online", tenantID, status.Status)
		} else {
			last = fmt.Errorf("tenant %d status %q is still on", tenantID, status.Status)
		}
		if !time.Now().Before(deadline) {
			return last
		}
		time.Sleep(2 * time.Second)
	}
}

func tenantStatusOn(status *vergeos.TenantStatus) bool {
	if status == nil || status.Starting || status.Stopping {
		return false
	}
	if status.Running {
		return true
	}
	switch status.Status {
	case "online", "migrating", "restarting", "reduced":
		return true
	default:
		return false
	}
}

func testAccDeleteClonedTenant(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := acctest.RequirePrefix(name); err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		ctx := context.Background()
		tenant, err := waitTenantByName(ctx, client, name)
		if err != nil {
			return err
		}
		// The clone name is visible before VergeOS drops the skipped rows.
		// Keep the zero-node and zero-storage requirement, and read until
		// the copy has finished or the wait expires.
		deadline := time.Now().Add(2 * time.Minute)
		var nodes []vergeos.TenantNode
		var storage []vergeos.TenantStorage
		for {
			nodes, err = client.TenantNodes.ListByTenant(ctx, tenant.Key.Int())
			if err != nil && !vergeos.IsNotFoundError(err) {
				return err
			}
			storage, err = client.TenantStorage.ListByTenant(ctx, tenant.Key.Int())
			if err != nil && !vergeos.IsNotFoundError(err) {
				return err
			}
			if len(nodes) == 0 && len(storage) == 0 {
				return deleteAccTenant(ctx, client, tenant.Key.Int())
			}
			if !time.Now().Before(deadline) {
				if len(nodes) != 0 {
					return fmt.Errorf("cloned tenant %q has %d nodes, want none because no_nodes is true", name, len(nodes))
				}
				return fmt.Errorf("cloned tenant %q has %d storage rows, want none because no_storage is true", name, len(storage))
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func waitTenantByName(ctx context.Context, client *vergeos.Client, name string) (*vergeos.Tenant, error) {
	deadline := time.Now().Add(2 * time.Minute)
	var last error
	for {
		tenant, err := client.Tenants.GetByName(ctx, name)
		if err == nil {
			return tenant, nil
		}
		last = err
		if !vergeos.IsNotFoundError(err) || !time.Now().Before(deadline) {
			return nil, fmt.Errorf("tenant %q: %w", name, last)
		}
		time.Sleep(2 * time.Second)
	}
}

func deleteAccTenant(ctx context.Context, client *vergeos.Client, id int) error {
	nodes, err := client.TenantNodes.ListByTenant(ctx, id)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	for _, node := range nodes {
		nodeID := node.Key.Int()
		// Kill can refuse a node that is not running. Delete is what matters.
		if err := client.TenantNodes.Kill(ctx, nodeID); err != nil && !vergeos.IsNotFoundError(err) {
			err = client.TenantNodes.Delete(ctx, nodeID)
			if err != nil && !vergeos.IsNotFoundError(err) {
				return fmt.Errorf("delete tenant node %d: %w", nodeID, err)
			}
			continue
		}
		if err := client.TenantNodes.Delete(ctx, nodeID); err != nil && !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("delete tenant node %d: %w", nodeID, err)
		}
	}
	storage, err := client.TenantStorage.ListByTenant(ctx, id)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	for _, row := range storage {
		rowID := row.Key.Int()
		if err := client.TenantStorage.Delete(ctx, rowID); err != nil && !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("delete tenant storage %d: %w", rowID, err)
		}
	}
	// A stopped clone can refuse power off. The delete below is the cleanup.
	offErr := client.Tenants.PowerOff(ctx, id)
	if offErr != nil && vergeos.IsNotFoundError(offErr) {
		return nil
	}
	if tenant, err := client.Tenants.Get(ctx, id); err == nil {
		if vnetID := tenant.VNet.Int(); vnetID > 0 {
			if err := client.Networks.Kill(ctx, vnetID); err != nil && !vergeos.IsNotFoundError(err) {
				offErr = fmt.Errorf("kill tenant %d network %d: %w", id, vnetID, err)
			}
		}
	}
	if err := client.Tenants.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		if offErr != nil {
			return fmt.Errorf("%v; delete tenant %d: %w", offErr, id, err)
		}
		return fmt.Errorf("delete tenant %d: %w", id, err)
	}
	return nil
}

func accStateID(s *terraform.State, resourceName string) (int, error) {
	rs, ok := s.RootModule().Resources[resourceName]
	if !ok {
		return 0, fmt.Errorf("resource not found: %s", resourceName)
	}
	id, err := strconv.Atoi(rs.Primary.ID)
	if err != nil {
		return 0, fmt.Errorf("%s id %q: %w", resourceName, rs.Primary.ID, err)
	}
	return id, nil
}
