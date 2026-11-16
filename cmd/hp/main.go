package main

import (
	"context"
	"crypto/rand"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/HoneyPaptan/hp-harness/internal/cli"
	"github.com/HoneyPaptan/hp-harness/internal/crew"
	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/anthropic"
	"github.com/HoneyPaptan/hp-harness/internal/provider/openai"
	"github.com/HoneyPaptan/hp-harness/internal/provider/transport"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	detached := os.Getenv("HP_DETACHED") == "1"
	if detached {
		signal.Ignore(syscall.SIGHUP)
		_ = os.Unsetenv("HP_DETACHED")
	}
	ctx, stop := commandContext(os.Args[1:])
	defer stop()
	env, err := buildEnv()
	if err != nil {
		return 1
	}
	if detached {
		env.Getenv = detachedGetenv
	}
	privateRunsDir(env.Getenv)
	if len(env.Args) > 0 && (env.Args[0] == "crew" || env.Args[0] == "crews") {
		return crew.Main(ctx, crewEnv(env), env.Args)
	}
	return cli.Main(ctx, env)
}

func crewEnv(e cli.Env) crew.Env {
	return crew.Env{
		Args: e.Args, Getenv: e.Getenv, Cwd: e.Cwd,
		Stdout: e.Stdout, Stderr: e.Stderr, Exe: e.Exe,
		Alive: processAlive, Spawn: spawnDetached, Pid: os.Getpid(),
		Signal: processSignal, Handover: terminal.Handover,
	}
}

func buildEnv() (cli.Env, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return cli.Env{}, err
	}
	exe, _ := os.Executable()
	env := cli.Env{
		Args: os.Args[1:], Getenv: os.Getenv, Cwd: cwd,
		Stdout: os.Stdout, Stderr: os.Stderr, Host: host.Native{},
		Rand: rand.Reader, Version: version, NewProvider: cli.SharedProviders(newProvider), NewJev: newJev,
		Spawn: spawnDetached, Alive: processAlive, ProcStart: processStart, Signal: processSignal, Pid: os.Getpid(),
		Lock: host.Native{}.Lock, Exe: exe, Os: runtime.GOOS,
		Term: terminal, RunCtl: runCtl,
	}
	env.LoginEnv = memoLoginEnv(&env)
	env.Crew = func(dir string) cli.CrewService {
		ce := crewEnv(env)
		ce.Cwd = dir
		return crew.Service{Env: ce}
	}
	return env, nil
}

func newJev(getenv func(string) string) (*jev.Client, error) {
	c, err := jev.FromEnv(getenv)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func newProvider(ep cli.Endpoint) provider.Provider {
	if ep.Preset == "anthropic" {
		return &anthropic.Client{BaseURL: ep.BaseURL, APIKey: ep.APIKey, HTTP: transport.NewHTTPClient()}
	}
	return &openai.Client{
		BaseURL: ep.BaseURL, APIKey: ep.APIKey, HTTP: transport.NewHTTPClient(),
		DisableThinking: ep.Preset == "deepseek", NoStreamOptions: ep.Preset == "mistral",
		NestedReasoning: ep.Preset == "openrouter", Headers: appHeaders(ep.Preset),
		UserAgent: "hp/" + version,
	}
}

func commandContext(args []string) (context.Context, func()) {
	if !needsSignals(args) {
		ctx, cancel := context.WithCancelCause(context.Background())
		return ctx, func() { cancel(nil) }
	}
	return signalContext()
}

func needsSignals(args []string) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "--version", "-version", "version", "runs", "providers", "list", "remove", "permissions", "stop", "help", "-h", "--help":
		return false
	}
	return true
}

func signalContext() (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig, ok := <-ch
		if !ok {
			return
		}
		if sig == syscall.SIGTERM {
			cancel(cli.ErrTerminated)
			return
		}
		cancel(context.Canceled)
	}()
	return ctx, func() {
		signal.Stop(ch)
		close(ch)
		cancel(nil)
	}
}

func detachedGetenv(key string) string {
	if key == "HP_DETACHED" {
		return "1"
	}
	return os.Getenv(key)
}

func privateRunsDir(getenv func(string) string) {
	dir := cli.RunsDir(getenv)
	if err := os.MkdirAll(dir, 0o700); err == nil {
		_ = os.Chmod(dir, 0o700)
	}
}

func appHeaders(preset string) map[string]string {
	if preset == "openrouter" {
		return openai.OpenRouterApp
	}
	return nil
}
