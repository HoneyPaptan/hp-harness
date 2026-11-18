package cli

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestDetailLines(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Text: "fix the bug\nin main.go"},
		{Role: provider.RoleAssistant, Text: "I will write it.", ToolCalls: []provider.ToolCall{
			{ID: "1", Name: "write", Args: json.RawMessage(`{"path":"a.txt","content":"one\ntwo\n","mode":420}`)},
		}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: "wrote a.txt\n"}}},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "2", Name: "bash", Args: json.RawMessage(`not json`)}}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "2", Text: "", IsError: true}}},
		{Role: provider.RoleAssistant, Text: "Done."},
	}
	want := []string{
		"› fix the bug",
		"in main.go",
		"",
		"I will write it.",
		"",
		"tool write",
		"    content:",
		"        one",
		"        two",
		"    mode: 420",
		"    path: a.txt",
		"",
		"result",
		"    wrote a.txt",
		"",
		"tool bash",
		"    not json",
		"",
		"error",
		"    (empty)",
		"",
		"Done.",
	}
	if got := detailLines(msgs); !slices.Equal(got, want) {
		t.Fatalf("lines:\n%q\nwant:\n%q", got, want)
	}
	if got := detailLines(nil); len(got) != 0 {
		t.Fatalf("empty run gave %q", got)
	}
}
