package cli

import (
	"context"
	"fmt"
	"io"
)

func Main(ctx context.Context, env Env) int {
	args := env.Args
	if len(args) == 0 {
		printUsage(env.Stderr)
		return 2
	}
	if args[0] == "--version" || args[0] == "-version" {
		printVersion(env)
		return 0
	}
	if isHelp(args[0]) && len(args) == 1 {
		printUsage(env.Stderr)
		return 0
	}
	switch args[0] {
	case "run":
		return cmdRun(ctx, env, args[1:])
	case "help", "-h", "--help":
		printUsage(env.Stderr)
		return 0
	default:
		printUsage(env.Stderr)
		return 2
	}
}

func isHelp(s string) bool {
	return s == "help" || s == "-h" || s == "--help"
}

func printVersion(env Env) {
	fmt.Fprintf(env.Stdout, "hp %s\n", env.Version)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: hp run [-m preset/model] [flags] \"prompt\"")
}
