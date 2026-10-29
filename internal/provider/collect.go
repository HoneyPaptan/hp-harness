package provider

import (
	"errors"
	"iter"
)

var ErrNoDone = errors.New("stream ended without done event")

func Collect(seq iter.Seq2[Event, error]) (Message, Usage, error) {
	var msg Message
	msg.Role = RoleAssistant
	var usage Usage
	var done bool

	for ev, err := range seq {
		if err != nil {
			return Message{}, Usage{}, err
		}
		switch ev.Kind {
		case EventText:
			msg.Text += ev.Text
		case EventToolCall:
			msg.ToolCalls = append(msg.ToolCalls, ev.Call)
		case EventDone:
			usage = ev.Usage
			done = true
		}
	}
	if !done {
		return Message{}, Usage{}, ErrNoDone
	}
	return msg, usage, nil
}
