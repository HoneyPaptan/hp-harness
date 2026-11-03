package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/permit"
)

func cmdResume(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var detach bool
	fs.BoolVar(&detach, "d", false, "detach")
	if err := fs.Parse(args); err != nil {
		return exitUsage(env, err)
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return exitUsage(env, usagef("run id required"))
	}
	id := rest[0]
	if !safeRunID(id) {
		return exitUsage(env, usagef("invalid run id %q", id))
	}
	if detach {
		jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
		st, err := replayRun(env, jpath)
		if err != nil {
			return exitUsage(env, usagef("run %q not found", id))
		}
		if _, err := openJev(env, st.Jev); err != nil {
			return exitUsage(env, err)
		}
		return spawnDetached(env, id)
	}
	return resumeRun(ctx, env, id)
}

func resumeRun(ctx context.Context, env Env, id string) int {
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	st, err := replayRun(env, jpath)
	if err != nil {
		return exitUsage(env, usagef("run %q not found", id))
	}
	if env.LoginEnv != nil {
		env.LoginEnv(st.Start.Dir)
	}
	cleanup, code := beginResume(env, id)
	if code >= 0 {
		return code
	}
	defer cleanup()
	markDetachedResume(env, jpath)
	ep, bySpec, err := resolveResumeSpecs(st.Start, env.Getenv)
	if err != nil {
		return exitUsage(env, err)
	}
	if err := bindCurrent(env, jpath, ep, bySpec); err != nil {
		return exitUsage(env, err)
	}
	client, err := openJev(env, st.Jev)
	if err != nil {
		return exitUsage(env, err)
	}
	out, pr, err := execResume(ctx, env, ep, bySpec, client, st, jpath, id)
	return finishResume(ctx, env, id, jpath, pr, out, err)
}

func endpointFromStart(st journal.RunStart, getenv func(string) string) (Endpoint, error) {
	ep := Endpoint{Preset: st.Preset, Model: st.Model, BaseURL: st.BaseURL, KeyEnv: st.KeyEnv}
	return applyKey(ep, getenv)
}

func execResume(ctx context.Context, env Env, ep Endpoint, bySpec map[string]Endpoint, client *jev.Client, st journal.State, jpath, id string) (core.Outcome, *printer, error) {
	start := st.Start
	skills, err := discoverSkills(env, start.Dir)
	if err != nil {
		return core.Outcome{}, nil, err
	}
	mode, rules, grants, _, _, err := effectivePermission(env, false, st.Permission)
	if err != nil {
		return core.Outcome{}, nil, err
	}
	pr := newPrinter(env, id, client)
	tools := openToolset(env, skills, client)
	defer tools.close()
	rp := runPerm{
		mode:  func() permit.Mode { return mode },
		rules: rules, grants: grants, dir: start.Dir,
	}
	deps := runDeps(env, ep, bySpec, tools.reg, pr.observer(), client, rp)
	out, err := core.Resume(ctx, deps, jpath)
	return out, pr, err
}

func finishResume(ctx context.Context, env Env, id, jpath string, pr *printer, out core.Outcome, err error) int {
	if errors.Is(err, core.ErrFinished) {
		fmt.Fprintf(env.Stderr, "run %s already finished\n", id)
		return 1
	}
	if pr == nil {
		return exitErr(env, id, jpath, err)
	}
	return finishRun(ctx, env, id, jpath, pr, out, err)
}
