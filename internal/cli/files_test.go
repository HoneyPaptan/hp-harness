package cli

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

type linkHost struct {
	*host.Memory
	links map[string]bool
}

func (h linkHost) ReadDir(path string) ([]host.Entry, error) {
	ents, err := h.Memory.ReadDir(path)
	for i := range ents {
		ents[i].IsLink = h.links[filepath.Join(path, ents[i].Name)]
	}
	return ents, err
}

func filesEnv(t *testing.T, root string, setup func(h linkHost)) linkHost {
	t.Helper()
	h := linkHost{Memory: host.NewMemory(), links: map[string]bool{}}
	if err := h.WriteFile(filepath.Join(root, ".keep"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	setup(h)
	return h
}

func TestListFiles(t *testing.T) {
	root := "/proj"
	tests := []struct {
		name  string
		setup func(h linkHost)
		want  []string
	}{
		{
			name: "hidden skipped",
			setup: func(h linkHost) {
				_ = h.WriteFile(filepath.Join(root, ".git/config"), []byte("x"))
				_ = h.WriteFile(filepath.Join(root, "ok.txt"), []byte("x"))
			},
			want: []string{"ok.txt"},
		},
		{
			name: "node_modules skipped",
			setup: func(h linkHost) {
				_ = h.WriteFile(filepath.Join(root, "node_modules/x.js"), []byte("x"))
				_ = h.WriteFile(filepath.Join(root, "a.go"), []byte("x"))
			},
			want: []string{"a.go"},
		},
		{
			name: "link skipped",
			setup: func(h linkHost) {
				_ = h.WriteFile(filepath.Join(root, "real.txt"), []byte("x"))
				_ = h.WriteFile(filepath.Join(root, "linked.txt"), []byte("x"))
				h.links[filepath.Join(root, "linked.txt")] = true
			},
			want: []string{"real.txt"},
		},
		{
			name: "gitignore patterns",
			setup: func(h linkHost) {
				_ = h.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\nbuild/\n/dist\n"))
				_ = h.WriteFile(filepath.Join(root, "app.log"), []byte("x"))
				_ = h.WriteFile(filepath.Join(root, "ok.go"), []byte("x"))
				_ = h.WriteFile(filepath.Join(root, "build", "out"), []byte("x"))
				_ = h.WriteFile(filepath.Join(root, "dist", "x"), []byte("x"))
				_ = h.WriteFile(filepath.Join(root, "sub", "dist", "y"), []byte("x"))
			},
			want: []string{"ok.go", "sub/dist/y"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := filesEnv(t, root, tc.setup)
			got, err := listFiles(h, root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestListFilesCapAndSorted(t *testing.T) {
	root := "/many"
	h := filesEnv(t, root, func(h linkHost) {
		for i := range 210 {
			if err := h.WriteFile(filepath.Join(root, fmt.Sprintf("f%03d.txt", i)), []byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	})
	got, err := listFiles(h, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 200 {
		t.Fatalf("len %d", len(got))
	}
	want := append([]string(nil), got...)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("not sorted")
	}
}
