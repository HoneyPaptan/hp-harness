package cli

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	detailIndent = "    "
	detailPrompt = "› "
)

func (a *sessionAgent) Detail() ([]string, error) {
	if !a.started {
		return nil, nil
	}
	recs, err := journal.Read(a.env.Host, a.jpath)
	if err != nil {
		return nil, err
	}
	st, err := journal.Replay(recs)
	if err != nil {
		return nil, err
	}
	return detailLines(st.Messages), nil
}

func detailLines(msgs []provider.Message) []string {
	var out []string
	block := func(lines []string) {
		if len(lines) == 0 {
			return
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, lines...)
	}
	for _, m := range msgs {
		block(messageText(m))
		for _, c := range m.ToolCalls {
			block(append([]string{"tool " + c.Name}, argLines(c.Args)...))
		}
		for _, r := range m.ToolResults {
			block(append([]string{resultLabel(r)}, indented(r.Text)...))
		}
	}
	return out
}

func messageText(m provider.Message) []string {
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if m.Role == provider.RoleUser {
		lines[0] = detailPrompt + lines[0]
	}
	return lines
}

func resultLabel(r provider.ToolResult) string {
	if r.IsError {
		return "error"
	}
	return "result"
}

func indented(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return []string{detailIndent + "(empty)"}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = detailIndent + line
	}
	return lines
}

func argLines(raw json.RawMessage) []string {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		return indented(string(raw))
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var out []string
	for _, k := range keys {
		out = append(out, argValue(k, args[k])...)
	}
	return out
}

func argValue(key string, raw json.RawMessage) []string {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return []string{detailIndent + key + ": " + string(raw)}
	}
	if !strings.Contains(strings.TrimRight(text, "\n"), "\n") {
		return []string{detailIndent + key + ": " + strings.TrimRight(text, "\n")}
	}
	lines := []string{detailIndent + key + ":"}
	for _, line := range indented(text) {
		lines = append(lines, detailIndent+line)
	}
	return lines
}
