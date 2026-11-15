package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/permit"
	"github.com/HoneyPaptan/hp-harness/internal/skill"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func cmdRun(ctx context.Context, env Env, args []string) int {
	opts, rest, err := parseRunFlags(env, args)
	if err != nil {
		return exitUsage(env, err)
	}
	prompt := strings.Join(rest, " ")
	if strings.TrimSpace(prompt) == "" {
		return exitUsage(env, usagef("prompt required"))
	}
	if opts.detach {
		return detachRun(ctx, env, opts, prompt)
	}
	return runAgent(ctx, env, opts, prompt)
}

type runOpts struct {
	model, dir, baseURL, keyEnv string
	steps, maxTokens            int
	fallbacks                   []string
	switchAfter, maxWait        time.Duration
	detach, jev, fullAccess     bool
	effort                      string
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func parseRunFlags(env Env, args []string) (runOpts, []string, error) {
	fs, o, fb := runFlagSet(env, "run")
	if err := fs.Parse(args); err != nil {
		return runOpts{}, nil, err
	}
	o.fallbacks = append([]string(nil), *fb...)
	return *o, fs.Args(), nil
}

func runFlagSet(env Env, name string) (*flag.FlagSet, *runOpts, *stringList) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	o := &runOpts{}
	fb := &stringList{}
	fs.StringVar(&o.model, "m", "", "preset/model")
	fs.StringVar(&o.dir, "C", "", "working directory")
	fs.IntVar(&o.steps, "steps", 0, "step limit")
	fs.IntVar(&o.maxTokens, "max-tokens", 0, "max tokens")
	fs.StringVar(&o.baseURL, "base-url", "", "API base URL")
	fs.StringVar(&o.keyEnv, "key-env", "", "API key env var")
	fs.Var(fb, "fallback", "fallback preset/model")
	fs.DurationVar(&o.switchAfter, "switch-after", 20*time.Second, "switch after limit wait")
	fs.DurationVar(&o.maxWait, "max-wait", 6*time.Hour, "max limit wait")
	fs.BoolVar(&o.detach, "d", false, "detach")
	fs.BoolVar(&o.jev, "jev", env.Getenv("HP_JEV") == "1", "use Jev for search and tool choice")
	fs.BoolVar(&o.fullAccess, "full-access", false, "skip permission checks")
	return fs, o, fb
}

func runAgent(ctx context.Context, env Env, o runOpts, prompt string) int {
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
	if env.LoginEnv != nil {
		env.LoginEnv(dir)
	}
	prompt = expandMentions(env.Host, dir, prompt)
	id, err := newRunID(env.Host.Now(), env.Rand)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	out, pr, err := execRun(ctx, env, ep, bySpec, client, dir, id, jpath, o, prompt)
	return finishRun(ctx, env, id, jpath, pr, out, err)
}

type runPerm struct {
	mode   func() permit.Mode
	rules  []permit.Rule
	grants *permit.Grants
	dir    string
}

func execRun(ctx context.Context, env Env, ep Endpoint, bySpec map[string]Endpoint, client *jev.Client, dir, id, jpath string, o runOpts, prompt string) (core.Outcome, *printer, error) {
	pr := newPrinter(env, id, client)
	skills, err := discoverSkills(env, dir)
	if err != nil {
		return core.Outcome{}, pr, err
	}
	mode, rules, grants, _, st, err := effectivePermission(env, o.fullAccess, "")
	if err != nil {
		return core.Outcome{}, pr, err
	}
	cfg := runConfig(id, ep, dir, jpath, o, prompt, skills, env, modeString(mode), st)
	tools := openToolset(env, dir, skills, client, st, nil)
	defer tools.close()
	rp := runPerm{
		mode:  func() permit.Mode { return mode },
		rules: rules, grants: grants, dir: dir,
	}
	deps := runDeps(env, ep, bySpec, tools.reg, pr.observer(), client, rp, st)
	out, err := core.Run(ctx, deps, cfg)
	return out, pr, err
}

func loginEnvExec(env Env, dir string) func() []string {
	if env.LoginEnv == nil {
		return nil
	}
	return env.LoginEnv(dir)
}

func runDeps(env Env, ep Endpoint, bySpec map[string]Endpoint, tools *tool.Registry, obs core.Observer, client *jev.Client, rp runPerm, st settings) core.Deps {
	d := core.Deps{
		Host: env.Host, Provider: env.NewProvider(ep), Tools: tools,
		ToolChooser: jevChooser(client), Observer: obs,
		Gate:    newGate(env.Host, rp.mode, rp.rules, rp.grants, rp.dir, env.Getenv("HOME"), nil, newReviewer(env, func() Endpoint { return ep })),
		ExecEnv: loginEnvExec(env, rp.dir),
	}
	d.ContextWindow = contextWindow(st, endpointSpec(ep), ep.Model)
	d.CompactPercent = compactPercent(st)
	d.Providers = limitProviders(env, bySpec)
	return d
}

func runConfig(id string, ep Endpoint, dir, jpath string, o runOpts, prompt string, skills []skill.Skill, env Env, perm string, st settings) core.Config {
	return core.Config{
		RunID: id, Model: ep.Model, System: buildSystem(env, dir, skills, st),
		Dir: dir, JournalPath: jpath, Prompt: prompt, StepLimit: o.steps,
		MaxTokens: o.maxTokens, Preset: ep.Preset, BaseURL: ep.BaseURL, KeyEnv: ep.KeyEnv,
		Fallbacks: append([]string(nil), o.fallbacks...), SwitchAfter: o.switchAfter, MaxWait: o.maxWait,
		Detached: o.detach, Jev: o.jev, Permission: perm, Effort: o.effort,
	}
}

func finishRun(ctx context.Context, env Env, id, jpath string, pr *printer, out core.Outcome, err error) int {
	if err == nil {
		pr.summary(out)
		if out.Reason == core.ReasonStepLimit {
			return 3
		}
		return 0
	}
	pr.ensureOutNL()
	if isInterrupt(ctx, err) {
		if !errors.Is(context.Cause(ctx), ErrTerminated) {
			markUserStop(env, jpath)
		}
		fmt.Fprintf(env.Stderr, "interrupted, resume with: hp resume %s\n", id)
		return 130
	}
	if errors.Is(err, core.ErrLimitTooLong) {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		fmt.Fprintf(env.Stderr, "resume with: hp resume %s\n", id)
		return 4
	}
	fmt.Fprintf(env.Stderr, "error: %v\n", err)
	if journalExists(env, jpath) {
		fmt.Fprintf(env.Stderr, "resume with: hp resume %s\n", id)
	}
	return 1
}

var ErrTerminated = errors.New("terminated")

func isInterrupt(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	return ctx.Err() != nil
}

func markUserStop(env Env, jpath string) {
	if err := journal.MarkStop(env.Host, jpath, "user"); err != nil {
		fmt.Fprintf(env.Stderr, "note: %v\n", err)
	}
}

func journalExists(env Env, path string) bool {
	_, err := env.Host.Stat(path)
	return err == nil
}

func exitUsage(env Env, err error) int {
	fmt.Fprintf(env.Stderr, "error: %v\n", err)
	return 2
}

func exitErr(env Env, id, jpath string, err error) int {
	fmt.Fprintf(env.Stderr, "error: %v\n", err)
	if jpath != "" && journalExists(env, jpath) {
		fmt.Fprintf(env.Stderr, "resume with: hp resume %s\n", id)
	}
	return 1
}
