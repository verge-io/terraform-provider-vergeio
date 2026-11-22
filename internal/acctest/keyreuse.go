package acctest

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"
)

// ResourceID is the numeric id stored for address.
func ResourceID(s *terraform.State, address string) (int, error) {
	rs, ok := s.RootModule().Resources[address]
	if !ok {
		return 0, fmt.Errorf("missing %s", address)
	}
	id, err := strconv.Atoi(rs.Primary.ID)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s id %q: %v", address, rs.Primary.ID, err)
	}
	return id, nil
}

// WaitUntilMissing polls get until VergeOS reports the row gone, then waits
// for the key reuse window the alias and tenant acceptance tests use.
func WaitUntilMissing(t *testing.T, get func() error, label string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		err := get()
		if vergeos.IsNotFoundError(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still present after delete: %v", label, err)
		}
		time.Sleep(time.Second)
	}
	t.Logf("%s gone; waiting 15s for key reuse window", label)
	time.Sleep(15 * time.Second)
}

// ForeignUntouched fails when apply renamed the outside row or stored its key.
func ForeignUntouched(s *terraform.State, address string, foreignKey int, keyReused bool, lookup func(context.Context) (string, int, error), foreignName string) error {
	ctx := context.Background()
	gotName, gotKey, err := lookup(ctx)
	if err != nil {
		return fmt.Errorf("foreign %s %d missing after apply (key reused=%v): %w", address, foreignKey, keyReused, err)
	}
	if gotName != foreignName {
		return fmt.Errorf("foreign name = %q, want %q (adopted/renamed? reused=%v)", gotName, foreignName, keyReused)
	}
	managedID, err := ResourceID(s, address)
	if err != nil {
		return err
	}
	if keyReused && managedID == foreignKey {
		return fmt.Errorf("managed %s adopted foreign key %d", address, foreignKey)
	}
	if gotKey == managedID {
		return fmt.Errorf("managed id %d matches foreign id", managedID)
	}
	return nil
}

// PlanWithoutRefresh is the plan flag for terraform destroy -refresh=false.
func PlanWithoutRefresh() *resource.AdditionalCLIOptions {
	return &resource.AdditionalCLIOptions{
		Plan: resource.PlanOptions{NoRefresh: true},
	}
}

// ForeignPresent fails when the outside row is missing or was renamed.
func ForeignPresent(foreignKey int, foreignName string, keyReused bool, lookup func(context.Context) (string, error)) error {
	name, err := lookup(context.Background())
	if err != nil {
		return fmt.Errorf("foreign row %d missing after destroy -refresh=false (key reused=%v): %w", foreignKey, keyReused, err)
	}
	if name != foreignName {
		return fmt.Errorf("foreign name = %q, want %q (key reused=%v)", name, foreignName, keyReused)
	}
	return nil
}
