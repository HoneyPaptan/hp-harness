package crew

import (
	"strings"
	"testing"
)

func TestParsePlan(t *testing.T) {
	cases := []struct {
		name    string
		plan    string
		want    []planTask
		wantErr string
	}{
		{
			name: "two tasks",
			plan: "# Goal\n\n## Read the code\nread files\n\n## Fix the bug\nmake the change\n",
			want: []planTask{
				{title: "Read the code", prompt: "read files", slug: "read-the-code"},
				{title: "Fix the bug", prompt: "make the change", slug: "fix-the-bug"},
			},
		},
		{
			name: "heading without body uses title",
			plan: "## Write tests\n",
			want: []planTask{
				{title: "Write tests", prompt: "Write tests", slug: "write-tests"},
			},
		},
		{
			name: "duplicate slugs",
			plan: "## Fix it\n## Fix it\n",
			want: []planTask{
				{title: "Fix it", prompt: "Fix it", slug: "fix-it"},
				{title: "Fix it", prompt: "Fix it", slug: "fix-it-2"},
			},
		},
		{
			name:    "empty plan",
			plan:    "# nothing\n",
			wantErr: "no ## tasks",
		},
		{
			name: "preamble ignored",
			plan: "intro text\n## Do work\nbody\n",
			want: []planTask{
				{title: "Do work", prompt: "body", slug: "do-work"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePlan([]byte(tc.plan))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err %v want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d tasks want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("task %d got %+v want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTaskSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Fix the README", "fix-the-readme"},
		{"Add: MCP support", "add-mcp-support"},
		{"Éclair!", "clair"},
		{"  ", "task"},
		{"a--b", "a-b"},
		{"CamelCase 2", "camelcase-2"},
	}
	for _, tc := range cases {
		if got := taskSlug(tc.in); got != tc.want {
			t.Errorf("taskSlug(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}
