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
	fmt.Fprintln(env.Stderr, unknownCommandLine(args[0]))
	return 2
}

func unknownCommandLine(name string) string {
	line := fmt.Sprintf("unknown command %q.", name)
	if near := nearestCommand(name); near != "" {
		line = fmt.Sprintf("unknown command %q, did you mean %q?", name, near)
	}
	return line + " hp help says what each command does, hp run PROMPT sends a prompt."
}

func nearestCommand(name string) string {
	best, bestDistance := "", 3
	for _, c := range commandHelps {
		if d := editDistance(name, c.name); d < bestDistance {
			best, bestDistance = c.name, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(a); i++ {
		prev := row[0]
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			next := min(row[j]+1, row[j-1]+1, prev+cost)
			prev, row[j] = row[j], next
		}
	}
	return row[len(b)]
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
