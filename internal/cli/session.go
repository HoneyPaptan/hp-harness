package cli

import (
	"context"
	"errors"
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
		fmt.Fprintln(env.Stderr, "hp needs a terminal for a session, use hp run PROMPT in a script")
		return 2
	}
	o, cont, pick, err := parseSessionFlags(env, args)
	if err != nil {
		return exitUsage(env, err)
	}
	if cont || pick {
		return continueSession(ctx, env, o, pick)
	}
	return freshSession(ctx, env, o)
}

func parseSessionFlags(env Env, args []string) (runOpts, bool, bool, error) {
	fs, o, fb := runFlagSet(env, "hp")
	var cont, pick bool
	fs.BoolVar(&cont, "c", false, "continue the latest run in this directory")
	fs.BoolVar(&pick, "r", false, "choose one of the recent runs in this directory to continue")
	if err := fs.Parse(args); err != nil {
		return runOpts{}, false, false, err
	}
	if len(fs.Args()) != 0 {
		return runOpts{}, false, false, usagef("a session takes no prompt, use hp run")
	}
	if o.detach {
		return runOpts{}, false, false, usagef("a session cannot detach, use hp run -d")
	}
	if cont && pick {
		return runOpts{}, false, false, usagef("use -c or -r, not both")
	}
	o.fallbacks = append([]string(nil), *fb...)
	return *o, cont, pick, nil
}

func freshSession(ctx context.Context, env Env, o runOpts) int {
	o, skipped := applyLastModel(env, o)
	noModel := skipped != "" || (strings.TrimSpace(o.model) == "" && strings.TrimSpace(env.Getenv("HP_MODEL")) == "")
	if noModel {
		if len(o.fallbacks) > 0 {
			return exitUsage(env, usagef("fallback requires a primary model"))
		}
		return startEmptySession(ctx, env, o, skipped)
	}
	ep, bySpec, err := resolveRunSpecs(o, env.Getenv)
	if err != nil {
		return exitUsage(env, err)
	}
	return openFreshSession(ctx, env, o, ep, bySpec, false, "")
}

func startEmptySession(ctx context.Context, env Env, o runOpts, intro string) int {
	bySpec := make(map[string]Endpoint)
	return openFreshSession(ctx, env, o, Endpoint{}, bySpec, true, intro)
}

func openFreshSession(ctx context.Context, env Env, o runOpts, ep Endpoint, bySpec map[string]Endpoint, pickModel bool, intro string) int {
	client, err := openJev(env, o.jev)
	if err != nil {
		return exitUsage(env, err)
	}
	dir, err := resolveWorkDir(env, o.dir)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	if env.LoginEnv != nil {
		env.LoginEnv(dir)
	}
	a, err := newSessionAgent(env, o, ep, bySpec, client, dir, nil)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	defer a.release()
	return runSession(ctx, env, tui.Config{Agent: a, Intro: intro, PickModel: pickModel})
}

func continueSession(ctx context.Context, env Env, o runOpts, pick bool) int {
	dir, err := resolveWorkDir(env, o.dir)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	ids, err := runsIn(env, dir, recentRunLimit)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	if len(ids) == 0 {
		return exitUsage(env, usagef("no run to continue in %s", dir))
	}
	if env.LoginEnv != nil {
		env.LoginEnv(dir)
	}
	if pick && len(ids) > 1 {
		return pickSession(ctx, env, o, ids)
	}
	a, unfinished, err := openRun(env, o, ids[0])
	if err != nil {
		return exitOpen(env, err)
	}
	defer a.release()
	return runSession(ctx, env, tui.Config{Agent: a, Resume: unfinished, Intro: "continuing " + ids[0]})
}

func pickSession(ctx context.Context, env Env, o runOpts, ids []string) int {
	var opened *sessionAgent
	defer func() {
		if opened != nil {
			opened.release()
		}
	}()
	open := func(id string) (tui.Agent, bool, string, error) {
		a, unfinished, err := openRun(env, o, id)
		if err != nil {
			return nil, false, "", err
		}
		opened = a
		return a, unfinished, "continuing " + id, nil
	}
	return runSession(ctx, env, tui.Config{RunChoices: runChoices(env, ids), OpenRun: open})
}

func exitOpen(env Env, err error) int {
	if errors.As(err, new(usageError)) {
		return exitUsage(env, err)
	}
	return exitErr(env, "", "", err)
}

func openRun(env Env, o runOpts, id string) (*sessionAgent, bool, error) {
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	st, err := replayRun(env, jpath)
	if err != nil {
		return nil, false, err
	}
	w, err := resolveStored(env, st, jpath)
	if err != nil {
		return nil, false, usagef("%v", err)
	}
	a, err := restoredAgent(env, st, w.bySpec, w.ep, w.client, id, jpath)
	if err != nil {
		return nil, false, err
	}
	if _, err := claimRun(env, id); err != nil {
		a.release()
		return nil, false, err
	}
	if o.model != "" {
		if err := a.useModel(o.model); err != nil {
			a.release()
			return nil, false, usagef("%v", err)
		}
	}
	return a, !st.Finished, nil
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
	client, err := openJev(env, st.Jev)
	if err != nil {
		return storedWiring{}, err
	}
	return storedWiring{ep: ep, bySpec: bySpec, client: client}, nil
}

func runSession(ctx context.Context, env Env, cfg tui.Config) int {
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
	cfg.In, cfg.Out, cfg.Width, cfg.Color = t, t, t.Width, colorOn(env.Getenv)
	cfg.Header = sessionHeader(env.Version)
	tui.NewSession(cfg).Run(ctx)
	if ctx.Err() != nil {
		return 130
	}
	return 0
}

func sessionHeader(version string) string {
	return "hp " + version + " · /help for commands"
}

func restoreTerminal(t Terminal, restore func() error) {
	_ = tui.DisableBracketedPaste(t)
	_ = restore()
}

func colorOn(getenv func(string) string) bool {
	return getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
}
