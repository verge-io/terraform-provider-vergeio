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

// passthroughDeviceTypes prefers pools that are cheaper to borrow than a GPU.
// The order matches the govergeos tenant node device integration test.
var passthroughDeviceTypes = []string{
	vergeos.DeviceTypeUSB,
	vergeos.DeviceTypeSRIOVNIC,
	vergeos.DeviceTypePCI,
	vergeos.DeviceTypeVGPU,
	vergeos.DeviceTypeVGPULegacy,
	vergeos.DeviceTypeHostGPU,
	vergeos.DeviceTypeGPULegacy,
}

type hostDeviceInventory struct {
	hostGPUs    int
	vgpuDevices int
	nodeGPUs    int
}

func TestTenantNodeDevicePassthroughSelection(t *testing.T) {
	groups := []vergeos.ResourceGroup{
		{Key: "compute", Name: "compute", Type: "vm", Enabled: true},
		{Key: "", Name: "blank", Type: vergeos.DeviceTypePCI, Enabled: true},
		{Key: "off", Name: "off", Type: vergeos.DeviceTypeUSB, Enabled: false},
		{Key: "gpu", Name: "GPU", Type: vergeos.DeviceTypeHostGPU, Enabled: true},
		{Key: "usb", Name: "USB", Type: vergeos.DeviceTypeUSB, Enabled: true},
	}
	got, ok := selectPassthroughResourceGroup(groups)
	if !ok || got.Key != "usb" {
		t.Fatalf("selected %#v ok=%v", got, ok)
	}
	if _, ok := selectPassthroughResourceGroup([]vergeos.ResourceGroup{{Key: "compute", Type: "vm", Enabled: true}}); ok {
		t.Fatal("a compute group is not a passthrough device")
	}
	if _, ok := selectPassthroughResourceGroup(nil); ok {
		t.Fatal("no groups should skip")
	}

	if !hostDeviceAvailable(vergeos.DeviceTypeUSB, hostDeviceInventory{}) {
		t.Fatal("a USB resource group is the host device inventory")
	}
	if hostDeviceAvailable(vergeos.DeviceTypeHostGPU, hostDeviceInventory{}) {
		t.Fatal("a GPU group with no host device should skip")
	}
	if !hostDeviceAvailable(vergeos.DeviceTypeHostGPU, hostDeviceInventory{hostGPUs: 1}) {
		t.Fatal("a detected host GPU should run")
	}
	if !hostDeviceAvailable(vergeos.DeviceTypeVGPU, hostDeviceInventory{vgpuDevices: 1}) {
		t.Fatal("a detected vGPU should run")
	}
	if !hostDeviceAvailable(vergeos.DeviceTypeGPULegacy, hostDeviceInventory{nodeGPUs: 1}) {
		t.Fatal("an enabled node GPU should run")
	}
}

func TestTenantNodeDeviceAcceptanceConfigParses(t *testing.T) {
	src := testAccTenantNodeDeviceConfig("tf-acc-tenant", "tf-acc-node", "tf-acc-gpu", 1, "24122fab-e8fa-c4a5-dcf5-b52bf1efa474", vergeos.DeviceTypePCI, "passthrough", false)
	if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
		t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
	}
}

// TestAccTenantNodeDevice attaches one device from a passthrough resource
// group to a tenant node, updates it, and imports it. The lab this runs
// against currently has no resource groups, so the test skips until a
// resource group and a host device are both available. Names start with
// tf-acc- so the tenant sweep can remove a leftover.
func TestAccTenantNodeDevice(t *testing.T) {
	acctest.PreCheck(t)
	group, reason := accTenantNodeDeviceTarget(t)
	if reason != "" {
		t.Skip(reason)
	}
	tier := accStorageTier(t)
	tenantName := acctest.Name("tenant-device")
	nodeName := acctest.Name("tenant-node-device")
	deviceName := acctest.Name("tenant-device")
	created := testAccTenantNodeDeviceConfig(tenantName, nodeName, deviceName, tier, group.Key, group.Type, "passed through", false)
	updated := testAccTenantNodeDeviceConfig(tenantName, nodeName, deviceName+"-renamed", tier, group.Key, group.Type, "renamed", true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantNodeDeviceDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "name", deviceName),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "type", group.Type),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "resource_group", group.Key),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "device_count", "1"),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "optional", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "machine_type", "tenant"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_node_device.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_node_device.test", "machine"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_node_device.test", "tenant_node_id", "vergeio_tenant_node.test", "id"),
					testAccTenantNodeDeviceListed,
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
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_node_device.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "name", deviceName+"-renamed"),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "description", "renamed"),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "optional", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_node_device.test", "device_count", "1"),
				),
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:      "vergeio_tenant_node_device.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckTenantNodeDeviceDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeletedMatching(s, "vergeio_tenant_node_device", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.TenantNodeDevices.Get(ctx, id)
		if err != nil {
			return err
		}
		if got.MachineType != vergeos.DeviceMachineTypeTenant {
			return &vergeos.NotFoundError{Resource: "TenantNodeDevice", ID: id}
		}
		nodeID, convErr := strconv.Atoi(attrs["tenant_node_id"])
		if convErr != nil || nodeID <= 0 {
			return nil
		}
		node, err := client.TenantNodes.Get(ctx, nodeID)
		if err != nil {
			if vergeos.IsNotFoundError(err) {
				return &vergeos.NotFoundError{Resource: "TenantNodeDevice", ID: id}
			}
			return err
		}
		if node.Machine.Int() != got.Machine {
			return &vergeos.NotFoundError{Resource: "TenantNodeDevice", ID: id}
		}
		return nil
	}); err != nil {
		return err
	}
	return testAccCheckTenantDestroy(s)
}

func testAccTenantNodeDeviceListed(s *terraform.State) error {
	device := s.RootModule().Resources["vergeio_tenant_node_device.test"]
	listed := s.RootModule().Resources["data.vergeio_tenant_node_devices.test"]
	if device == nil || device.Primary.ID == "" {
		return fmt.Errorf("tenant node device missing from state")
	}
	if listed == nil {
		return fmt.Errorf("tenant node devices data source missing from state")
	}
	count, err := strconv.Atoi(listed.Primary.Attributes["devices.#"])
	if err != nil {
		return fmt.Errorf("devices.#: %w", err)
	}
	for i := 0; i < count; i++ {
		if listed.Primary.Attributes[fmt.Sprintf("devices.%d.id", i)] == device.Primary.ID {
			return nil
		}
	}
	return fmt.Errorf("device %s was not in vergeio_tenant_node_devices", device.Primary.ID)
}

func testAccTenantNodeDeviceConfig(tenantName, nodeName, deviceName string, tier int, groupKey, groupType, description string, optional bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(nodeName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(deviceName); err != nil {
		panic(err)
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

resource "vergeio_tenant_node_device" "test" {
  tenant_node_id = vergeio_tenant_node.test.id
  name           = %q
  description    = %q
  type           = %q
  resource_group = %q
  device_count   = 1
  optional       = %t
  enabled        = true
}

data "vergeio_tenant_node_devices" "test" {
  tenant_node_id = vergeio_tenant_node.test.id
  depends_on     = [vergeio_tenant_node_device.test]
}
`, tenantName, nodeName, tier, deviceName, description, groupType, groupKey, optional))
}

// accTenantNodeDeviceTarget returns the resource group to attach, or a skip
// reason when the lab has no passthrough resource group or no host device.
func accTenantNodeDeviceTarget(t *testing.T) (vergeos.ResourceGroup, string) {
	t.Helper()
	client, err := acctest.SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	groups, err := client.ResourceGroups.List(ctx)
	if err != nil {
		t.Fatalf("list resource groups: %v", err)
	}
	group, ok := selectPassthroughResourceGroup(groups)
	if !ok {
		return vergeos.ResourceGroup{}, "no resource group or host device is available"
	}
	inv := hostDeviceInventory{}
	if devices, err := client.NodeHostGPUDevices.List(ctx); err != nil {
		t.Logf("host GPU devices: %v", err)
	} else {
		inv.hostGPUs = len(devices)
	}
	if devices, err := client.NodeVGPUDevices.List(ctx); err != nil {
		t.Logf("vGPU devices: %v", err)
	} else {
		inv.vgpuDevices = len(devices)
	}
	if gpus, err := client.NodeGPUs.List(ctx); err != nil {
		t.Logf("node GPUs: %v", err)
	} else {
		inv.nodeGPUs = len(gpus)
	}
	if !hostDeviceAvailable(group.Type, inv) {
		return group, fmt.Sprintf("no host device is available for resource group %s (%s)", group.Name, group.Type)
	}
	return group, ""
}

func selectPassthroughResourceGroup(groups []vergeos.ResourceGroup) (vergeos.ResourceGroup, bool) {
	byType := map[string][]vergeos.ResourceGroup{}
	for _, group := range groups {
		if !group.Enabled || group.Key == "" || group.Type == "" {
			continue
		}
		byType[group.Type] = append(byType[group.Type], group)
	}
	for _, deviceType := range passthroughDeviceTypes {
		if len(byType[deviceType]) > 0 {
			return byType[deviceType][0], true
		}
	}
	return vergeos.ResourceGroup{}, false
}

// hostDeviceAvailable reports whether the lab has hardware for this device type.
// USB, PCI, and SR-IOV devices are published as the resource group itself.
// GPU types also need a detected host device, because a resource group can
// exist after the card is gone.
func hostDeviceAvailable(deviceType string, inv hostDeviceInventory) bool {
	switch deviceType {
	case vergeos.DeviceTypeHostGPU, vergeos.DeviceTypeGPULegacy:
		return inv.hostGPUs > 0 || inv.nodeGPUs > 0
	case vergeos.DeviceTypeVGPU, vergeos.DeviceTypeVGPULegacy:
		return inv.vgpuDevices > 0 || inv.nodeGPUs > 0
	default:
		return true
	}
}
