package web

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	KindUser       = "user"
	KindText       = "text"
	KindThinking   = "thinking"
	KindToolStart  = "tool_start"
	KindToolResult = "tool_result"
	KindTurnEnd    = "turn_end"
	KindWait       = "wait"
	KindSwitch     = "switch"
	KindRetry      = "retry"
	KindError      = "error"
	KindRunStatus  = "run_status"

	toolTextCap = 30 * 1024
)

type Event struct {
	ID           int64     `json:"id"`
	Kind         string    `json:"kind"`
	Text         string    `json:"text,omitempty"`
	CallID       string    `json:"call_id,omitempty"`
	Name         string    `json:"name,omitempty"`
	Args         string    `json:"args,omitempty"`
	IsError      bool      `json:"is_error,omitempty"`
	Truncated    bool      `json:"truncated,omitempty"`
	Discard      bool      `json:"discard,omitempty"`
	Model        string    `json:"model,omitempty"`
	From         string    `json:"from,omitempty"`
	To           string    `json:"to,omitempty"`
	Until        time.Time `json:"until,omitzero"`
	DelayMS      int64     `json:"delay_ms,omitempty"`
	Attempt      int       `json:"attempt,omitempty"`
	Max          int       `json:"max,omitempty"`
	Status       string    `json:"status,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Steps        *int      `json:"steps,omitempty"`
	InputTokens  *int      `json:"input_tokens,omitempty"`
	OutputTokens *int      `json:"output_tokens,omitempty"`
	TTFTMS       *int64    `json:"ttft_ms,omitempty"`
	Jev          *JevSpend `json:"jev,omitempty"`
}

func capText(s string) (string, bool) {
	if len(s) <= toolTextCap {
		return s, false
	}
	cut := toolTextCap
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

func toolStartEvent(c provider.ToolCall) Event {
	args, cut := capText(string(c.Args))
	return Event{Kind: KindToolStart, CallID: c.ID, Name: c.Name, Args: args, Truncated: cut}
}

func toolResultEvent(r provider.ToolResult) Event {
	text, cut := capText(r.Text)
	return Event{Kind: KindToolResult, CallID: r.CallID, Text: text, IsError: r.IsError, Truncated: cut}
}

func turnEndEvent(s Stats) Event {
	ttft := s.TTFT.Milliseconds()
	return Event{
		Kind: KindTurnEnd, Reason: s.Reason, Steps: &s.Steps,
		InputTokens: &s.InputTokens, OutputTokens: &s.OutputTokens,
		TTFTMS: &ttft, Jev: s.Jev,
	}
}

func recordEvents(rec journal.Record) []Event {
	switch rec.Type {
	case journal.TypeUser:
		return decoded(rec, func(u journal.User) Event { return Event{Kind: KindUser, Text: u.Text} })
	case journal.TypeAssistant:
		return decoded(rec, func(a journal.Assistant) Event { return Event{Kind: KindText, Text: a.Message.Text} })
	case journal.TypeToolStarted:
		return decoded(rec, func(t journal.ToolStarted) Event { return toolStartEvent(t.Call) })
	case journal.TypeToolResult:
		return decoded(rec, func(t journal.ToolResultData) Event { return toolResultEvent(t.Result) })
	case journal.TypeLimitWait:
		return decoded(rec, func(l journal.LimitWait) Event { return Event{Kind: KindWait, Model: l.Model, Until: l.Until} })
	case journal.TypeModelSwitch:
		return decoded(rec, func(s journal.ModelSwitch) Event { return Event{Kind: KindSwitch, From: s.From, To: s.To} })
	case journal.TypeRunEnd:
		return decoded(rec, func(e journal.RunEnd) Event { return Event{Kind: KindTurnEnd, Reason: e.Reason} })
	}
	return nil
}

func decoded[T any](rec journal.Record, build func(T) Event) []Event {
	var v T
	if json.Unmarshal(rec.Data, &v) != nil {
		return nil
	}
	return []Event{build(v)}
}
