package skill

import (
	"reflect"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func writeSkill(h host.Host, dir, folder, content string) {
	h.WriteFile(dir+"/"+folder+"/SKILL.md", []byte(content))
}

func TestDiscover(t *testing.T) {
	tests := []struct {
		name  string
		setup func(h host.Host)
		dirs  []string
		want  []Skill
	}{
		{
			name: "sorted",
			setup: func(h host.Host) {
				writeSkill(h, "/a", "beta", "---\ndescription: B\n---\n")
				writeSkill(h, "/a", "alpha", "---\ndescription: A\n---\n")
			},
			dirs: []string{"/a"},
			want: []Skill{
				{Name: "alpha", Description: "A", Dir: "/a/alpha", Path: "/a/alpha/SKILL.md"},
				{Name: "beta", Description: "B", Dir: "/a/beta", Path: "/a/beta/SKILL.md"},
			},
		},
		{
			name: "folder name",
			setup: func(h host.Host) {
				writeSkill(h, "/a", "deploy", "---\ndescription: d\n---\n")
			},
			dirs: []string{"/a"},
			want: []Skill{{Name: "deploy", Description: "d", Dir: "/a/deploy", Path: "/a/deploy/SKILL.md"}},
		},
		{
			name: "skip no description",
			setup: func(h host.Host) {
				writeSkill(h, "/a", "x", "---\nname: x\n---\n")
			},
			dirs: []string{"/a"},
			want: nil,
		},
		{
			name: "skip no frontmatter",
			setup: func(h host.Host) {
				writeSkill(h, "/a", "x", "plain\n")
			},
			dirs: []string{"/a"},
			want: nil,
		},
		{
			name:  "missing dir",
			setup: func(h host.Host) {},
			dirs:  []string{"/missing", "/a"},
			want:  nil,
		},
		{
			name: "first dir wins",
			setup: func(h host.Host) {
				writeSkill(h, "/first", "dup", "---\ndescription: one\n---\n")
				writeSkill(h, "/second", "dup", "---\ndescription: two\n---\n")
			},
			dirs: []string{"/first", "/second"},
			want: []Skill{{Name: "dup", Description: "one", Dir: "/first/dup", Path: "/first/dup/SKILL.md"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			tc.setup(h)
			got, err := Discover(h, tc.dirs)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
