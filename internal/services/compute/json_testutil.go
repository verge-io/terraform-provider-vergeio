package compute

import (
	"encoding/json"
	"testing"
)

func jsonObject(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return obj
}

func requireBool(t *testing.T, obj map[string]any, key string, want bool) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %v", key, got, want)
	}
}

func requireString(t *testing.T, obj map[string]any, key, want string) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %q", key, got, want)
	}
}

func requireNumber(t *testing.T, obj map[string]any, key string, want float64) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %v", key, got, want)
	}
}

func requireAbsent(t *testing.T, obj map[string]any, key string) {
	t.Helper()
	if _, ok := obj[key]; ok {
		t.Fatalf("field %q was sent: %#v", key, obj[key])
	}
}
