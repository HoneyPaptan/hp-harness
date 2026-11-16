package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const maxToolNotes = 2

func (r *runner) correctUnknownTool(err error, ut *provider.UnknownToolError, defs []provider.ToolDef) error {
	if r.toolNotes >= maxToolNotes {
		return fmt.Errorf("model kept calling tools that do not exist, gave up after %d corrections: %w", maxToolNotes, err)
	}
	note := unknownToolNote(ut.Name, defs)
	if err := r.w.ToolNote(note); err != nil {
		return err
	}
	r.msgs = append(r.msgs, provider.Message{Role: provider.RoleUser, Text: note})
	r.toolNotes++
	r.showUnknownTool(ut.Name)
	return nil
}

func (r *runner) showUnknownTool(name string) {
	id := fmt.Sprintf("tool-note-%d", r.toolNotes)
	r.deps.Observer.emitToolStart(provider.ToolCall{ID: id, Name: name, Args: json.RawMessage("{}")})
	text := fmt.Sprintf("no tool named %s; asked the model to use a real one (%d/%d)", name, r.toolNotes, maxToolNotes)
	if name == "" {
		text = fmt.Sprintf("no such tool; asked the model to use a real one (%d/%d)", r.toolNotes, maxToolNotes)
	}
	r.deps.Observer.emitToolResult(provider.ToolResult{CallID: id, Text: text, IsError: true})
}

func unknownToolNote(name string, defs []provider.ToolDef) string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	called := "a tool that does not exist"
	if name != "" {
		called = fmt.Sprintf("a tool named %q, which does not exist", name)
	}
	return fmt.Sprintf("Your last reply called %s, so it was discarded. Use only these tools: %s. Redo that step.", called, strings.Join(names, ", "))
}
