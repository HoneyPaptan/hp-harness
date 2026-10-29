package tool

import (
	"encoding/json"
	"testing"
)

func TestRegistry(t *testing.T) {
	a := Tool{Name: "a", Description: "da", Schema: json.RawMessage(`{"type":"object"}`)}
	b := Tool{Name: "b", Description: "db", Schema: json.RawMessage(`{"type":"object","x":1}`)}
	r := NewRegistry(a, b)
	if _, ok := r.Get("missing"); ok {
		t.Fatal("expected miss")
	}
	defs := r.Defs()
	if len(defs) != 2 || defs[0].Name != "a" || defs[1].Name != "b" {
		t.Fatalf("order: %+v", defs)
	}
	if string(defs[1].Schema) != `{"type":"object","x":1}` {
		t.Fatalf("schema passthrough: %s", defs[1].Schema)
	}
}

func TestBuiltinsOrder(t *testing.T) {
	names := make([]string, len(Builtins()))
	for i, t := range Builtins() {
		names[i] = t.Name
	}
	want := []string{"read", "write", "edit", "bash", "grep"}
	if len(names) > len(want) {
		t.Fatalf("builtins: %v", names)
	}
	for i, name := range names {
		if name != want[i] {
			t.Fatalf("got %v want prefix %v", names, want[:len(names)])
		}
	}
}
