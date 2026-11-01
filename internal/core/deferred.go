package core

import (
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func loadedTools(msgs []provider.Message) []string {
	var out []string
	searches := map[string]bool{}
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant {
			searches = searchCalls(m.ToolCalls)
			continue
		}
		for _, tr := range m.ToolResults {
			if searches[tr.CallID] && !tr.IsError {
				out = append(out, tool.LoadedNames(tr.Text)...)
			}
		}
	}
	return out
}

func searchCalls(calls []provider.ToolCall) map[string]bool {
	out := make(map[string]bool, len(calls))
	for _, c := range calls {
		if c.Name == tool.SearchName {
			out[c.ID] = true
		}
	}
	return out
}
