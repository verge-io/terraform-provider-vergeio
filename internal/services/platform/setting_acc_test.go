// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
	"terraform-provider-vergeio/internal/client"
)

func TestSettingAcceptanceConfigParses(t *testing.T) {
	for _, src := range []string{
		testAccSettingConfig("501"),
		testAccSettingConfig("502"),
	} {
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

func TestAccSetting_MaxConnections(t *testing.T) {
	var original, cloudName string
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			original, cloudName = readSettingFixture(t)
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSettingRestored(&original, &cloudName),
		Steps: []resource.TestStep{
			{
				Config: testAccSettingConfig("501"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_setting.test", "key", "max_connections"),
					resource.TestCheckResourceAttr("vergeio_setting.test", "id", "max_connections"),
					resource.TestCheckResourceAttr("vergeio_setting.test", "value", "501"),
					resource.TestCheckResourceAttrSet("vergeio_setting.test", "default_value"),
				),
			},
			{
				Config: testAccSettingConfig("501"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: testAccSettingConfig("502"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_setting.test", "value", "502"),
					resource.TestCheckResourceAttr("vergeio_setting.test", "key", "max_connections"),
				),
			},
			{
				Config: testAccSettingConfig("502"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "vergeio_setting.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func readSettingFixture(t *testing.T) (string, string) {
	t.Helper()
	sdk, err := acctest.SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	setting, err := sdk.Settings.GetByKey(ctx, "max_connections")
	if err != nil {
		t.Fatal(err)
	}
	cloud, err := sdk.Settings.GetCloudName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return setting.Value, cloud
}

func testAccCheckSettingRestored(original, cloudName *string) func(*terraform.State) error {
	return func(*terraform.State) error {
		if original == nil || *original == "" || cloudName == nil || *cloudName == "" {
			return nil
		}
		ctx := context.Background()
		sdk, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		var problems []error
		got, err := sdk.Settings.GetByKey(ctx, "max_connections")
		if err != nil {
			problems = append(problems, err)
		} else if got.Value != got.DefaultValue {
			problems = append(problems, fmt.Errorf("max_connections is %q after destroy, default is %q", got.Value, got.DefaultValue))
		}
		cloud, err := sdk.Settings.GetCloudName(ctx)
		if err != nil {
			problems = append(problems, err)
		} else if cloud != *cloudName {
			problems = append(problems, fmt.Errorf("cloud_name changed from %q to %q", *cloudName, cloud))
		}
		if restoreErr := putSettingValue(ctx, "max_connections", *original); restoreErr != nil {
			problems = append(problems, restoreErr)
		}
		return errors.Join(problems...)
	}
}

func putSettingValue(ctx context.Context, key, value string) error {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	c := vergeio.NewClient(host, username, password, true)
	buf, err := json.Marshal(struct {
		Value string `json:"value"`
	}{Value: value})
	if err != nil {
		return err
	}
	resp, err := c.Put(ctx, "api/v4/settings/"+url.PathEscape(key), bytes.NewBuffer(buf))
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return err
}

func testAccSettingConfig(value string) string {
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_setting" "test" {
  key   = "max_connections"
  value = %q
}
`, value))
}
