package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	doctorOK         = "ok"
	doctorWarn       = "warn"
	doctorFail       = "fail"
	doctorGitTimeout = 3 * time.Second
)

var doctorNetworkBudget = 5 * time.Second

type doctorCheck struct {
	status string
	name   string
	msg    string
	fix    string
}

func cmdDoctor(ctx context.Context, env Env, args []string) int {
	if len(args) != 0 {
		return exitUsage(env, usagef("doctor takes no arguments"))
	}
	checks := []doctorCheck{doctorVersion(env), doctorSettings(env)}
	checks = append(checks, doctorDirs(env)...)
	checks = append(checks, doctorGit(ctx, env), doctorKeys(env))
	checks = append(checks, doctorModels(env)...)
	checks = append(checks, doctorNetwork(ctx, env)...)
	return printDoctor(env, checks)
}

func printDoctor(env Env, checks []doctorCheck) int {
	counts := map[string]int{}
	for _, c := range checks {
		counts[c.status]++
		fmt.Fprintf(env.Stdout, "[%s] %s: %s\n", c.status, c.name, c.msg)
		if c.fix != "" {
			fmt.Fprintf(env.Stdout, "  fix: %s\n", c.fix)
		}
	}
	fmt.Fprintf(env.Stdout, "hp doctor: ok=%d warn=%d fail=%d\n", counts[doctorOK], counts[doctorWarn], counts[doctorFail])
	if counts[doctorFail] > 0 {
		return 1
	}
	return 0
}

func doctorVersion(env Env) doctorCheck {
	msg := "hp " + env.Version
	if env.Exe != "" {
		msg += " at " + env.Exe
	}
	return doctorCheck{status: doctorOK, name: "version", msg: msg}
}

func doctorSettings(env Env) doctorCheck {
	file, err := settingsPath(rawGetenv(env))
	if err != nil {
		return doctorCheck{status: doctorFail, name: "settings", msg: err.Error(), fix: "set HOME to an absolute path"}
	}
	if _, _, err := loadSettingsAndRules(env); err != nil {
		return doctorCheck{status: doctorFail, name: "settings", msg: firstLine(err.Error()), fix: "edit or remove " + file}
	}
	if _, err := env.Host.Stat(file); errors.Is(err, fs.ErrNotExist) {
		return doctorCheck{status: doctorOK, name: "settings", msg: "no " + file + ", hp uses its defaults"}
	}
	return doctorCheck{status: doctorOK, name: "settings", msg: "read " + file}
}

func doctorDirs(env Env) []doctorCheck {
	config, err := userConfigDir(rawGetenv(env))
	if err != nil {
		return []doctorCheck{{status: doctorFail, name: "config folder", msg: err.Error(), fix: "set HOME to an absolute path"}}
	}
	return []doctorCheck{
		doctorWritable(env, "config folder", config),
		doctorWritable(env, "state folder", path.Dir(runsDir(env.Getenv))),
	}
}

func doctorWritable(env Env, name, dir string) doctorCheck {
	if _, err := env.Host.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return doctorCheck{status: doctorOK, name: name, msg: dir + " does not exist yet, hp creates it when needed"}
	}
	probe := path.Join(dir, ".doctor-probe")
	if err := env.Host.WriteFile(probe, nil); err != nil {
		return doctorCheck{status: doctorFail, name: name, msg: firstLine(err.Error()), fix: "make " + dir + " writable by your user"}
	}
	_ = env.Host.Remove(probe)
	return doctorCheck{status: doctorOK, name: name, msg: dir + " is writable"}
}

func doctorGit(ctx context.Context, env Env) doctorCheck {
	res, err := env.Host.Exec(ctx, host.ExecRequest{Command: "git --version", Dir: env.Cwd, Timeout: doctorGitTimeout})
	if err != nil || res.ExitCode != 0 {
		return doctorCheck{status: doctorWarn, name: "git", msg: "git did not run", fix: "install git; hp crew and hp add <git-url> need it"}
	}
	return doctorCheck{status: doctorOK, name: "git", msg: strings.TrimSpace(string(res.Output))}
}

func doctorKeys(env Env) doctorCheck {
	getenv := rawGetenv(env)
	file, _ := keyFilePath(getenv)
	var found []string
	seen := map[string]bool{}
	for _, name := range presetNames() {
		key := presets[name].keyEnv
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if env.Getenv(key) == "" {
			continue
		}
		source := "env"
		if getenv(key) == "" {
			source = file
		}
		found = append(found, key+" ("+source+")")
	}
	if len(found) == 0 {
		return doctorCheck{status: doctorWarn, name: "keys", msg: "no provider key is set", fix: "hp key set GROQ_API_KEY (hp providers lists the names)"}
	}
	return doctorCheck{status: doctorOK, name: "keys", msg: strings.Join(found, ", ")}
}

func doctorModels(env Env) []doctorCheck {
	var checks []doctorCheck
	if spec := strings.TrimSpace(env.Getenv("HP_MODEL")); spec != "" {
		checks = append(checks, doctorModel(env, "HP_MODEL", spec))
	}
	if spec := loadLastModel(env); spec != "" {
		checks = append(checks, doctorModel(env, "last model", spec))
	}
	if len(checks) == 0 {
		checks = append(checks, doctorCheck{status: doctorWarn, name: "model", msg: "no HP_MODEL and no saved model", fix: "run hp and pick a model, or export HP_MODEL=groq/openai/gpt-oss-120b"})
	}
	return checks
}

func doctorModel(env Env, name, spec string) doctorCheck {
	ep, err := resolveEndpoint(spec, "", "", env.Getenv)
	if err != nil {
		return doctorCheck{status: doctorFail, name: name, msg: spec + ": " + firstLine(err.Error()), fix: doctorModelFix(spec)}
	}
	return doctorCheck{status: doctorOK, name: name, msg: spec + " resolves to " + ep.BaseURL}
}

func doctorModelFix(spec string) string {
	preset, _ := splitFirstSlash(spec)
	if def, ok := presets[preset]; ok && def.keyEnv != "" {
		return "hp key set " + def.keyEnv
	}
	return "use preset/model, for example " + modelExample
}

type doctorTarget struct {
	name     string
	ep       Endpoint
	optional bool
}

func doctorTargets(env Env) []doctorTarget {
	var targets []doctorTarget
	for _, name := range configuredPresets(env.Getenv) {
		ep, err := resolveEndpoint(name+"/x", "", "", env.Getenv)
		if err != nil {
			continue
		}
		targets = append(targets, doctorTarget{name: name, ep: ep, optional: presets[name].keyEnv == ""})
	}
	for _, spec := range []string{env.Getenv("HP_MODEL"), loadLastModel(env)} {
		if ep, err := resolveEndpoint(spec, "", "", env.Getenv); err == nil && ep.Preset == customPreset {
			targets = append(targets, doctorTarget{name: customPreset, ep: ep})
		}
	}
	return targets
}

func doctorNetwork(ctx context.Context, env Env) []doctorCheck {
	targets := doctorTargets(env)
	ctx, cancel := context.WithTimeout(ctx, doctorNetworkBudget)
	defer cancel()
	results := make([]chan doctorCheck, len(targets))
	for i, t := range targets {
		results[i] = make(chan doctorCheck, 1)
		go func() { results[i] <- doctorEndpoint(ctx, env, t) }()
	}
	var checks []doctorCheck
	answered := false
	for _, ch := range results {
		c := <-ch
		answered = answered || c.status == doctorOK
		checks = append(checks, c)
	}
	if !answered {
		checks = append(checks, doctorCheck{status: doctorFail, name: "providers", msg: "no provider answered, so hp cannot run a model", fix: "hp key set NAME for a provider, or start Ollama"})
	}
	return checks
}

func doctorEndpoint(ctx context.Context, env Env, t doctorTarget) doctorCheck {
	ids, err := provider.ListModels(ctx, env.NewProvider(t.ep))
	switch {
	case err == nil:
		return doctorCheck{status: doctorOK, name: t.name, msg: fmt.Sprintf("%s answered, %d models", t.ep.BaseURL, len(ids))}
	case errors.Is(err, provider.ErrNoModelList):
		return doctorCheck{status: doctorOK, name: t.name, msg: "key is set; this provider has no model list to check"}
	}
	status := doctorFail
	if t.optional {
		status = doctorWarn
	}
	msg, fix := doctorEndpointError(t, err)
	return doctorCheck{status: status, name: t.name, msg: msg, fix: fix}
}

func doctorEndpointError(t doctorTarget, err error) (string, string) {
	var httpErr *provider.HTTPError
	switch {
	case errors.As(err, &httpErr) && (httpErr.Status == 401 || httpErr.Status == 403):
		return fmt.Sprintf("%s refused the key (HTTP %d)", t.ep.BaseURL, httpErr.Status), "hp key set " + t.ep.KeyEnv + " with a valid key"
	case errors.As(err, &httpErr):
		return fmt.Sprintf("%s answered HTTP %d", t.ep.BaseURL, httpErr.Status), "check the base URL; hp providers shows each preset URL"
	case errors.Is(err, context.DeadlineExceeded):
		return t.ep.BaseURL + " did not answer in " + doctorNetworkBudget.String(), "check your network or proxy"
	case t.optional:
		return cutRunes(firstLine(err.Error()), 160), "start " + t.name + " or ignore this if you do not use it"
	}
	return cutRunes(firstLine(err.Error()), 160), "check your network, proxy or base URL"
}
