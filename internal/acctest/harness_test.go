package acctest

import (
	"strings"
	"testing"
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
