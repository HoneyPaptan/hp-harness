package tool

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
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

type walked struct {
	path string
	link bool
}

func TestWalk(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		links []string
		root  string
		want  []walked
	}{
		{
			name:  "nested files",
			files: []string{"/w/a.txt", "/w/sub/b.txt", "/w/sub/deep/c.txt"},
			root:  "/w",
			want:  []walked{{path: "/w/a.txt"}, {path: "/w/sub/b.txt"}, {path: "/w/sub/deep/c.txt"}},
		},
		{
			name:  "git directory skipped",
			files: []string{"/w/.git/config", "/w/sub/.git/config", "/w/a.txt"},
			root:  "/w",
			want:  []walked{{path: "/w/a.txt"}},
		},
		{
			name:  "symlinked directory not descended",
			files: []string{"/w/a.txt", "/w/linked/b.txt"},
			links: []string{"/w/linked"},
			root:  "/w",
			want:  []walked{{path: "/w/a.txt"}},
		},
		{
			name:  "symlinked file visited with link flag",
			files: []string{"/w/a.txt", "/w/b.txt"},
			links: []string{"/w/b.txt"},
			root:  "/w",
			want:  []walked{{path: "/w/a.txt"}, {path: "/w/b.txt", link: true}},
		},
		{
			name:  "file root visited",
			files: []string{"/w/a.txt", "/w/b.txt"},
			root:  "/w/a.txt",
			want:  []walked{{path: "/w/a.txt"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := walkEnv(t, tt.files, tt.links)
			var got []walked
			err := Walk(env, tt.root, func(p string, e host.Entry) {
				got = append(got, walked{path: p, link: e.IsLink})
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestWalkMissingRoot(t *testing.T) {
	env := walkEnv(t, nil, nil)
	err := Walk(env, "/missing", func(string, host.Entry) { t.Fatal("visited") })
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err %v", err)
	}
}

func walkEnv(t *testing.T, files, links []string) Env {
	t.Helper()
	h := linkHost{Memory: host.NewMemory(), links: map[string]bool{}}
	for _, f := range files {
		if err := h.WriteFile(f, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range links {
		h.links[l] = true
	}
	return Env{Host: h, Dir: "/w"}
}
