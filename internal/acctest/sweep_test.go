package acctest

import (
	"context"
	"os"
	"testing"
)

func TestReferencesSweptObject(t *testing.T) {
	vms := map[int]string{9: ResourcePrefix + "vm"}
	networks := map[int]string{4: ResourcePrefix + "net"}
	users := map[int]string{7: ResourcePrefix + "user"}

	if !referencesSweptObject("vms/9", vms, networks, users) {
		t.Fatal("vm reference should match")
	}
	if !referencesSweptObject("vnets/4", vms, networks, users) {
		t.Fatal("network reference should match")
	}
	if !referencesSweptObject("users/7", vms, networks, users) {
		t.Fatal("user reference should match")
	}
	if referencesSweptObject("vms/10", vms, networks, users) {
		t.Fatal("unrelated vm reference must not match")
	}
	if referencesSweptObject("production", vms, networks, users) {
		t.Fatal("bare names must not match")
	}
	if ownerVMID("vms/9") != 9 {
		t.Fatal("owner vm id was not parsed")
	}
	if ownerVMID("vnets/4") != 0 {
		t.Fatal("non-vm owner must be ignored")
	}
}

// TestSweep removes prefixed leftovers. It is skipped unless TF_ACC and lab
// credentials are set, so unit CI never contacts a VergeOS system.
func TestSweep(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance sweep skipped unless TF_ACC=1")
	}
	PreCheck(t)
	if err := Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
}
