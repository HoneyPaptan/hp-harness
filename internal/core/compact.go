package core

import (
	"context"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	handoffPrefix         = "Earlier work in this run, compacted.\n\nUser prompts:\n"
	summarySystemPrompt   = "You write a handoff note for an agent that will continue this work. Say what was asked, what was done, which files changed, what failed, and what is left. Be short and concrete."
	summaryToolResultCap  = 2048
	defaultCompactPercent = 80
	summaryTokenCap       = 8192
)

func Compact(ctx context.Context, d Deps, journalPath string) (bool, error) {
	w, st, err := openJournal(d.Host, journalPath)
	if err != nil {
		return false, err
	}
	r, err := newResumeRunner(d, w, journalPath, st)
	if err != nil {
		return false, err
	}
	if !r.compactOn() {
		return false, nil
	}
	return r.compact(ctx, false)
}

func (r *runner) compactOn() bool {
	return r.deps.ContextWindow != nil && r.deps.ContextWindow(r.reqModel) > 0
}

func (r *runner) usableWindow() int {
	w := r.deps.ContextWindow(r.reqModel)
	if u := w - r.cfg.MaxTokens; u > 0 {
		return u
	}
	return w
}

func (r *runner) compactThreshold() int {
	p := r.deps.CompactPercent
	if p == 0 {
		p = defaultCompactPercent
	}
	return r.usableWindow() * p / 100
}

func (r *runner) overThreshold() bool {
	tok := r.lastTurnTokens
	if tok == 0 {
		for _, m := range r.msgs {
			tok += msgTokens(m)
		}
	}
	return tok >= r.compactThreshold()
}

func (r *runner) compact(ctx context.Context, overflow bool) (bool, error) {
	if !r.compactOn() {
		if overflow {
			return false, provider.ErrContextOverflow
		}
		return false, nil
	}
	keep := splitKeep(r.msgs, r.usableWindow())
	if keep <= 0 || keep >= len(r.msgs) {
		if overflow {
			return false, provider.ErrContextOverflow
		}
		return false, nil
	}
	old := r.msgs[:len(r.msgs)-keep]
	win := r.deps.ContextWindow(r.reqModel)
	prompts := fitUserPrompts(old, win/5)
	body := renderOldPlain(old)
	max := win / 10
	if max > summaryTokenCap {
		max = summaryTokenCap
	}
	msg, u, err := provider.Collect(r.prov.Stream(ctx, provider.Request{
		Model: r.reqModel, System: summarySystemPrompt, MaxTokens: max,
		Messages: []provider.Message{{Role: provider.RoleUser, Text: body}},
	}))
	if err != nil || msg.Text == "" {
		if overflow {
			return false, provider.ErrContextOverflow
		}
		return false, nil
	}
	r.usage.InputTokens += u.InputTokens
	r.usage.OutputTokens += u.OutputTokens
	handoff := handoffPrefix + prompts + "\n\nSummary:\n" + msg.Text
	if err := r.w.Compact(handoff, keep); err != nil {
		if overflow {
			return false, provider.ErrContextOverflow
		}
		return false, err
	}
	tail := append([]provider.Message(nil), r.msgs[len(r.msgs)-keep:]...)
	r.msgs = append([]provider.Message{{Role: provider.RoleUser, Text: handoff}}, tail...)
	r.fromHandoff = true
	r.lastTurnTokens = 0
	r.deps.Observer.emitCompact(keep)
	return true, nil
}

func splitKeep(msgs []provider.Message, usable int) int {
	n := len(msgs)
	if n <= 1 {
		return 0
	}
	target := usable / 20
	if target < 1 {
		target = 1
	}
	keep, sum := 1, msgTokens(msgs[n-1])
	for i := n - 2; i >= 0; i-- {
		if sum >= target {
			break
		}
		sum += msgTokens(msgs[i])
		keep++
	}
	at := n - keep
	for at > 0 {
		m := msgs[at]
		if m.Role == provider.RoleUser && m.Text == "" && len(m.ToolResults) > 0 {
			at--
			continue
		}
		if at > 0 && len(msgs[at-1].ToolCalls) > 0 {
			at--
			continue
		}
		break
	}
	if at <= 0 {
		return 0
	}
	return n - at
}

func msgTokens(m provider.Message) int {
	n := len(m.Text)
	for _, c := range m.ToolCalls {
		n += len(c.ID) + len(c.Name) + len(c.Args) + len(c.Extra)
	}
	for _, tr := range m.ToolResults {
		n += len(tr.CallID) + len(tr.Text)
	}
	return n / 4
}

func hasHandoffPrefix(s string) bool {
	return len(s) >= len(handoffPrefix) && s[:len(handoffPrefix)] == handoffPrefix
}

func fitUserPrompts(msgs []provider.Message, budget int) string {
	used, out := 0, ""
	for _, m := range msgs {
		if m.Role != provider.RoleUser || m.Text == "" || hasHandoffPrefix(m.Text) {
			continue
		}
		need := len(m.Text)
		if out != "" {
			need++
		}
		if used+need > budget {
			break
		}
		if out != "" {
			out += "\n"
		}
		out += m.Text
		used += need
	}
	return out
}

func renderOldPlain(msgs []provider.Message) string {
	out := ""
	for _, m := range msgs {
		if m.Text != "" {
			out += m.Text + "\n"
		}
		for _, tr := range m.ToolResults {
			t := tr.Text
			if len(t) > summaryToolResultCap {
				t = t[:summaryToolResultCap]
			}
			out += t + "\n"
		}
	}
	return out
}

func (r *runner) markFromHandoff() {
	r.fromHandoff = len(r.msgs) > 0 && r.msgs[0].Role == provider.RoleUser && hasHandoffPrefix(r.msgs[0].Text)
}
