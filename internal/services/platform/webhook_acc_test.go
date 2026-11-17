// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestWebhookAcceptanceConfigParses(t *testing.T) {
	for _, src := range []string{
		testAccWebhookConfig("tf-acc-hook", "https://example.com/hook", 10, `{"text":"one"}`),
		testAccWebhookConfig("tf-acc-hook", "https://example.com/hook/v2", 30, `{"text":"one"}`),
		testAccWebhookConfig("tf-acc-hook", "https://example.com/hook/v2", 30, `{"text":"two"}`),
	} {
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

func TestAccWebhook_URLAndDelivery(t *testing.T) {
	name := acctest.Name("hook")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	created := testAccWebhookConfig(name, "https://example.com/hook", 10, `{"text":"one"}`)
	updated := testAccWebhookConfig(name, "https://example.com/hook/v2", 30, `{"text":"one"}`)
	replaced := testAccWebhookConfig(name, "https://example.com/hook/v2", 30, `{"text":"two"}`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "url", "https://example.com/hook"),
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "timeout", "10"),
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "authorization_type", "bearer"),
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "authorization_value_wo_version", "1"),
					resource.TestCheckResourceAttrSet("vergeio_webhook_url.test", "id"),
					resource.TestCheckResourceAttr("vergeio_webhook.test", "message", `{"text":"one"}`),
					resource.TestCheckResourceAttrSet("vergeio_webhook.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_webhook.test", "webhook_url_id", "vergeio_webhook_url.test", "id"),
				),
			},
			{
				Config: created,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "url", "https://example.com/hook/v2"),
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "timeout", "30"),
					resource.TestCheckResourceAttr("vergeio_webhook.test", "message", `{"text":"one"}`),
				),
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:            "vergeio_webhook_url.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"authorization_value_wo_version"},
			},
			{
				ResourceName:            "vergeio_webhook.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"status", "status_info", "last_attempt", "created"},
			},
			{
				Config: replaced,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_webhook.test", "message", `{"text":"two"}`),
				),
			},
			{
				Config: replaced,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccCheckWebhookDestroy(s *terraform.State) error {
	sdk, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_webhook", func(ctx context.Context, id int) error {
		_, err := sdk.Webhooks.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_webhook_url", func(ctx context.Context, id int) error {
		_, err := sdk.WebhookURLs.Get(ctx, id)
		return err
	})
}

func testAccWebhookConfig(name, rawURL string, timeout int, message string) string {
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_webhook_url" "test" {
  name                           = %q
  url                            = %q
  timeout                        = %d
  authorization_type             = "bearer"
  authorization_value_wo         = "example-token"
  authorization_value_wo_version = 1
}

resource "vergeio_webhook" "test" {
  webhook_url_id = vergeio_webhook_url.test.id
  message        = %q
}
`, name, rawURL, timeout, message))
}
