// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform_test

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

// TestAccCertificate_KeyReuseDoesNotAdoptForeign covers #326. Ownership is
// the readonly created time, so a description change on the same row stays
// an update. domain_name still forces replacement when it changes on the
// same certificate.
func TestAccCertificate_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	acctest.PreCheck(t)
	domain := acctest.Name("cert-kr326") + ".example.com"
	foreignDomain := acctest.Name("cert-kr326-f") + ".example.com"
	if err := acctest.RequirePrefix(domain); err != nil {
		t.Fatal(err)
	}
	if err := acctest.RequirePrefix(foreignDomain); err != nil {
		t.Fatal(err)
	}
	config := testAccCertificateSelfSigned(domain, "mine")

	var managedKey int
	var managedCreated string
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		_ = client.Certificates.Delete(context.Background(), foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCertificateKeyReuseDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_certificate.test", "domain_name", domain),
					resource.TestCheckResourceAttrSet("vergeio_certificate.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_certificate.test", "created"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["vergeio_certificate.test"]
						id, err := acctest.ResourceID(s, "vergeio_certificate.test")
						if err != nil {
							return err
						}
						managedKey = id
						managedCreated = rs.Primary.Attributes["created"]
						if managedCreated == "" || managedCreated == "0" {
							return fmt.Errorf("created is %q; the ownership check cannot tell rows apart", managedCreated)
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
					if err := client.Certificates.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed certificate %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.Certificates.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("certificate %d", managedKey))
					created, err := client.Certificates.Create(ctx, &vergeos.CertificateCreateRequest{
						Type:        vergeos.CertificateTypeSelfSigned,
						DomainList:  foreignDomain,
						Description: "belongs to someone else",
						KeyType:     vergeos.CertificateKeyTypeECDSA,
					})
					if err != nil {
						t.Fatalf("create foreign certificate: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed certificate key=%d created=%s; foreign key=%d created=%d reused=%v",
						managedKey, managedCreated, foreignKey, created.Created, keyReused)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_certificate.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_certificate.test", "domain_name", domain),
					resource.TestCheckResourceAttrSet("vergeio_certificate.test", "created"),
					func(s *terraform.State) error {
						if err := acctest.ForeignUntouched(s, "vergeio_certificate.test", foreignKey, keyReused, func(ctx context.Context) (string, int, error) {
							client, err := acctest.SDKClient()
							if err != nil {
								return "", 0, err
							}
							got, err := client.Certificates.Get(ctx, foreignKey)
							if err != nil {
								return "", 0, err
							}
							return got.Description, got.Key.Int(), nil
						}, "belongs to someone else"); err != nil {
							return err
						}
						rs := s.RootModule().Resources["vergeio_certificate.test"]
						if rs.Primary.Attributes["created"] == "" || rs.Primary.Attributes["created"] == managedCreated {
							return fmt.Errorf("managed certificate kept created %s after outside delete", managedCreated)
						}
						return nil
					},
				),
			},
		},
	})
}

func testAccCheckCertificateKeyReuseDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeletedMatching(s, "vergeio_certificate", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.Certificates.Get(ctx, id)
		if err != nil {
			return err
		}
		if created := attrs["created"]; created != "" && created != "0" && strconv.FormatInt(got.Created, 10) != created {
			return &vergeos.NotFoundError{Resource: "Certificate", ID: id}
		}
		return nil
	})
}
