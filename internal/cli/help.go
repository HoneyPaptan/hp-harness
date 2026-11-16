package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

type commandHelp struct {
	name  string
	usage string
	about string
}

var commandHelps = []commandHelp{
	{"run", "hp run [flags] [PROMPT | -]", "run a prompt to the end and journal every step; the prompt can come from stdin"},
	{"resume", "hp resume [-d] ID", "continue an unfinished run"},
	{"attach", "hp attach ID", "follow a background run"},
	{"stop", "hp stop ID", "stop a background run"},
	{"runs", "hp runs", "list runs"},
	{"recover", "hp recover [-dry-run] [-max-age DURATION]", "restart background runs whose process died"},
	{"autostart", "hp autostart on|off|status", "run hp recover at login"},
	{"crew", "hp crew run|resume|status|report|stop|retry|pause|unpause ...", "run a markdown plan as parallel runs in git worktrees"},
	{"crews", "hp crews", "list crews"},
	{"providers", "hp providers", "list provider presets and which keys are set"},
	{"web", "hp web [flags]", "serve the local web UI"},
	{"add", strings.TrimPrefix(addUsage, "usage: "), "connect an MCP server or install a skill"},
	{"list", "hp list", "list MCP servers and skills"},
	{"remove", "hp remove NAME", "remove an MCP server or skill"},
	{"key", strings.TrimPrefix(keyUsage, "usage: "), "store, list or remove provider API keys"},
	{"permissions", "hp permissions [-json]", "show the permission mode and settings path"},
	{"version", "hp version", "print the version"},
}

var sessionHelp = commandHelp{"hp", "hp [-c] [flags]", "open an interactive session, -c continues the newest run in this directory"}

func printUsage(w io.Writer) {
	names := make([]string, len(commandHelps))
	for i, c := range commandHelps {
		names[i] = c.name
	}
	fmt.Fprintf(w, "usage: hp <%s> ...\n", strings.Join(names, "|"))
}

func printHelp(w io.Writer) {
	printUsage(w)
	fmt.Fprintln(w, "hp with no command opens an interactive session; hp -c continues the newest run in this directory.")
	for _, c := range commandHelps {
		fmt.Fprintf(w, "  %-11s %s\n", c.name, c.about)
	}
	fmt.Fprintln(w, "hp COMMAND -h shows the flags of one command.")
}

func commandHelpFor(name string) (commandHelp, bool) {
	if name == sessionHelp.name {
		return sessionHelp, true
	}
	for _, c := range commandHelps {
		if c.name == name {
			return c, true
		}
	}
	return commandHelp{}, false
}

func helpRequested(args []string) bool {
	return len(args) == 1 && isHelp(args[0])
}

func printCommandHelp(w io.Writer, name string) {
	c, ok := commandHelpFor(name)
	if !ok {
		printUsage(w)
		return
	}
	fmt.Fprintf(w, "usage: %s\n%s\n", c.usage, c.about)
}

func setFlagUsage(fs *flag.FlagSet, env Env, name string) {
	fs.SetOutput(env.Stderr)
	fs.Usage = func() {
		printCommandHelp(env.Stderr, name)
		fs.PrintDefaults()
	}
}
