package cli

import (
	"encoding/json"
	"flag"
	"fmt"
)

func cmdPermissions(env Env, args []string) int {
	fs := flag.NewFlagSet("permissions", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage(env, err)
	}
	if fs.NArg() != 0 {
		return exitUsage(env, usagef("unexpected arguments"))
	}
	st, rules, err := loadSettingsAndRules(env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 2
	}
	mode, source, err := resolvePermissionMode(false, rawGetenv(env), "", st)
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 2
	}
	file, err := settingsPath(rawGetenv(env))
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 2
	}
	if !asJSON {
		fmt.Fprintf(env.Stdout, "mode: %s (%s)\n", mode, source)
		fmt.Fprintf(env.Stdout, "settings: %s\n", file)
		return 0
	}
	out := struct {
		Mode     string     `json:"mode"`
		Source   string     `json:"source"`
		Rules    []ruleJSON `json:"rules"`
		Settings string     `json:"settings"`
	}{
		Mode: string(mode), Source: source, Settings: file,
	}
	for _, r := range rules {
		out.Rules = append(out.Rules, ruleJSON{Tool: r.Tool, Pattern: r.Pattern, Verdict: string(r.Verdict)})
	}
	enc := json.NewEncoder(env.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

type ruleJSON struct {
	Tool    string `json:"tool"`
	Pattern string `json:"pattern"`
	Verdict string `json:"verdict"`
}
