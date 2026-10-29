package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type printer struct {
	env       Env
	id        string
	outNL     bool
	turnStart time.Time
	ttft      time.Duration
	ttftDone  bool
}

func newPrinter(env Env, id string) *printer {
	return &printer{env: env, id: id}
}

func (p *printer) observer() core.Observer {
	return core.Observer{
		OnTurnStart: p.onTurnStart, OnEvent: p.onEvent,
		OnToolStart: p.onToolStart, OnToolResult: p.onToolResult,
	}
}

func (p *printer) onTurnStart() {
	p.turnStart = p.env.Host.Now()
}

func (p *printer) onEvent(ev provider.Event) {
	p.markTTFT()
	if ev.Kind != provider.EventText {
		return
	}
	io.WriteString(p.env.Stdout, ev.Text)
	p.outNL = len(ev.Text) > 0 && ev.Text[len(ev.Text)-1] == '\n'
}

func (p *printer) markTTFT() {
	if p.ttftDone || p.turnStart.IsZero() {
		return
	}
	p.ttft = p.env.Host.Now().Sub(p.turnStart)
	p.ttftDone = true
}

func (p *printer) onToolStart(c provider.ToolCall) {
	p.markTTFT()
	p.ensureOutNL()
	fmt.Fprintf(p.env.Stderr, "> %s %s\n", c.Name, toolMainArg(c.Args))
}

func (p *printer) onToolResult(r provider.ToolResult) {
	if !r.IsError {
		return
	}
	line := firstLine(r.Text)
	fmt.Fprintf(p.env.Stderr, "  ! %s\n", cutRunes(line, 120))
}

func (p *printer) ensureOutNL() {
	if p.outNL {
		return
	}
	io.WriteString(p.env.Stdout, "\n")
	p.outNL = true
}

func (p *printer) summary(out core.Outcome) {
	io.WriteString(p.env.Stderr, "\n")
	fmt.Fprintf(p.env.Stderr, "run %s  %s  steps %d  tokens %d/%d  ttft %dms\n",
		p.id, out.Reason, out.Steps, out.Usage.InputTokens, out.Usage.OutputTokens, p.ttft.Milliseconds())
}

func toolMainArg(raw json.RawMessage) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, k := range []string{"path", "command", "pattern", "name"} {
		if v, ok := m[k]; ok {
			return cutRunes(cleanArg(v), 80)
		}
	}
	return ""
}

func cleanArg(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) != nil {
		return string(v)
	}
	return replaceNL(s)
}

func replaceNL(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' {
			out = append(out, ' ')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

func cutRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	n := max - 3
	if n < 0 {
		n = 0
	}
	return string(r[:n]) + "..."
}
