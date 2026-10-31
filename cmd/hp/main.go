package main

import (
	"context"
	"crypto/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/HoneyPaptan/hp-harness/internal/cli"
	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/openai"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	if os.Getenv("HP_DETACHED") == "1" {
		signal.Ignore(syscall.SIGHUP)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	env, err := buildEnv()
	if err != nil {
		return 1
	}
	return cli.Main(ctx, env)
}

func buildEnv() (cli.Env, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return cli.Env{}, err
	}
	return cli.Env{
		Args: os.Args[1:], Getenv: os.Getenv, Cwd: cwd,
		Stdout: os.Stdout, Stderr: os.Stderr, Host: host.Native{},
		Rand: rand.Reader, Version: version, NewProvider: newProvider, NewJev: newJev,
		Spawn: spawnDetached, Alive: processAlive, Signal: processSignal, Pid: os.Getpid(),
		Term: stdTerminal{},
	}, nil
}

func newJev(getenv func(string) string) (*jev.Client, error) {
	c, err := jev.FromEnv(getenv)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func newProvider(ep cli.Endpoint) provider.Provider {
	return &openai.Client{
		BaseURL: ep.BaseURL, APIKey: ep.APIKey, HTTP: &http.Client{},
	}
}
