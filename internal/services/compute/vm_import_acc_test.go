// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
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

var accHexKey = regexp.MustCompile(`(?i)^[0-9a-f]{40}$`)

func TestVMImportAcceptanceConfigParses(t *testing.T) {
	src := testAccVMImportConfig("tf-acc-vmimport", "https://downloads.example.com/guest.ova")
	if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	media := testAccVMImportMediaConfig("tf-acc-media", "https://downloads.example.com/golden.qcow2")
	if _, diags := hclsyntax.ParseConfig([]byte(media), "acc.tf", hcl.InitialPos); diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	adopted := testAccVMImportAdoptConfig("tf-acc-vmimport-adopt", "https://downloads.example.com/guest.ova")
	if _, diags := hclsyntax.ParseConfig([]byte(adopted), "acc.tf", hcl.InitialPos); diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	omitted := testAccVMImportOmittedConfig("tf-acc-vmimport-opt", "https://downloads.example.com/guest.ova")
	if _, diags := hclsyntax.ParseConfig([]byte(omitted), "acc.tf", hcl.InitialPos); diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	stored := testAccVMImportStoredOptionConfig("tf-acc-vmimport-opt", "https://downloads.example.com/guest.ova")
	if _, diags := hclsyntax.ParseConfig([]byte(stored), "acc.tf", hcl.InitialPos); diags.HasErrors() {
		t.Fatal(diags.Error())
	}
}

func TestAccVMImport_OVA(t *testing.T) {
	rawURL := os.Getenv("TF_ACC_VERGEIO_OVA_URL")
	name := acctest.Name("vmimport")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if rawURL == "" {
				t.Skip("set TF_ACC_VERGEIO_OVA_URL to an http or https OVA or OVF to run the live VM import test")
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMImportDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMImportConfig(name, rawURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "target", "vm"),
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "status", "complete"),
					resource.TestCheckResourceAttrSet("vergeio_vm_import.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_vm_import.test", "vm_id"),
					resource.TestCheckResourceAttrPair("data.vergeio_vm_import.test", "vm_id", "vergeio_vm_import.test", "vm_id"),
				),
			},
			{
				ResourceName:            "vergeio_vm_import.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func TestAccVMImport_AdoptDestroy(t *testing.T) {
	rawURL := os.Getenv("TF_ACC_VERGEIO_OVA_URL")
	name := acctest.Name("vmimport-adopt")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	var importID, vmID string
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if rawURL == "" {
				t.Skip("set TF_ACC_VERGEIO_OVA_URL to an http or https OVA or OVF to run the live VM import test")
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMImportDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMImportConfig(name, rawURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "adopted", "false"),
					resource.TestCheckResourceAttrSet("vergeio_vm_import.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_vm_import.test", "vm_id"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_vm_import.test"]
						if !ok {
							return fmt.Errorf("vergeio_vm_import.test missing")
						}
						importID = rs.Primary.ID
						vmID = rs.Primary.Attributes["vm_id"]
						return nil
					},
				),
			},
			{
				Config: testAccVMImportAdoptConfig(name, rawURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_import.adopted", "adopted", "true"),
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "adopted", "false"),
					resource.TestCheckResourceAttrPair("vergeio_vm_import.adopted", "id", "vergeio_vm_import.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_vm_import.adopted", "vm_id", "vergeio_vm_import.test", "vm_id"),
				),
			},
			{
				Config: testAccVMImportConfig(name, rawURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "adopted", "false"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_vm_import.test"]
						if !ok {
							return fmt.Errorf("vergeio_vm_import.test missing after adopted destroy")
						}
						if rs.Primary.ID != importID || rs.Primary.Attributes["vm_id"] != vmID {
							return fmt.Errorf("owner changed from import %s vm %s to import %s vm %s", importID, vmID, rs.Primary.ID, rs.Primary.Attributes["vm_id"])
						}
						return testAccVMImportStillPresent(rs.Primary.ID, vmID)
					},
				),
			},
		},
	})
}

func TestAccVMImport_StoredOption(t *testing.T) {
	rawURL := os.Getenv("TF_ACC_VERGEIO_OVA_URL")
	name := acctest.Name("vmimport-opt")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	var preserveMacs, tier string
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if rawURL == "" {
				t.Skip("set TF_ACC_VERGEIO_OVA_URL to an http or https OVA or OVF to run the live VM import test")
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMImportDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMImportOmittedConfig(name, rawURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_vm_import.test", "preserve_macs"),
					resource.TestCheckResourceAttrSet("vergeio_vm_import.test", "preferred_tier"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_vm_import.test"]
						if !ok {
							return fmt.Errorf("vergeio_vm_import.test missing")
						}
						preserveMacs = rs.Primary.Attributes["preserve_macs"]
						tier = rs.Primary.Attributes["preferred_tier"]
						switch preserveMacs {
						case "true", "false":
						default:
							return fmt.Errorf("preserve_macs = %q", preserveMacs)
						}
						switch tier {
						case "1", "2", "3", "4", "5":
						default:
							return fmt.Errorf("preferred_tier = %q", tier)
						}
						return nil
					},
				),
			},
			{
				ResourceName:            "vergeio_vm_import.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				PreConfig: func() {
					if preserveMacs == "" || tier == "" {
						t.Fatal("create did not store preserve_macs and preferred_tier")
					}
					t.Setenv("TF_VAR_preserve_macs", preserveMacs)
					t.Setenv("TF_VAR_preferred_tier", tier)
				},
				Config: testAccVMImportStoredOptionConfig(name, rawURL),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccVMImport_Disk(t *testing.T) {
	rawURL := os.Getenv("TF_ACC_VERGEIO_DISK_URL")
	name := acctest.Name("media")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if rawURL == "" {
				t.Skip("set TF_ACC_VERGEIO_DISK_URL to an http or https disk image to run the live media import test")
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMImportDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMImportMediaConfig(name, rawURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "target", "media"),
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "status", "complete"),
					resource.TestCheckResourceAttrSet("vergeio_vm_import.test", "file_id"),
					resource.TestCheckResourceAttr("vergeio_vm_import.test", "owns_file", "true"),
				),
			},
			{
				ResourceName:            "vergeio_vm_import.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func testAccCheckVMImportDestroy(s *terraform.State) error {
	sdk, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_vm_import" || rs.Primary.ID == "" {
			continue
		}
		if accHexKey.MatchString(rs.Primary.ID) {
			_, err := sdk.VMImports.Get(ctx, rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("vm import %s still exists after destroy", rs.Primary.ID)
			}
			if !vergeos.IsNotFoundError(err) {
				return fmt.Errorf("checking vm import %s was destroyed: %w", rs.Primary.ID, err)
			}
			if rs.Primary.Attributes["adopted"] == "true" {
				continue
			}
			if vmID, convErr := strconv.Atoi(rs.Primary.Attributes["vm_id"]); convErr == nil && vmID > 0 && rs.Primary.Attributes["delete_vm"] != "false" {
				_, err = sdk.VMs.Get(ctx, vmID)
				if err == nil {
					return fmt.Errorf("virtual machine %d created by vm import %s still exists after destroy", vmID, rs.Primary.ID)
				}
				if !vergeos.IsNotFoundError(err) {
					return fmt.Errorf("checking virtual machine %d was destroyed: %w", vmID, err)
				}
			}
			if rs.Primary.Attributes["owns_file"] == "true" {
				if fileID, convErr := strconv.Atoi(rs.Primary.Attributes["file_id"]); convErr == nil && fileID > 0 {
					_, err = sdk.Files.Get(ctx, fileID)
					if err == nil {
						return fmt.Errorf("catalog file %d downloaded by vm import %s still exists after destroy", fileID, rs.Primary.ID)
					}
					if !vergeos.IsNotFoundError(err) {
						return fmt.Errorf("checking catalog file %d was destroyed: %w", fileID, err)
					}
				}
			}
			continue
		}
		fileID, convErr := strconv.Atoi(rs.Primary.ID)
		if convErr != nil || fileID <= 0 {
			return fmt.Errorf("vm import id %q", rs.Primary.ID)
		}
		_, err = sdk.Files.Get(ctx, fileID)
		if err == nil {
			return fmt.Errorf("media file %d still exists after destroy", fileID)
		}
		if !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("checking media file %d was destroyed: %w", fileID, err)
		}
	}
	return nil
}

func testAccVMImportConfig(name, rawURL string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm_import" "test" {
  name           = %q
  url            = %q
  preserve_macs  = false
  preferred_tier = "3"

  timeouts {
    create = "20m"
  }
}

data "vergeio_vm_import" "test" {
  id = vergeio_vm_import.test.id
}
`, name, rawURL))
}

func testAccVMImportStillPresent(importID, vmID string) error {
	sdk, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if _, err := sdk.VMImports.Get(ctx, importID); err != nil {
		return fmt.Errorf("vm import %s after adopted destroy: %w", importID, err)
	}
	id, convErr := strconv.Atoi(vmID)
	if convErr != nil || id <= 0 {
		return fmt.Errorf("vm id %q", vmID)
	}
	if _, err := sdk.VMs.Get(ctx, id); err != nil {
		return fmt.Errorf("virtual machine %s after adopted destroy: %w", vmID, err)
	}
	return nil
}

func testAccVMImportAdoptConfig(name, rawURL string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return testAccVMImportConfig(name, rawURL) + `
resource "vergeio_vm_import" "adopted" {
  name = vergeio_vm_import.test.name
  file = vergeio_vm_import.test.file_id

  timeouts {
    create = "20m"
  }
}
`
}

func testAccVMImportOmittedConfig(name, rawURL string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm_import" "test" {
  name = %q
  url  = %q

  timeouts {
    create = "20m"
  }
}
`, name, rawURL))
}

func testAccVMImportStoredOptionConfig(name, rawURL string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
variable "preserve_macs" {
  type = bool
}

variable "preferred_tier" {
  type = string
}

resource "vergeio_vm_import" "test" {
  name           = %q
  url            = %q
  preserve_macs  = var.preserve_macs
  preferred_tier = var.preferred_tier

  timeouts {
    create = "20m"
  }
}
`, name, rawURL))
}

func testAccVMImportMediaConfig(name, rawURL string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm_import" "test" {
  name   = %q
  target = "media"
  url    = %q

  timeouts {
    create = "20m"
  }
}
`, name, rawURL))
}
