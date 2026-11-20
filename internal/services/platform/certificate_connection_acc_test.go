// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

const certificateWebhookCount = 20

func TestCertificateWebhookAcceptanceConfigParses(t *testing.T) {
	src, err := testAccCertificateWithWebhooks("tf-acc-cert.example.com", "tf-acc-hook", "UI certificate", certificateWebhookCount)
	if err != nil {
		t.Fatal(err)
	}
	if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
		t.Fatalf("acceptance config did not parse: %s", diags.Error())
	}
}

func TestAccCertificate_AlongsideWebhookURLs(t *testing.T) {
	t.Setenv("TF_CLI_ARGS_apply", "-parallelism=4")

	domain := acctest.Name("cert") + ".example.com"
	hooks := acctest.Name("hook")
	if err := acctest.RequirePrefix(domain); err != nil {
		t.Fatal(err)
	}
	if err := acctest.RequirePrefix(hooks); err != nil {
		t.Fatal(err)
	}
	cfg, err := testAccCertificateWithWebhooks(domain, hooks, "UI certificate", certificateWebhookCount)
	if err != nil {
		t.Fatal(err)
	}
	check := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("vergeio_certificate.test", "domain_name", domain),
		resource.TestCheckResourceAttr("vergeio_certificate.test", "type", "self_signed"),
		resource.TestCheckResourceAttr("vergeio_webhook_url.hook_00", "name", hooks+"-00"),
		resource.TestCheckResourceAttr("vergeio_webhook_url.hook_19", "name", hooks+"-19"),
		func(s *terraform.State) error {
			n := 0
			for _, rs := range s.RootModule().Resources {
				if rs.Type == "vergeio_certificate" || rs.Type == "vergeio_webhook_url" {
					n++
				}
			}
			if n != certificateWebhookCount+1 {
				return fmt.Errorf("managed resources = %d, want %d", n, certificateWebhookCount+1)
			}
			return nil
		},
	)

	steps := make([]resource.TestStep, 0, 6)
	for range 3 {
		steps = append(steps,
			resource.TestStep{Config: cfg, Check: check},
			resource.TestStep{Config: cfg, Destroy: true},
		)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccCheckCertificateDestroy(s); err != nil {
				return err
			}
			return testAccCheckWebhookURLDestroy(s)
		},
		Steps: steps,
	})
}

func testAccCertificateWithWebhooks(domain, hookPrefix, description string, count int) (string, error) {
	var b strings.Builder
	if _, err := fmt.Fprintf(&b, `
resource "vergeio_certificate" "test" {
  type        = "self_signed"
  domain_name = %q
  domain_list = %q
  key_type    = "ecdsa"
  description = %q
}
`, domain, domain, description); err != nil {
		return "", err
	}
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("%s-%02d", hookPrefix, i)
		if _, err := fmt.Fprintf(&b, `
resource "vergeio_webhook_url" "hook_%02d" {
  name = %q
  url  = %q
}
`, i, name, "https://example.com/hook/"+name); err != nil {
			return "", err
		}
	}
	return acctest.Config(b.String()), nil
}
