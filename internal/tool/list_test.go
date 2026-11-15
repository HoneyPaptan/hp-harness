package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestListWalksRelativeAndSkipsGit(t *testing.T) {
	m := host.NewMemory()
	for _, p := range []string{"/w/b.go", "/w/a/c.go", "/w/.git/HEAD"} {
		if err := m.WriteFile(p, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	env := Env{Host: m, Dir: "/w"}
	res, err := runList(context.Background(), env, json.RawMessage(`{}`))
	if err != nil || res.IsError {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if res.Text != "a/c.go\nb.go" {
		t.Fatalf("text = %q", res.Text)
	}
	res, _ = runList(context.Background(), env, json.RawMessage(`{"path":"a"}`))
	if res.Text != "c.go" {
		t.Fatalf("sub text = %q", res.Text)
	}
	res, _ = runList(context.Background(), env, json.RawMessage(`{"path":"nope"}`))
	if !res.IsError {
		t.Fatalf("missing path not an error: %+v", res)
	}
}

func TestBrowserBuiltinsHaveNoShell(t *testing.T) {
	for _, tl := range BrowserBuiltins() {
		if tl.Name == "bash" {
			t.Fatal("bash in browser builtins")
		}
	}
}
