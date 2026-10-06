// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestSiteAcceptanceConfigParses(t *testing.T) {
	for _, src := range []string{
		testAccSiteConfig("tf-acc-site", "tf-acc-profile", "tf-acc-in", "Remote lab", "Austin", 1, false),
		testAccSiteConfig("tf-acc-site", "tf-acc-profile", "tf-acc-in", "Paired lab", "Dallas", 3, false),
		testAccSiteConfig("tf-acc-site", "tf-acc-profile", "tf-acc-in", "Paired lab", "Dallas", 3, true),
		testAccOutgoingConfig("tf-acc-site", "tf-acc-profile", "tf-acc-out", "https://192.0.2.10", "example-code", "first", 604800),
		testAccOutgoingConfig("tf-acc-site", "tf-acc-profile", "tf-acc-out", "https://192.0.2.10", "example-code", "second", 1209600),
	} {
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

// TestAccSite_SyncAndStatus runs on one VergeOS system. The site URL is not
// a second system, and the test does not create an outgoing sync. Outgoing
// sync needs a real remote and registration code. See TestAccSite_OutgoingSync.
func TestAccSite_SyncAndStatus(t *testing.T) {
	siteName := acctest.Name("site")
	profileName := acctest.Name("cloud")
	incomingName := acctest.Name("in")
	for _, name := range []string{siteName, profileName, incomingName} {
		if err := acctest.RequirePrefix(name); err != nil {
			t.Fatal(err)
		}
	}
	created := testAccSiteConfig(siteName, profileName, incomingName, "Remote lab", "Austin", 1, false)
	updated := testAccSiteConfig(siteName, profileName, incomingName, "Paired lab", "Dallas", 3, false)
	behind := testAccSiteConfig(siteName, profileName, incomingName, "Paired lab", "Dallas", 3, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_site.test", "name", siteName),
					resource.TestCheckResourceAttr("vergeio_site.test", "url", "https://203.0.113.10"),
					resource.TestCheckResourceAttr("vergeio_site.test", "enabled", "false"),
					resource.TestCheckResourceAttr("vergeio_site.test", "description", "Remote lab"),
					resource.TestCheckResourceAttr("vergeio_site.test", "city", "Austin"),
					resource.TestCheckResourceAttr("vergeio_site.test", "config_cloud_snapshots", "disabled"),
					resource.TestCheckResourceAttrSet("vergeio_site.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_site.test", "site_id"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "name", profileName),
					resource.TestCheckResourceAttrSet("vergeio_snapshot_profile.test", "period.0.key"),
					resource.TestCheckResourceAttr("vergeio_site_sync_incoming.test", "name", incomingName),
					resource.TestCheckResourceAttr("vergeio_site_sync_incoming.test", "min_snapshots", "1"),
					resource.TestCheckResourceAttrSet("vergeio_site_sync_incoming.test", "site_id"),
					resource.TestCheckResourceAttrPair("vergeio_site_sync_incoming.test", "site_id", "vergeio_site.test", "id"),
					resource.TestCheckResourceAttr("data.vergeio_site_sync_incoming_status.test", "name", incomingName),
					resource.TestCheckResourceAttrPair("data.vergeio_site_sync_incoming_status.test", "id", "vergeio_site_sync_incoming.test", "id"),
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
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_site.test", "description", "Paired lab"),
					resource.TestCheckResourceAttr("vergeio_site.test", "city", "Dallas"),
					resource.TestCheckResourceAttr("vergeio_site_sync_incoming.test", "min_snapshots", "3"),
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
				ResourceName:            "vergeio_site.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"status", "status_info", "authentication_status", "last_stat_update", "created", "modified", "remote_user", "vsan_host", "vsan_port", "is_tenant", "incoming_syncs_enabled", "outgoing_syncs_enabled", "repairs_outgoing_enabled", "incoming_stats_enabled", "outgoing_stats_enabled", "outgoing_management_enabled", "incoming_management_enabled"},
			},
			{
				ResourceName:            "vergeio_site_sync_incoming.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"status", "status_info", "state", "last_sync", "registration_code", "system_created"},
			},
			{
				ResourceName:      "vergeio_snapshot_profile.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config:      behind,
				ExpectError: regexp.MustCompile(`incoming sync .+ (has not run|last ran [0-9]+ seconds ago, past max_lag_seconds 1)`),
			},
		},
	})
}

// TestAccSite_OutgoingSync creates an outgoing sync. VergeOS requires a
// registration code from a real remote incoming sync, so the test skips
// unless TF_ACC_VERGEIO_REMOTE_URL and TF_ACC_VERGEIO_REGISTRATION_CODE are set.
func TestAccSite_OutgoingSync(t *testing.T) {
	siteName := acctest.Name("site")
	profileName := acctest.Name("cloud")
	outgoingName := acctest.Name("out")
	for _, name := range []string{siteName, profileName, outgoingName} {
		if err := acctest.RequirePrefix(name); err != nil {
			t.Fatal(err)
		}
	}
	remoteURL := strings.TrimSpace(os.Getenv("TF_ACC_VERGEIO_REMOTE_URL"))
	code := strings.TrimSpace(os.Getenv("TF_ACC_VERGEIO_REGISTRATION_CODE"))
	created := testAccOutgoingConfig(siteName, profileName, outgoingName, remoteURL, code, "first", 604800)
	updated := testAccOutgoingConfig(siteName, profileName, outgoingName, remoteURL, code, "second", 1209600)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if strings.TrimSpace(os.Getenv("TF_ACC_VERGEIO_REMOTE_URL")) == "" || strings.TrimSpace(os.Getenv("TF_ACC_VERGEIO_REGISTRATION_CODE")) == "" {
				t.Skip("set TF_ACC_VERGEIO_REMOTE_URL and TF_ACC_VERGEIO_REGISTRATION_CODE to create an outgoing site sync")
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_site.test", "url", remoteURL),
					resource.TestCheckResourceAttr("vergeio_site_sync_outgoing.test", "name", outgoingName),
					resource.TestCheckResourceAttr("vergeio_site_sync_outgoing.test", "note", "first"),
					resource.TestCheckResourceAttr("vergeio_site_sync_outgoing.test", "registration_code_wo_version", "1"),
					resource.TestCheckResourceAttr("vergeio_site_sync_outgoing.test", "period.0.retention", "604800"),
					resource.TestCheckResourceAttrSet("vergeio_site_sync_outgoing.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_site_sync_outgoing.test", "site_id"),
					resource.TestCheckResourceAttrSet("vergeio_site_sync_outgoing.test", "period.0.key"),
					resource.TestCheckResourceAttrPair("vergeio_site_sync_outgoing.test", "site_id", "vergeio_site.test", "id"),
					resource.TestCheckResourceAttrPair("data.vergeio_site_sync_outgoing_status.test", "id", "vergeio_site_sync_outgoing.test", "id"),
					resource.TestCheckResourceAttr("data.vergeio_site_sync_outgoing_status.test", "name", outgoingName),
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
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_site_sync_outgoing.test", "note", "second"),
					resource.TestCheckResourceAttr("vergeio_site_sync_outgoing.test", "period.0.retention", "1209600"),
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
				ResourceName:            "vergeio_site_sync_outgoing.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"registration_code_wo_version", "status", "status_info", "state", "last_run", "user", "remote_site_id", "remote_vsan_host", "remote_vsan_port", "remote_sync_id", "remote_min_snapshots", "remote_snaps_status", "remote_snaps_status_info", "remote_snaps_last_refresh"},
			},
		},
	})
}

func testAccCheckSiteDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	checks := []struct {
		resourceType string
		get          func(context.Context, int) error
	}{
		{"vergeio_site", func(ctx context.Context, id int) error {
			_, err := client.Sites.Get(ctx, id)
			return err
		}},
		{"vergeio_site_sync_incoming", func(ctx context.Context, id int) error {
			_, err := client.SiteSyncsIncoming.Get(ctx, id)
			return err
		}},
		{"vergeio_site_sync_outgoing", func(ctx context.Context, id int) error {
			_, err := client.SiteSyncsOutgoing.Get(ctx, id)
			return err
		}},
		{"vergeio_snapshot_profile", func(ctx context.Context, id int) error {
			_, err := client.SnapshotProfiles.Get(ctx, id)
			return err
		}},
	}
	for _, check := range checks {
		if err := acctest.CheckDeleted(s, check.resourceType, check.get); err != nil {
			return err
		}
	}
	return nil
}

func testAccSiteConfig(siteName, profileName, incomingName, description, city string, minSnapshots int, lag bool) string {
	status := ""
	if lag {
		status = `
data "vergeio_site_sync_incoming_status" "behind" {
  id              = vergeio_site_sync_incoming.test.id
  max_lag_seconds = 1
}
`
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_snapshot_profile" "test" {
  name        = %q
  description = "Cloud snapshots"

  period {
    name      = "nightly"
    frequency = "daily"
    hour      = 1
    minute    = 0
    retention = 604800
    immutable = true
  }
}

resource "vergeio_site" "test" {
  name                   = %q
  url                    = "https://203.0.113.10"
  enabled                = false
  description            = %q
  city                   = %q
  config_cloud_snapshots = "disabled"
}

resource "vergeio_site_sync_incoming" "test" {
  site_id       = vergeio_site.test.id
  name          = %q
  min_snapshots = %d
  enabled       = true
}

data "vergeio_site_sync_incoming_status" "test" {
  site_id = vergeio_site.test.id
  name    = vergeio_site_sync_incoming.test.name
}
%s
`, profileName, siteName, description, city, incomingName, minSnapshots, status))
}

func testAccOutgoingConfig(siteName, profileName, outgoingName, remoteURL, code, note string, retention int) string {
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_snapshot_profile" "test" {
  name        = %q
  description = "Cloud snapshots"

  period {
    name      = "nightly"
    frequency = "daily"
    hour      = 1
    minute    = 0
    retention = 604800
    immutable = true
  }
}

resource "vergeio_site" "test" {
  name                   = %q
  url                    = %q
  enabled                = false
  config_cloud_snapshots = "disabled"
}

resource "vergeio_site_sync_outgoing" "test" {
  site_id     = vergeio_site.test.id
  name        = %q
  description = "Nightly copies"
  enabled     = false
  note        = %q

  registration_code_wo         = %q
  registration_code_wo_version = 1

  period {
    profile_period = vergeio_snapshot_profile.test.period[0].key
    retention      = %d
    priority       = 5
    do_not_expire  = false
  }
}

data "vergeio_site_sync_outgoing_status" "test" {
  id = vergeio_site_sync_outgoing.test.id
}
`, profileName, siteName, remoteURL, outgoingName, note, code, retention))
}
