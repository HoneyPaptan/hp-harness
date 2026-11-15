package core

import (
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
	r.deps.Observer.emitRetry(0, r.toolNotes, maxToolNotes)
	return nil
}

func unknownToolNote(name string, defs []provider.ToolDef) string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return fmt.Sprintf("Your last reply called a tool named %q, which does not exist, so it was discarded. Use only these tools: %s. Redo that step.", name, strings.Join(names, ", "))
}
