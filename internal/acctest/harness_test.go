package acctest

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"
)

func TestNameUsesRequiredPrefix(t *testing.T) {
	name := Name("network")
	if err := RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, ResourcePrefix+"network-") {
		t.Fatalf("name %q does not include the resource kind after the prefix", name)
	}
	if name == Name("network") {
		t.Fatal("Name should return a unique suffix")
	}
}

func TestNameStripsDuplicatePrefix(t *testing.T) {
	name := Name(ResourcePrefix + "vm")
	if strings.HasPrefix(name, ResourcePrefix+ResourcePrefix) {
		t.Fatalf("prefix was applied twice: %s", name)
	}
	if err := RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
}

func TestRequirePrefixRejectsUnprefixedNames(t *testing.T) {
	for _, name := range []string{"", "prod-net", "tf-acc", "TF-ACC-vm"} {
		if err := RequirePrefix(name); err == nil {
			t.Errorf("RequirePrefix(%q) succeeded", name)
		}
	}
}

func TestHasPrefix(t *testing.T) {
	if HasPrefix("production-vm") {
		t.Fatal("production names must not match the sweeper prefix")
	}
	if !HasPrefix(ResourcePrefix + "vm-abc") {
		t.Fatal("prefixed names must match")
	}
}

func TestCheckDeletedMatchingTreatsForeignKeyAsGone(t *testing.T) {
	s := &terraform.State{
		Modules: []*terraform.ModuleState{
			{
				Path: []string{"root"},
				Resources: map[string]*terraform.ResourceState{
					"vergeio_tenant_storage.test": {
						Type: "vergeio_tenant_storage",
						Primary: &terraform.InstanceState{
							ID: "2",
							Attributes: map[string]string{
								"id":        "2",
								"tenant_id": "10",
							},
						},
					},
				},
			},
		},
	}

	err := CheckDeletedMatching(s, "vergeio_tenant_storage", func(ctx context.Context, id int, attrs map[string]string) error {
		if id != 2 {
			t.Fatalf("id = %d", id)
		}
		if attrs["tenant_id"] != "10" {
			t.Fatalf("attrs = %#v", attrs)
		}
		// Simulate Get success for a foreign row that reused key 2.
		return &vergeos.NotFoundError{Resource: "TenantStorage", ID: id}
	})
	if err != nil {
		t.Fatalf("foreign reused key should be treated as gone: %v", err)
	}
}

func TestCheckDeletedMatchingFailsWhenOwnedRowRemains(t *testing.T) {
	s := &terraform.State{
		Modules: []*terraform.ModuleState{
			{
				Path: []string{"root"},
				Resources: map[string]*terraform.ResourceState{
					"vergeio_tenant_storage.test": {
						Type: "vergeio_tenant_storage",
						Primary: &terraform.InstanceState{
							ID: "2",
							Attributes: map[string]string{
								"id":        "2",
								"tenant_id": "10",
							},
						},
					},
				},
			},
		},
	}

	err := CheckDeletedMatching(s, "vergeio_tenant_storage", func(ctx context.Context, id int, attrs map[string]string) error {
		return nil // owned row still present
	})
	if err == nil || !strings.Contains(err.Error(), "still exists after destroy") {
		t.Fatalf("err = %v, want still exists", err)
	}
}

func TestCheckDeletedWrapsMatching(t *testing.T) {
	s := &terraform.State{
		Modules: []*terraform.ModuleState{
			{
				Path: []string{"root"},
				Resources: map[string]*terraform.ResourceState{
					"vergeio_tenant.test": {
						Type: "vergeio_tenant",
						Primary: &terraform.InstanceState{
							ID:         "5",
							Attributes: map[string]string{"id": "5", "name": "gone"},
						},
					},
				},
			},
		},
	}
	err := CheckDeleted(s, "vergeio_tenant", func(ctx context.Context, id int) error {
		return &vergeos.NotFoundError{Resource: "Tenant", ID: id}
	})
	if err != nil {
		t.Fatal(err)
	}
}
