// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccSnapshotProfile_KeyReuseDoesNotAdoptForeign covers #291. VergeOS 26.2
// snapshot_profiles rows have no readonly id, so a different name is another row.
func TestAccSnapshotProfile_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	acctest.PreCheck(t)
	name := acctest.Name("snap-kr291")
	foreignName := acctest.Name("snap-kr291-f")
	config := testAccSnapshotProfileOnly(name, "mine")

	var managedKey int
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		ctx := context.Background()
		periods, err := client.SnapshotProfilePeriods.ListByProfile(ctx, foreignKey)
		if err == nil {
			for _, period := range periods {
				_ = client.SnapshotProfilePeriods.Delete(ctx, period.Key.Int())
			}
		}
		_ = client.SnapshotProfiles.Delete(ctx, foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSnapshotProfileKeyReuseDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_snapshot_profile.test", "id"),
					func(s *terraform.State) error {
						id, err := acctest.ResourceID(s, "vergeio_snapshot_profile.test")
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
					if err := deleteSnapshotProfile(ctx, client, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed snapshot profile %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.SnapshotProfiles.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("snapshot profile %d", managedKey))
					created, err := client.SnapshotProfiles.Create(ctx, &vergeos.SnapshotProfileCreateRequest{
						Name:        foreignName,
						Description: "belongs to someone else",
					})
					if err != nil {
						t.Fatalf("create foreign snapshot profile: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed snapshot profile key=%d; foreign key=%d name=%s reused=%v", managedKey, foreignKey, created.Name, keyReused)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_snapshot_profile.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "name", name),
					func(s *terraform.State) error {
						return acctest.ForeignUntouched(s, "vergeio_snapshot_profile.test", foreignKey, keyReused, func(ctx context.Context) (string, int, error) {
							client, err := acctest.SDKClient()
							if err != nil {
								return "", 0, err
							}
							got, err := client.SnapshotProfiles.Get(ctx, foreignKey)
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

func deleteSnapshotProfile(ctx context.Context, client *vergeos.Client, id int) error {
	periods, err := client.SnapshotProfilePeriods.ListByProfile(ctx, id)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	for _, period := range periods {
		if err := client.SnapshotProfilePeriods.Delete(ctx, period.Key.Int()); err != nil && !vergeos.IsNotFoundError(err) {
			return err
		}
	}
	return client.SnapshotProfiles.Delete(ctx, id)
}

func testAccCheckSnapshotProfileKeyReuseDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeletedMatching(s, "vergeio_snapshot_profile", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.SnapshotProfiles.Get(ctx, id)
		if err != nil {
			return err
		}
		if name := attrs["name"]; name != "" && got.Name != name {
			return &vergeos.NotFoundError{Resource: "SnapshotProfile", ID: id}
		}
		return nil
	})
}

func testAccSnapshotProfileOnly(name, description string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_snapshot_profile" "test" {
  name        = %q
  description = %q
}
`, name, description))
}
