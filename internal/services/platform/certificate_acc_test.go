// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestCertificateAcceptanceConfigParses(t *testing.T) {
	pub := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n"
	priv := "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"
	for _, src := range []string{
		testAccCertificateSelfSigned("tf-acc-cert.local", "first"),
		testAccCertificateSelfSigned("tf-acc-cert.local", "second"),
		testAccCertificateManual("tf-acc-cert.local", "uploaded", pub, priv),
		testAccCertificateManual("tf-acc-cert.local", "replaced", pub, priv),
	} {
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

func TestAccCertificate_SelfSigned(t *testing.T) {
	domain := acctest.Name("cert") + ".local"
	if err := acctest.RequirePrefix(domain); err != nil {
		t.Fatal(err)
	}
	created := testAccCertificateSelfSigned(domain, "UI certificate")
	updated := testAccCertificateSelfSigned(domain, "Replacement UI certificate")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCertificateDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_certificate.test", "domain_name", domain),
					resource.TestCheckResourceAttr("vergeio_certificate.test", "type", "self_signed"),
					resource.TestCheckResourceAttr("vergeio_certificate.test", "key_type", "ecdsa"),
					resource.TestCheckResourceAttr("vergeio_certificate.test", "description", "UI certificate"),
					resource.TestCheckResourceAttrSet("vergeio_certificate.test", "id"),
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
					resource.TestCheckResourceAttr("vergeio_certificate.test", "description", "Replacement UI certificate"),
					resource.TestCheckResourceAttr("vergeio_certificate.test", "domain_name", domain),
				),
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:            "vergeio_certificate.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"created", "modified", "expires"},
			},
		},
	})
}

func TestAccCertificate_Manual(t *testing.T) {
	domain := acctest.Name("upload") + ".local"
	if err := acctest.RequirePrefix(domain); err != nil {
		t.Fatal(err)
	}
	pub, priv := testCertificatePEM(t, domain)
	created := testAccCertificateManual(domain, "uploaded", pub, priv)
	updated := testAccCertificateManual(domain, "replaced", pub, priv)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCertificateDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_certificate.test", "domain_name", domain),
					resource.TestCheckResourceAttr("vergeio_certificate.test", "type", "manual"),
					resource.TestCheckResourceAttr("vergeio_certificate.test", "description", "uploaded"),
					resource.TestCheckResourceAttr("vergeio_certificate.test", "private_key_wo_version", "1"),
					resource.TestCheckResourceAttrSet("vergeio_certificate.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_certificate.test", "public_certificate"),
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
					resource.TestCheckResourceAttr("vergeio_certificate.test", "description", "replaced"),
				),
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:            "vergeio_certificate.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"created", "modified", "expires", "public_certificate", "chain", "private_key_wo_version"},
			},
		},
	})
}

func testAccCheckCertificateDestroy(s *terraform.State) error {
	sdk, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_certificate", func(ctx context.Context, id int) error {
		_, err := sdk.Certificates.Get(ctx, id)
		return err
	})
}

func testAccCertificateSelfSigned(domain, description string) string {
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_certificate" "test" {
  type        = "self_signed"
  domain_name = %q
  key_type    = "ecdsa"
  description = %q
}
`, domain, description))
}

func testAccCertificateManual(domain, description, pub, priv string) string {
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_certificate" "test" {
  type                   = "manual"
  domain_name            = %q
  description            = %q
  public_certificate     = <<-EOT
%sEOT
  private_key_wo         = <<-EOT
%sEOT
  private_key_wo_version = 1
}
`, domain, description, pub, priv))
}

func testCertificatePEM(t *testing.T, dnsName string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     []string{dnsName},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pub := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	priv := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	return string(pub), string(priv)
}
