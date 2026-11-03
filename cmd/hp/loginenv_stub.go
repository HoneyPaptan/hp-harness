//go:build !unix

package main

import "github.com/HoneyPaptan/hp-harness/internal/cli"

func memoLoginEnv(*cli.Env) func(string) func() []string {
	return nil
}
