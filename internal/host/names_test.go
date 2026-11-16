package host

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestListNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	got, err := ReadNames(Native{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{Name: "a.jsonl"}, {Name: "link", IsDir: true, IsLink: true}, {Name: "sub", IsDir: true}}
	slices.SortFunc(got, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadNamesFallsBackToReadDir(t *testing.T) {
	m := NewMemory()
	if err := m.WriteFile("/d/x.jsonl", []byte("1")); err != nil {
		t.Fatal(err)
	}
	got, err := ReadNames(m, "/d")
	if err != nil || len(got) != 1 || got[0].Name != "x.jsonl" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
