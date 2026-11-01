package anthropic

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func encoded(t *testing.T, req provider.Request) map[string]any {
	t.Helper()
	b, err := json.Marshal(buildRequest(req, 0))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func blockTypes(m wireMessage) []string {
	out := make([]string, len(m.Content))
	for i, b := range m.Content {
		out[i] = b.Type
	}
	return out
}

func TestMapMessagesMerging(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "read", Args: json.RawMessage(`{"path":"a"}`)}
	result := provider.ToolResult{CallID: "c1", Text: "out"}
	cases := []struct {
		name string
		msgs []provider.Message
		want [][]string
	}{
		{
			name: "consecutive prompts merge",
			msgs: []provider.Message{
				{Role: provider.RoleUser, Text: "one"},
				{Role: provider.RoleUser, Text: "two"},
			},
			want: [][]string{{"text", "text"}},
		},
		{
			name: "interrupted tools then a prompt",
			msgs: []provider.Message{
				{Role: provider.RoleUser, Text: "go"},
				{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{call}},
				{Role: provider.RoleUser, ToolResults: []provider.ToolResult{result}},
				{Role: provider.RoleUser, Text: "stop that"},
			},
			want: [][]string{{"text"}, {"tool_use"}, {"tool_result", "text"}},
		},
		{
			name: "results sort ahead of earlier text",
			msgs: []provider.Message{
				{Role: provider.RoleUser, Text: "first"},
				{Role: provider.RoleUser, ToolResults: []provider.ToolResult{result}},
			},
			want: [][]string{{"tool_result", "text"}},
		},
		{
			name: "same message orders results first",
			msgs: []provider.Message{
				{Role: provider.RoleUser, Text: "text", ToolResults: []provider.ToolResult{result}},
			},
			want: [][]string{{"tool_result", "text"}},
		},
		{
			name: "empty assistant turn disappears and users merge",
			msgs: []provider.Message{
				{Role: provider.RoleUser, Text: "one"},
				{Role: provider.RoleAssistant},
				{Role: provider.RoleUser, Text: "two"},
			},
			want: [][]string{{"text", "text"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapMessages(tc.msgs)
			shape := make([][]string, len(got))
			for i, m := range got {
				shape[i] = blockTypes(m)
			}
			if !reflect.DeepEqual(shape, tc.want) {
				t.Fatalf("shape = %v, want %v", shape, tc.want)
			}
		})
	}
}

func TestCacheControlPlacement(t *testing.T) {
	req := provider.Request{
		Model: "m", System: "sys",
		Tools: []provider.ToolDef{{Name: "a", Schema: json.RawMessage(`{"type":"object"}`)}, {Name: "b"}},
		Messages: []provider.Message{
			{Role: provider.RoleUser, Text: "one"},
			{Role: provider.RoleAssistant, Text: "ok"},
			{Role: provider.RoleUser, Text: "two"},
			{Role: provider.RoleUser, Text: "three"},
		},
	}
	w := buildRequest(req, 0)
	if w.System[0].CacheControl == nil {
		t.Fatal("system block has no breakpoint")
	}
	if w.Tools[0].CacheControl != nil || w.Tools[1].CacheControl == nil {
		t.Fatalf("tool breakpoints wrong: %+v", w.Tools)
	}
	var marked []string
	for _, m := range w.Messages {
		for _, b := range m.Content {
			if b.CacheControl != nil {
				marked = append(marked, b.Text)
			}
		}
	}
	if !reflect.DeepEqual(marked, []string{"three"}) {
		t.Fatalf("message breakpoints on %v", marked)
	}
}

func TestToolDefinitionBytesStayStable(t *testing.T) {
	tools := []provider.ToolDef{
		{Name: "z", Schema: json.RawMessage(`{"b":1,"a":2}`)},
		{Name: "a", Schema: json.RawMessage(`{"type":"object"}`)},
	}
	first, _ := json.Marshal(mapTools(tools))
	second, _ := json.Marshal(mapTools(tools))
	if string(first) != string(second) {
		t.Fatalf("differs:\n%s\n%s", first, second)
	}
	if !strings.Contains(string(first), `{"b":1,"a":2}`) {
		t.Fatalf("schema rewritten: %s", first)
	}
}

func TestMaxTokensChoice(t *testing.T) {
	cases := []struct {
		name              string
		requested, config int
		want              int
	}{
		{name: "built in default", want: defaultMaxTokens},
		{name: "client config", config: 900, want: 900},
		{name: "request wins", requested: 50, config: 900, want: 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maxTokens(tc.requested, tc.config); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestWireIDAcceptsForeignToolCallIDs(t *testing.T) {
	msgs := mapMessages([]provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "functions.read:0", Name: "read"}}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "functions.read:0", Text: "x"}}},
	})
	use, result := msgs[0].Content[0], msgs[1].Content[0]
	if use.ID != "functions_read_0" || result.ToolUseID != use.ID {
		t.Fatalf("ids %q %q", use.ID, result.ToolUseID)
	}
}

func TestToolInputAlwaysAnObject(t *testing.T) {
	cases := []struct {
		name string
		args string
		want string
	}{
		{name: "object", args: `{"a":1}`, want: `{"a":1}`},
		{name: "empty", args: "", want: "{}"},
		{name: "cut off", args: `{"a":`, want: "{}"},
		{name: "array", args: `[1]`, want: "{}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(toolInput(json.RawMessage(tc.args))); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRequestShape(t *testing.T) {
	got := encoded(t, provider.Request{
		Model: "m", Messages: []provider.Message{{Role: provider.RoleUser, Text: "hi"}},
	})
	if got["stream"] != true || got["max_tokens"] != float64(defaultMaxTokens) {
		t.Fatalf("request = %v", got)
	}
	if _, ok := got["system"]; ok {
		t.Fatal("empty system was sent")
	}
}
