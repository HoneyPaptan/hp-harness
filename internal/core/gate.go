package core

import (
	"context"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type Verdict struct {
	Allow  bool
	Reason string
}

func heldResult(callID, reason string) provider.ToolResult {
	text := "held: not allowed"
	if reason != "" {
		text = "held: " + reason
	}
	return provider.ToolResult{CallID: callID, Text: text, IsError: true}
}

func (r *runner) gateCall(ctx context.Context, call provider.ToolCall) (provider.ToolResult, bool, error) {
	if r.deps.Gate == nil {
		return provider.ToolResult{}, true, nil
	}
	v := r.deps.Gate(ctx, call, r.msgs)
	if err := ctx.Err(); err != nil {
		return provider.ToolResult{}, false, err
	}
	if v.Allow {
		return provider.ToolResult{}, true, nil
	}
	return heldResult(call.ID, v.Reason), false, nil
}
