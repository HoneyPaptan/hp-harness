package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/permit"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type askAnswer int

const (
	askYes askAnswer = iota
	askAlways
	askNo
)

type askFunc func(ctx context.Context, action permit.Action) askAnswer

type gateReview func(ctx context.Context, action permit.Action, msgs []provider.Message) core.Verdict

func defaultReview(context.Context, permit.Action, []provider.Message) core.Verdict {
	return core.Verdict{Allow: false, Reason: "needs review"}
}

func newGate(h host.Host, mode func() permit.Mode, rules []permit.Rule, grants *permit.Grants, dir, home string, ask askFunc, review gateReview) func(context.Context, provider.ToolCall, []provider.Message) core.Verdict {
	if review == nil {
		review = defaultReview
	}
	return func(ctx context.Context, call provider.ToolCall, msgs []provider.Message) core.Verdict {
		a := permit.Describe(call, dir)
		if a.Kind == permit.KindWrite && writeEscapes(h, dir, call) {
			a.Linked = true
		}
		v, reason := permit.Decide(mode(), rules, grants, a, dir, home)
		switch v {
		case permit.Allow:
			return core.Verdict{Allow: true}
		case permit.Deny:
			return core.Verdict{Allow: false, Reason: reason}
		case permit.Ask:
			if ask == nil {
				return core.Verdict{Allow: false, Reason: "needs approval: run it in a session or use --full-access"}
			}
			switch ask(ctx, a) {
			case askYes:
				return core.Verdict{Allow: true}
			case askAlways:
				grants.Add(a)
				return core.Verdict{Allow: true}
			case askNo:
				return core.Verdict{Allow: false, Reason: "declined by the user"}
			default:
				return core.Verdict{Allow: false, Reason: "declined by the user"}
			}
		case permit.Review:
			return review(ctx, a, msgs)
		default:
			return core.Verdict{Allow: false, Reason: reason}
		}
	}
}

func writeEscapes(h host.Host, dir string, call provider.ToolCall) bool {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return true
	}
	rel := args.Path
	if filepath.IsAbs(rel) {
		r, err := filepath.Rel(dir, rel)
		if err != nil {
			return true
		}
		rel = r
	}
	if slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "..") {
		return true
	}
	return pathHasSymlink(h, dir, rel)
}
