package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

func cmdResume(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage(env, err)
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return exitUsage(env, usagef("run id required"))
	}
	return resumeRun(ctx, env, rest[0])
}

func resumeRun(ctx context.Context, env Env, id string) int {
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	start, err := journal.ReadStart(env.Host, jpath)
	if err != nil {
		return exitUsage(env, usagef("run %q not found", id))
	}
	ep, err := endpointFromStart(start, env.Getenv)
	if err != nil {
		return exitUsage(env, err)
	}
	out, pr, err := execResume(ctx, env, ep, start, jpath, id)
	return finishResume(env, id, pr, out, err)
}

func endpointFromStart(st journal.RunStart, getenv func(string) string) (Endpoint, error) {
	ep := Endpoint{Preset: st.Preset, Model: st.Model, BaseURL: st.BaseURL, KeyEnv: st.KeyEnv}
	return applyKey(ep, getenv)
}

func execResume(ctx context.Context, env Env, ep Endpoint, start journal.RunStart, jpath, id string) (core.Outcome, *printer, error) {
	skills, err := discoverSkills(env, start.Dir)
	if err != nil {
		return core.Outcome{}, nil, err
	}
	pr := newPrinter(env, id)
	deps := core.Deps{
		Host: env.Host, Provider: env.NewProvider(ep), Tools: buildRegistry(skills),
		Observer: pr.observer(),
	}
	out, err := core.Resume(ctx, deps, jpath)
	return out, pr, err
}

func finishResume(env Env, id string, pr *printer, out core.Outcome, err error) int {
	if errors.Is(err, core.ErrFinished) {
		fmt.Fprintf(env.Stderr, "run %s already finished\n", id)
		return 1
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	pr.summary(out)
	if out.Reason == core.ReasonStepLimit {
		return 3
	}
	return 0
}
