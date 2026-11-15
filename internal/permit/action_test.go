package permit

import (
	"encoding/json"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestDescribeTools(t *testing.T) {
	dir := "/proj"
	tests := []struct {
		name string
		call provider.ToolCall
		want Action
	}{
		{
			name: "list dir",
			call: provider.ToolCall{Name: "list", Args: json.RawMessage(`{"path":"src"}`)},
			want: Action{Tool: "list", Target: "src", Kind: KindRead},
		},
		{
			name: "read relative",
			call: provider.ToolCall{Name: "read", Args: json.RawMessage(`{"path":"README.md"}`)},
			want: Action{Tool: "read", Target: "README.md", Kind: KindRead},
		},
		{
			name: "read absolute inside",
			call: provider.ToolCall{Name: "read", Args: json.RawMessage(`{"path":"/proj/internal/x.go"}`)},
			want: Action{Tool: "read", Target: "internal/x.go", Kind: KindRead},
		},
		{
			name: "read outside",
			call: provider.ToolCall{Name: "read", Args: json.RawMessage(`{"path":"/etc/passwd"}`)},
			want: Action{Tool: "read", Target: "/etc/passwd", Kind: KindRead},
		},
		{
			name: "grep path",
			call: provider.ToolCall{Name: "grep", Args: json.RawMessage(`{"pattern":"x","path":"cmd"}`)},
			want: Action{Tool: "grep", Target: "cmd", Kind: KindRead},
		},
		{
			name: "grep default path",
			call: provider.ToolCall{Name: "grep", Args: json.RawMessage(`{"pattern":"x"}`)},
			want: Action{Tool: "grep", Target: ".", Kind: KindRead},
		},
		{
			name: "edit",
			call: provider.ToolCall{Name: "edit", Args: json.RawMessage(`{"path":"a.go","old_string":"a","new_string":"b"}`)},
			want: Action{Tool: "edit", Target: "a.go", Kind: KindWrite},
		},
		{
			name: "write",
			call: provider.ToolCall{Name: "write", Args: json.RawMessage(`{"path":"internal/x.go","content":"x"}`)},
			want: Action{Tool: "write", Target: "internal/x.go", Kind: KindWrite},
		},
		{
			name: "bash",
			call: provider.ToolCall{Name: "bash", Args: json.RawMessage(`{"command":"go test ./..."}`)},
			want: Action{Tool: "bash", Target: "go test ./...", Kind: KindExec},
		},
		{
			name: "search",
			call: provider.ToolCall{Name: "search", Args: json.RawMessage(`{"query":"how auth works"}`)},
			want: Action{Tool: "search", Target: "how auth works", Kind: KindNet},
		},
		{
			name: "web_fetch",
			call: provider.ToolCall{Name: "web_fetch", Args: json.RawMessage(`{"url":"https://pkg.go.dev/std"}`)},
			want: Action{Tool: "web_fetch", Target: "https://pkg.go.dev/std", Kind: KindExternal},
		},
		{
			name: "web_search",
			call: provider.ToolCall{Name: "web_search", Args: json.RawMessage(`{"query":"go modules"}`)},
			want: Action{Tool: "web_search", Target: "go modules", Kind: KindExternal},
		},
		{
			name: "skill",
			call: provider.ToolCall{Name: "skill", Args: json.RawMessage(`{"name":"deploy"}`)},
			want: Action{Tool: "skill", Target: "deploy", Kind: KindMeta},
		},
		{
			name: "tools",
			call: provider.ToolCall{Name: "tools", Args: json.RawMessage(`{"query":"slack"}`)},
			want: Action{Tool: "tools", Target: "", Kind: KindMeta},
		},
		{
			name: "mcp",
			call: provider.ToolCall{Name: "slack__post", Args: json.RawMessage(`{}`)},
			want: Action{Tool: "mcp", Target: "slack__post", Kind: KindExternal},
		},
		{
			name: "unknown",
			call: provider.ToolCall{Name: "custom", Args: json.RawMessage(`{}`)},
			want: Action{Tool: "custom", Target: "custom", Kind: KindExternal},
		},
		{
			name: "read bad json",
			call: provider.ToolCall{Name: "read", Args: json.RawMessage(`{`)},
			want: Action{Tool: "read", Target: "", Kind: KindRead},
		},
		{
			name: "bash bad json",
			call: provider.ToolCall{Name: "bash", Args: json.RawMessage(`not json`)},
			want: Action{Tool: "bash", Target: "", Kind: KindExec},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Describe(tc.call, dir)
			if got != tc.want {
				t.Fatalf("Describe() = %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestActionString(t *testing.T) {
	tests := []struct {
		a    Action
		want string
	}{
		{Action{Tool: "bash", Target: "go test ./..."}, "bash: go test ./..."},
		{Action{Tool: "write", Target: "internal/x.go"}, "write: internal/x.go"},
		{Action{Tool: "read", Target: "README.md"}, "read: README.md"},
		{Action{Tool: "mcp", Target: "slack__post"}, "mcp: slack__post"},
		{Action{Tool: "search", Target: "query"}, "search: query"},
	}
	for _, tc := range tests {
		if got := tc.a.String(); got != tc.want {
			t.Fatalf("String() = %q want %q", got, tc.want)
		}
	}
}

func TestStringArgIgnoresOtherFieldTypes(t *testing.T) {
	cases := map[string]provider.ToolCall{
		"x":              {Name: "web_search", Args: json.RawMessage(`{"query":"x","count":5}`)},
		"y":              {Name: "search", Args: json.RawMessage(`{"query":"y","limit":3}`)},
		"https://a.dev/": {Name: "web_fetch", Args: json.RawMessage(`{"url":"https://a.dev/","extra":true}`)},
	}
	for want, call := range cases {
		if got := Describe(call, "/w").Target; got != want {
			t.Errorf("%s: target %q", call.Name, got)
		}
	}
}
