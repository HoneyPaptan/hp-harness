package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

func cmdSession(ctx context.Context, env Env, args []string) int {
	if env.Term == nil || !env.Term.Interactive() {
		printUsage(env.Stderr)
		return 2
	}
	o, cont, err := parseSessionFlags(env, args)
	if err != nil {
		return exitUsage(env, err)
	}
	if cont {
		return continueSession(ctx, env, o)
	}
	return freshSession(ctx, env, o)
}

func parseSessionFlags(env Env, args []string) (runOpts, bool, error) {
	fs, o, fb := runFlagSet(env, "hp")
	var cont bool
	fs.BoolVar(&cont, "c", false, "continue the latest run in this directory")
	if err := fs.Parse(args); err != nil {
		return runOpts{}, false, err
	}
	if len(fs.Args()) != 0 {
		return runOpts{}, false, usagef("a session takes no prompt, use hp run")
	}
	if o.detach {
		return runOpts{}, false, usagef("a session cannot detach, use hp run -d")
	}
	o.fallbacks = append([]string(nil), *fb...)
	return *o, cont, nil
}

func freshSession(ctx context.Context, env Env, o runOpts) int {
	if strings.TrimSpace(o.model) == "" && strings.TrimSpace(env.Getenv("HP_MODEL")) == "" {
		return exitUsage(env, usagef("no model set: use -m preset/model or set HP_MODEL"))
	}
	ep, bySpec, err := resolveRunSpecs(o, env.Getenv)
	if err != nil {
		return exitUsage(env, err)
	}
	client, err := openJev(env, o.jev)
	if err != nil {
		return exitUsage(env, err)
	}
	dir, err := resolveWorkDir(env, o.dir)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	a, err := newSessionAgent(env, o, ep, bySpec, client, dir)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	defer a.release()
	return runSession(ctx, env, a, false, "")
}

func continueSession(ctx context.Context, env Env, o runOpts) int {
	dir, err := resolveWorkDir(env, o.dir)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	id, err := latestRunIn(env, dir)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	if id == "" {
		return exitUsage(env, usagef("no run to continue in %s", dir))
	}
	a, unfinished, code := openRun(env, o, id)
	if code >= 0 {
		return code
	}
	defer a.release()
	if o.model != "" {
		if err := a.SetModel(o.model); err != nil {
			return exitUsage(env, err)
		}
	}
	return runSession(ctx, env, a, unfinished, "continuing "+id)
}

func openRun(env Env, o runOpts, id string) (*sessionAgent, bool, int) {
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	st, err := replayRun(env, jpath)
	if err != nil {
		return nil, false, exitErr(env, "", "", err)
	}
	w, err := resolveStored(env, st, jpath)
	if err != nil {
		return nil, false, exitUsage(env, err)
	}
	a, err := restoredAgent(env, st, w.bySpec, w.ep, w.client, id, jpath)
	if err != nil {
		return nil, false, exitErr(env, "", "", err)
	}
	if _, code := beginResume(env, id); code >= 0 {
		return nil, false, code
	}
	return a, !st.Finished, -1
}

type storedWiring struct {
	ep     Endpoint
	bySpec map[string]Endpoint
	client *jev.Client
}

func resolveStored(env Env, st journal.State, jpath string) (storedWiring, error) {
	ep, bySpec, err := resolveResumeSpecs(st.Start, env.Getenv)
	if err != nil {
		return storedWiring{}, err
	}
	if err := bindCurrent(env, jpath, ep, bySpec); err != nil {
		return storedWiring{}, err
	}
	client, err := openJev(env, st.Start.Jev)
	if err != nil {
		return storedWiring{}, err
	}
	return storedWiring{ep: ep, bySpec: bySpec, client: client}, nil
}

func runSession(ctx context.Context, env Env, a tui.Agent, resume bool, intro string) int {
	t := env.Term
	ctx, stop := t.Signals(ctx)
	defer stop()
	restore, err := t.MakeRaw()
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	defer restoreTerminal(t, restore)
	_ = tui.EnableBracketedPaste(t)
	tui.NewSession(tui.Config{
		In: t, Out: t, Width: t.Width, Color: colorOn(env.Getenv),
		Agent: a, Resume: resume, Intro: intro,
	}).Run(ctx)
	if ctx.Err() != nil {
		return 130
	}
	return 0
}

func restoreTerminal(t Terminal, restore func() error) {
	_ = tui.DisableBracketedPaste(t)
	_ = restore()
}

func colorOn(getenv func(string) string) bool {
	return getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
}
