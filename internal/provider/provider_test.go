package provider

import (
	"testing"
)

func TestProvider(t *testing.T) {
	provider := New("dev")()
	if provider == nil {
		t.Fatal("provider should not be nil")
	}
}