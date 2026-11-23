// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

func TestWebhookAcceptanceConfigParses(t *testing.T) {
	for _, src := range []string{
		testAccWebhookConfig("tf-acc-hook", "https://example.com/hook", 10, `{"text":"one"}`),
		testAccWebhookConfig("tf-acc-hook", "https://example.com/hook/v2", 30, `{"text":"one"}`),
		testAccWebhookConfig("tf-acc-hook", "https://example.com/hook/v2", 30, `{"text":"two"}`),
		testAccWebhookURLRetriesZero("tf-acc-hook"),
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

// TestAccWebhookURL_KeyReuseDoesNotAdoptForeign covers #291. VergeOS 26.2
// webhook_urls rows have no readonly id, so a different name is another row.
func TestAccWebhookURL_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	acctest.PreCheck(t)
	name := acctest.Name("hook-kr291")
	foreignName := acctest.Name("hook-kr291-f")
	config := testAccWebhookURLOnly(name, "https://example.com/tf-acc-mine")

	var managedKey int
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		_ = client.WebhookURLs.Delete(context.Background(), foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookURLDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_webhook_url.test", "id"),
					func(s *terraform.State) error {
						id, err := acctest.ResourceID(s, "vergeio_webhook_url.test")
						if err != nil {
							return err
						}
						managedKey = id
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
					if err := client.WebhookURLs.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed webhook url %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.WebhookURLs.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("webhook url %d", managedKey))
					created, err := client.WebhookURLs.Create(ctx, &vergeos.WebhookURLCreateRequest{
						Name: foreignName,
						URL:  "https://example.com/tf-acc-other",
					})
					if err != nil {
						t.Fatalf("create foreign webhook url: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed webhook url key=%d; foreign key=%d name=%s reused=%v", managedKey, foreignKey, created.Name, keyReused)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_webhook_url.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "name", name),
					func(s *terraform.State) error {
						return acctest.ForeignUntouched(s, "vergeio_webhook_url.test", foreignKey, keyReused, func(ctx context.Context) (string, int, error) {
							client, err := acctest.SDKClient()
							if err != nil {
								return "", 0, err
							}
							got, err := client.WebhookURLs.Get(ctx, foreignKey)
							if err != nil {
								return "", 0, err
							}
							return got.Name, got.Key.Int(), nil
						}, foreignName)
					},
				),
			},
		},
	})
}

func testAccCheckWebhookURLDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeletedMatching(s, "vergeio_webhook_url", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.WebhookURLs.Get(ctx, id)
		if err != nil {
			return err
		}
		if name := attrs["name"]; name != "" && got.Name != name {
			return &vergeos.NotFoundError{Resource: "WebhookURL", ID: id}
		}
		return nil
	})
}

func TestAccWebhookURL_RetriesZeroImport(t *testing.T) {
	name := acctest.Name("hook-retries0")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	config := testAccWebhookURLRetriesZero(name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookURLDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "url", "https://example.com/tf-acc-retries0"),
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "retries", "0"),
					resource.TestCheckResourceAttrSet("vergeio_webhook_url.test", "id"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "vergeio_webhook_url.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccWebhookURLRetriesZero(name string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_webhook_url" "test" {
  name    = %q
  url     = "https://example.com/tf-acc-retries0"
  retries = 0
}
`, name))
}

// TestAccWebhookURL_KeyReuseDestroyRefreshOff covers #322. Destroy with
// refresh off must leave the destination that now holds the stored key.
// webhook_urls has no created time, so the unique name is the identity.
func TestAccWebhookURL_KeyReuseDestroyRefreshOff(t *testing.T) {
	acctest.PreCheck(t)
	name := acctest.Name("hook-kr322")
	foreignName := acctest.Name("hook-kr322-f")
	config := testAccWebhookURLOnly(name, "https://example.com/tf-acc-mine")

	var managedKey int
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		_ = client.WebhookURLs.Delete(context.Background(), foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		AdditionalCLIOptions:     acctest.PlanWithoutRefresh(),
		CheckDestroy:             testAccCheckWebhookURLDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_webhook_url.test", "id"),
					func(s *terraform.State) error {
						id, err := acctest.ResourceID(s, "vergeio_webhook_url.test")
						if err != nil {
							return err
						}
						managedKey = id
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
					if err := client.WebhookURLs.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed webhook url %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.WebhookURLs.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("webhook url %d", managedKey))
					created, err := client.WebhookURLs.Create(ctx, &vergeos.WebhookURLCreateRequest{
						Name: foreignName,
						URL:  "https://example.com/tf-acc-other",
					})
					if err != nil {
						t.Fatalf("create foreign webhook url: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed webhook url key=%d; foreign key=%d name=%s reused=%v", managedKey, foreignKey, created.Name, keyReused)
				},
				Config: acctest.Config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_webhook_url.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: func(*terraform.State) error {
					if err := acctest.ForeignPresent(foreignKey, foreignName, keyReused, func(ctx context.Context) (string, error) {
						client, err := acctest.SDKClient()
						if err != nil {
							return "", err
						}
						got, err := client.WebhookURLs.Get(ctx, foreignKey)
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
					if err := client.WebhookURLs.Delete(context.Background(), foreignKey); err != nil && !vergeos.IsNotFoundError(err) {
						return fmt.Errorf("delete foreign webhook url %d: %w", foreignKey, err)
					}
					foreignKey = 0
					return nil
				},
			},
		},
	})
}

func TestAccWebhookURL_AdoptMatchingRow(t *testing.T) {
	name := acctest.Name("hook-adopt")
	rawURL := "https://example.com/tf-acc-hook-adopt"
	var key int
	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || key <= 0 {
			return
		}
		_ = client.WebhookURLs.Delete(context.Background(), key)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookURLDestroy,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					created, err := client.WebhookURLs.Create(context.Background(), &vergeos.WebhookURLCreateRequest{
						Name: name,
						URL:  rawURL,
					})
					if err != nil {
						t.Fatalf("create webhook destination: %v", err)
					}
					key = created.Key.Int()
				},
				Config: testAccWebhookURLOnly(name, rawURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_webhook_url.test", "url", rawURL),
					func(s *terraform.State) error {
						id, err := acctest.ResourceID(s, "vergeio_webhook_url.test")
						if err != nil {
							return err
						}
						if id != key {
							return fmt.Errorf("id = %d, want existing destination %d", id, key)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccWebhookURL_RejectDifferentURL(t *testing.T) {
	name := acctest.Name("hook-adopt-diff")
	var key int
	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || key <= 0 {
			return
		}
		_ = client.WebhookURLs.Delete(context.Background(), key)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					created, err := client.WebhookURLs.Create(context.Background(), &vergeos.WebhookURLCreateRequest{
						Name: name,
						URL:  "https://example.com/tf-acc-other",
					})
					if err != nil {
						t.Fatalf("create webhook destination: %v", err)
					}
					key = created.Key.Int()
				},
				Config:      testAccWebhookURLOnly(name, "https://example.com/tf-acc-hook-adopt"),
				ExpectError: regexp.MustCompile(`terraform import vergeio_webhook_url`),
			},
		},
	})
}

func testAccWebhookURLOnly(name, rawURL string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_webhook_url" "test" {
  name = %q
  url  = %q
}
`, name, rawURL))
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
