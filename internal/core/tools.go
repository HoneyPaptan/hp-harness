package core

import (
	"context"
	"fmt"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func (r *runner) turnTools(ctx context.Context) []provider.ToolDef {
	defs := r.deps.Tools.Defs()
	if r.deps.ToolChooser == nil {
		return defs
	}
	if chosen := r.deps.ToolChooser.Choose(ctx, r.msgs, defs); len(chosen) > 0 {
		return chosen
	}
	return defs
}

func (r *runner) runTools(ctx context.Context, calls []provider.ToolCall) ([]provider.ToolResult, error) {
	env := tool.Env{Host: r.deps.Host, Dir: r.cfg.Dir}
	out := make([]provider.ToolResult, 0, len(calls))
	for _, call := range calls {
		tr, err := r.execCall(ctx, env, call)
		if err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, nil
}

func (r *runner) execCall(ctx context.Context, env tool.Env, call provider.ToolCall) (provider.ToolResult, error) {
	if err := r.w.ToolStarted(call); err != nil {
		return provider.ToolResult{}, err
	}
	r.deps.Observer.emitToolStart(call)
	tr, err := r.invoke(ctx, env, call)
	if err != nil {
		return provider.ToolResult{}, err
	}
	if err := r.w.ToolResult(tr); err != nil {
		return provider.ToolResult{}, err
	}
	r.deps.Observer.emitToolResult(tr)
	return tr, nil
}

func (r *runner) invoke(ctx context.Context, env tool.Env, call provider.ToolCall) (provider.ToolResult, error) {
	t, ok := r.deps.Tools.Get(call.Name)
	if !ok {
		text := fmt.Sprintf("unknown tool: %s", call.Name)
		return provider.ToolResult{CallID: call.ID, Text: text, IsError: true}, nil
	}
	res, err := t.Run(ctx, env, call.Args)
	if err != nil {
		return provider.ToolResult{}, err
	}
	return provider.ToolResult{CallID: call.ID, Text: res.Text, IsError: res.IsError}, nil
}

func appendResults(msgs []provider.Message, results []provider.ToolResult) []provider.Message {
	for _, tr := range results {
		msgs = journal.AttachResult(msgs, tr)
	}
	return msgs
}
