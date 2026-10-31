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
	"github.com/HoneyPaptan/hp-harness/internal/skill"
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
	detach, jev                 bool
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
	id, err := newRunID(env.Host.Now(), env.Rand)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	out, pr, err := execRun(ctx, env, ep, bySpec, client, dir, id, jpath, o, prompt)
	return finishRun(ctx, env, id, jpath, pr, out, err)
}

func execRun(ctx context.Context, env Env, ep Endpoint, bySpec map[string]Endpoint, client *jev.Client, dir, id, jpath string, o runOpts, prompt string) (core.Outcome, *printer, error) {
	skills, err := discoverSkills(env, dir)
	if err != nil {
		return core.Outcome{}, nil, err
	}
	pr := newPrinter(env, id, client)
	cfg := runConfig(id, ep, dir, jpath, o, prompt, skills, env)
	deps := runDeps(env, ep, bySpec, skills, pr.observer(), client)
	out, err := core.Run(ctx, deps, cfg)
	return out, pr, err
}

func runDeps(env Env, ep Endpoint, bySpec map[string]Endpoint, skills []skill.Skill, obs core.Observer, client *jev.Client) core.Deps {
	d := core.Deps{
		Host: env.Host, Provider: env.NewProvider(ep), Tools: buildRegistry(skills, jevTools(client)),
		ToolChooser: jevChooser(client), Observer: obs,
	}
	d.Providers = limitProviders(env, bySpec)
	return d
}

func runConfig(id string, ep Endpoint, dir, jpath string, o runOpts, prompt string, skills []skill.Skill, env Env) core.Config {
	return core.Config{
		RunID: id, Model: ep.Model, System: buildSystem(env, dir, skills),
		Dir: dir, JournalPath: jpath, Prompt: prompt, StepLimit: o.steps,
		MaxTokens: o.maxTokens, Preset: ep.Preset, BaseURL: ep.BaseURL, KeyEnv: ep.KeyEnv,
		Fallbacks: append([]string(nil), o.fallbacks...), SwitchAfter: o.switchAfter, MaxWait: o.maxWait,
		Jev: o.jev,
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

func isInterrupt(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	return ctx.Err() != nil
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
