package jev

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestSearchNativeSymlinksWithheld(t *testing.T) {
	const secret = "TOPSECRETVALUE"
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "target"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "target"), filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "safe.go"), []byte("harmless\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{"query":"q"}`, `{"query":"q","path":"notes.txt"}`, `{"query":"q","path":"linked"}`, `{"query":"q","path":"linked/target"}`} {
		c, f := searchServer(t, func(string) float64 { return 0.9 })
		env := tool.Env{Host: host.Native{}, Dir: dir}
		if _, err := SearchTool(c).Run(context.Background(), env, json.RawMessage(args)); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(f.sent(), secret) || strings.Contains(f.sent(), "notes.txt") {
			t.Fatalf("%s leaked", args)
		}
	}
}
