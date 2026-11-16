package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHead(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	mem := NewMemory()
	if err := mem.WriteFile("/f", []byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		n    int
		want string
	}{{3, "abc"}, {6, "abcdef"}, {10, "abcdef"}}
	for _, tc := range cases {
		for name, got := range map[string]func() ([]byte, error){
			"native": func() ([]byte, error) { return ReadHead(Native{}, file, tc.n) },
			"memory": func() ([]byte, error) { return ReadHead(mem, "/f", tc.n) },
		} {
			data, err := got()
			if err != nil || string(data) != tc.want {
				t.Errorf("%s n=%d: got %q, %v, want %q", name, tc.n, data, err, tc.want)
			}
		}
	}
	if _, err := ReadHead(Native{}, filepath.Join(dir, "missing"), 3); !os.IsNotExist(err) {
		t.Fatalf("err = %v", err)
	}
}
