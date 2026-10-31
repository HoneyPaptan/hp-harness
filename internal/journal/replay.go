package journal

import (
	"encoding/json"
	"fmt"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type Pending struct {
	Call    provider.ToolCall
	Started bool
}

type State struct {
	Start     RunStart
	Messages  []provider.Message
	Pending   []Pending
	Steps     int
	Current   string
	Finished  bool
	EndReason string
}

type replayer struct {
	st        State
	started   map[string]bool
	resultIDs map[string]bool
}

func Replay(records []Record) (State, error) {
	if err := checkStart(records); err != nil {
		return State{}, err
	}
	r := &replayer{
		started:   make(map[string]bool),
		resultIDs: make(map[string]bool),
	}
	for i, rec := range records {
		if err := r.apply(rec, i); err != nil {
			return State{}, err
		}
	}
	r.st.Pending = pendingCalls(r.st.Messages, r.started, r.resultIDs)
	return r.st, nil
}

func decode[T any](rec Record) (T, error) {
	var v T
	err := json.Unmarshal(rec.Data, &v)
	return v, err
}

func (r *replayer) apply(rec Record, i int) error {
	switch rec.Type {
	case TypeRunStart:
		return r.applyRunStart(rec, i)
	case TypeUser:
		return r.applyUser(rec)
	case TypeAssistant:
		return r.applyAssistant(rec)
	case TypeToolStarted:
		return r.applyToolStarted(rec)
	case TypeToolResult:
		return r.applyToolResult(rec)
	case TypeLimitWait:
		return nil
	case TypeModelSwitch:
		return r.applyModelSwitch(rec)
	case TypeRunEnd:
		return r.applyRunEnd(rec)
	default:
		return fmt.Errorf("unknown record type %q", rec.Type)
	}
}

func (r *replayer) applyRunStart(rec Record, i int) error {
	if i != 0 {
		return fmt.Errorf("unexpected %s", TypeRunStart)
	}
	start, err := decode[RunStart](rec)
	if err != nil {
		return err
	}
	r.st.Start = start
	return nil
}

func (r *replayer) applyUser(rec Record) error {
	u, err := decode[User](rec)
	if err != nil {
		return err
	}
	r.st.Messages = append(r.st.Messages, provider.Message{Role: provider.RoleUser, Text: u.Text})
	r.st.Steps = 0
	r.st.Finished = false
	r.st.EndReason = ""
	return nil
}

func (r *replayer) applyAssistant(rec Record) error {
	a, err := decode[Assistant](rec)
	if err != nil {
		return err
	}
	r.st.Messages = append(r.st.Messages, a.Message)
	r.st.Steps++
	clear(r.started)
	clear(r.resultIDs)
	return nil
}

func (r *replayer) applyToolStarted(rec Record) error {
	ts, err := decode[ToolStarted](rec)
	if err != nil {
		return err
	}
	r.started[ts.Call.ID] = true
	return nil
}

func (r *replayer) applyToolResult(rec Record) error {
	tr, err := decode[ToolResultData](rec)
	if err != nil {
		return err
	}
	r.resultIDs[tr.Result.CallID] = true
	r.st.Messages = attachResult(r.st.Messages, tr.Result)
	return nil
}

func (r *replayer) applyModelSwitch(rec Record) error {
	sw, err := decode[ModelSwitch](rec)
	if err != nil {
		return err
	}
	r.st.Current = sw.To
	return nil
}

func (r *replayer) applyRunEnd(rec Record) error {
	e, err := decode[RunEnd](rec)
	if err != nil {
		return err
	}
	r.st.Finished = true
	r.st.EndReason = e.Reason
	return nil
}

func AttachResult(msgs []provider.Message, tr provider.ToolResult) []provider.Message {
	return attachResult(msgs, tr)
}

func attachResult(msgs []provider.Message, tr provider.ToolResult) []provider.Message {
	n := len(msgs)
	if n > 0 {
		last := msgs[n-1]
		if last.Role == provider.RoleUser && last.Text == "" && len(last.ToolResults) > 0 {
			last.ToolResults = append(last.ToolResults, tr)
			msgs[n-1] = last
			return msgs
		}
	}
	return append(msgs, provider.Message{
		Role:        provider.RoleUser,
		ToolResults: []provider.ToolResult{tr},
	})
}

func pendingCalls(msgs []provider.Message, started, resultIDs map[string]bool) []Pending {
	var last *provider.Message
	for i := range msgs {
		if msgs[i].Role == provider.RoleAssistant {
			last = &msgs[i]
		}
	}
	if last == nil || len(last.ToolCalls) == 0 {
		return nil
	}
	var out []Pending
	for _, call := range last.ToolCalls {
		if resultIDs[call.ID] {
			continue
		}
		out = append(out, Pending{Call: call, Started: started[call.ID]})
	}
	return out
}
