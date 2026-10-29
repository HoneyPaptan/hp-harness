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
	if code, done := globalFlags(env, args); done {
		return code
	}
	return dispatch(ctx, env, args)
}

func globalFlags(env Env, args []string) (int, bool) {
	if args[0] == "--version" || args[0] == "-version" {
		printVersion(env)
		return 0, true
	}
	if isHelp(args[0]) && len(args) == 1 {
		printUsage(env.Stderr)
		return 0, true
	}
	return 0, false
}

func dispatch(ctx context.Context, env Env, args []string) int {
	switch args[0] {
	case "run":
		return cmdRun(ctx, env, args[1:])
	case "resume":
		return cmdResume(ctx, env, args[1:])
	case "runs":
		return cmdRuns(env, args[1:])
	case "stop":
		return cmdStop(env, args[1:])
	case "version":
		return cmdVersion(env, args[1:])
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
	fmt.Fprintln(w, "usage: hp <run|resume|attach|stop|runs|version> ...")
}
