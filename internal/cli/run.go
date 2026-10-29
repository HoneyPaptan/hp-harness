package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/core"
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
	return runAgent(ctx, env, opts, prompt)
}

type runOpts struct {
	model, dir, baseURL, keyEnv string
	steps, maxTokens            int
}

func parseRunFlags(env Env, args []string) (runOpts, []string, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var o runOpts
	fs.StringVar(&o.model, "m", "", "preset/model")
	fs.StringVar(&o.dir, "C", "", "working directory")
	fs.IntVar(&o.steps, "steps", 0, "step limit")
	fs.IntVar(&o.maxTokens, "max-tokens", 0, "max tokens")
	fs.StringVar(&o.baseURL, "base-url", "", "API base URL")
	fs.StringVar(&o.keyEnv, "key-env", "", "API key env var")
	if err := fs.Parse(args); err != nil {
		return runOpts{}, nil, err
	}
	return o, fs.Args(), nil
}

func runAgent(ctx context.Context, env Env, o runOpts, prompt string) int {
	ep, err := resolveEndpoint(o.model, o.baseURL, o.keyEnv, env.Getenv)
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
	out, pr, err := execRun(ctx, env, ep, dir, id, jpath, o, prompt)
	return finishRun(ctx, env, id, jpath, pr, out, err)
}

func execRun(ctx context.Context, env Env, ep Endpoint, dir, id, jpath string, o runOpts, prompt string) (core.Outcome, *printer, error) {
	skills, err := discoverSkills(env, dir)
	if err != nil {
		return core.Outcome{}, nil, err
	}
	pr := newPrinter(env, id)
	cfg := runConfig(id, ep, dir, jpath, o, prompt, skills, env)
	deps := core.Deps{
		Host: env.Host, Provider: env.NewProvider(ep), Tools: buildRegistry(skills),
		Observer: pr.observer(),
	}
	out, err := core.Run(ctx, deps, cfg)
	return out, pr, err
}

func runConfig(id string, ep Endpoint, dir, jpath string, o runOpts, prompt string, skills []skill.Skill, env Env) core.Config {
	return core.Config{
		RunID: id, Model: ep.Model, System: buildSystem(env, dir, skills),
		Dir: dir, JournalPath: jpath, Prompt: prompt, StepLimit: o.steps,
		MaxTokens: o.maxTokens, Preset: ep.Preset, BaseURL: ep.BaseURL, KeyEnv: ep.KeyEnv,
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
	if isInterrupt(ctx, err) {
		fmt.Fprintf(env.Stderr, "interrupted, resume with: hp resume %s\n", id)
		return 130
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
