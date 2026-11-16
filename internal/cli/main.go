package cli

import (
	"context"
	"fmt"
	"strings"
)

func Main(ctx context.Context, env Env) int {
	args := env.Args
	if len(args) > 0 {
		if code, done := globalFlags(env, args); done {
			return code
		}
	}
	env = applyKeyFile(env)
	if len(args) == 0 {
		return cmdSession(ctx, env, args)
	}
	return dispatch(ctx, env, args)
}

func globalFlags(env Env, args []string) (int, bool) {
	if args[0] == "--version" || args[0] == "-version" {
		printVersion(env)
		return 0, true
	}
	if helpRequested(args) {
		printHelp(env.Stderr)
		return 0, true
	}
	return 0, false
}

func dispatch(ctx context.Context, env Env, args []string) int {
	if len(args) > 1 && isHelp(args[1]) && !parsesFlags(args[0]) {
		printCommandHelp(env.Stderr, args[0])
		return 0
	}
	switch args[0] {
	case "run":
		return cmdRun(ctx, env, args[1:])
	case "resume":
		return cmdResume(ctx, env, args[1:])
	case "runs":
		return cmdRuns(env, args[1:])
	case "providers":
		return cmdProviders(env, args[1:])
	case "attach":
		return cmdAttach(ctx, env, args[1:])
	case "stop":
		return cmdStop(env, args[1:])
	case "recover":
		return cmdRecover(ctx, env, args[1:])
	case "autostart":
		return cmdAutostart(env, args[1:])
	case "web":
		return cmdWeb(ctx, env, args[1:])
	case "add":
		return cmdAdd(ctx, env, args[1:])
	case "list":
		return cmdList(env, args[1:])
	case "remove":
		return cmdRemove(env, args[1:])
	case "key":
		return cmdKey(env, args[1:])
	case "permissions":
		return cmdPermissions(env, args[1:])
	case "doctor":
		return cmdDoctor(ctx, env, args[1:])
	case "version":
		return cmdVersion(env, args[1:])
	case "help", "-h", "--help":
		printHelp(env.Stderr)
		return 0
	default:
		return unknownCommand(ctx, env, args)
	}
}

func unknownCommand(ctx context.Context, env Env, args []string) int {
	if strings.HasPrefix(args[0], "-") {
		return cmdSession(ctx, env, args)
	}
	printUsage(env.Stderr)
	return 2
}

func parsesFlags(command string) bool {
	switch command {
	case "run", "resume", "recover", "web", "permissions":
		return true
	}
	return false
}

func isHelp(s string) bool {
	return s == "help" || s == "-h" || s == "--help"
}

func printVersion(env Env) {
	fmt.Fprintf(env.Stdout, "hp %s\n", env.Version)
}
