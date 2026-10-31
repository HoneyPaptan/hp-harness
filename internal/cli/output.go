package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type printer struct {
	env       Env
	id        string
	jev       *jev.Client
	midLine   bool
	turnStart time.Time
	ttft      time.Duration
	ttftDone  bool
}

func newPrinter(env Env, id string, client *jev.Client) *printer {
	return &printer{env: env, id: id, jev: client}
}

func (p *printer) observer() core.Observer {
	return core.Observer{
		OnTurnStart: p.onTurnStart, OnEvent: p.onEvent,
		OnToolStart: p.onToolStart, OnToolResult: p.onToolResult,
		OnWait: p.onWait, OnSwitch: p.onSwitch, OnRetry: p.onRetry,
	}
}

func (p *printer) onWait(spec string, d time.Duration, until time.Time) {
	p.ensureOutNL()
	sec := d.Round(time.Second)
	local := until.In(time.Local).Format("15:04:05")
	fmt.Fprintf(p.env.Stderr, "limited on %s, waiting %s until %s\n", spec, sec, local)
}

func (p *printer) onSwitch(from, to string) {
	p.ensureOutNL()
	fmt.Fprintf(p.env.Stderr, "switching from %s to %s\n", from, to)
}

func (p *printer) onRetry(d time.Duration, attempt, max int) {
	p.ensureOutNL()
	fmt.Fprintf(p.env.Stderr, "stream failed, retrying in %s (attempt %d/%d)\n", d.Round(time.Second), attempt, max)
}

func (p *printer) onTurnStart() {
	p.turnStart = p.env.Host.Now()
}

func (p *printer) onEvent(ev provider.Event) {
	p.markTTFT()
	if ev.Kind != provider.EventText || ev.Text == "" {
		return
	}
	io.WriteString(p.env.Stdout, ev.Text)
	p.midLine = !strings.HasSuffix(ev.Text, "\n")
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
	if !p.midLine {
		return
	}
	io.WriteString(p.env.Stdout, "\n")
	p.midLine = false
}

func (p *printer) summary(out core.Outcome) {
	p.ensureOutNL()
	fmt.Fprintf(p.env.Stderr, "run %s  %s  steps %d  tokens %d/%d  ttft %dms%s\n",
		p.id, out.Reason, out.Steps, out.Usage.InputTokens, out.Usage.OutputTokens, p.ttft.Milliseconds(), p.jevSummary())
}

func (p *printer) jevSummary() string {
	if p.jev == nil {
		return ""
	}
	s := p.jev.Spent()
	return fmt.Sprintf("  jev requests %d tokens %d/%d", s.Requests, s.InputTokens, s.OutputTokens)
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
