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
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/openai"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
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
		Rand: rand.Reader, Version: version, NewProvider: newProvider,
	}, nil
}

func newProvider(ep cli.Endpoint) provider.Provider {
	return &openai.Client{
		BaseURL: ep.BaseURL, APIKey: ep.APIKey, HTTP: &http.Client{},
	}
}
