package crew

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Env struct {
	Args   []string
	Getenv func(string) string
	Cwd    string
	Stdout io.Writer
	Stderr io.Writer
	Exe    string
	Alive  func(pid int) bool
	Git    func(dir string, args ...string) (string, error)
	RunHP  func(ctx context.Context, args ...string) (string, error)
	Spawn  func(args []string, logPath string) (int, error)
	Pid    int
}

type runFlags struct {
	n       int
	model   string
	baseURL string
	keyEnv  string
}

func Main(ctx context.Context, env Env, args []string) int {
	if len(args) == 0 {
		return usage(env)
	}
	switch args[0] {
	case "crew":
		if len(args) < 2 {
			return usage(env)
		}
		return crewCommand(ctx, env, args[1], args[2:])
	case "crews":
		return cmdCrews(env, args[1:])
	default:
		return usage(env)
	}
}

func crewCommand(ctx context.Context, env Env, name string, args []string) int {
	switch name {
	case "run":
		return cmdRun(ctx, env, args)
	case "resume":
		return cmdResume(ctx, env, args)
	case "status":
		return cmdStatus(env, args)
	case "report":
		return cmdReport(env, args)
	default:
		return usage(env)
	}
}

func cmdRun(ctx context.Context, env Env, args []string) int {
	o, rest, err := parseRunFlags(env, args)
	if err != nil {
		return usageErr(env, err)
	}
	if len(rest) == 0 {
		return usageErr(env, fmt.Errorf("plan file required"))
	}
	if len(rest) > 1 {
		return usageErr(env, fmt.Errorf("flags must come before the plan file, got: %s", strings.Join(rest[1:], " ")))
	}
	planPath := resolvePath(env.Cwd, rest[0])
	data, err := os.ReadFile(planPath)
	if err != nil {
		return exitErr(env, err)
	}
	tasks, err := parsePlan(data)
	if err != nil {
		return exitErr(env, err)
	}
	repo, base, err := gitRootHead(env, env.Cwd)
	if err != nil {
		return exitErr(env, err)
	}
	st := newState(env.Getenv, newCrewID(), planPath, repo, base, o, tasks)
	if err := st.save(env.Getenv); err != nil {
		return exitErr(env, err)
	}
	fmt.Fprintf(env.Stdout, "%s\n", st.ID)
	release, err := beginSupervise(env, st.ID)
	if err != nil {
		return exitErr(env, err)
	}
	defer release()
	if err := supervise(ctx, env, st); err != nil {
		if isCanceled(ctx) {
			fmt.Fprintf(env.Stderr, "interrupted, resume with: hp crew resume %s\n", st.ID)
			return 130
		}
		return exitErr(env, err)
	}
	return 0
}

func cmdResume(ctx context.Context, env Env, args []string) int {
	id, ok := oneArg(env, args, "crew id required")
	if !ok {
		return 2
	}
	st, err := load(env.Getenv, id)
	if err != nil {
		return exitErr(env, err)
	}
	release, err := beginSupervise(env, id)
	if err != nil {
		return exitErr(env, err)
	}
	defer release()
	if err := supervise(ctx, env, st); err != nil {
		if isCanceled(ctx) {
			fmt.Fprintf(env.Stderr, "interrupted, resume with: hp crew resume %s\n", id)
			return 130
		}
		return exitErr(env, err)
	}
	return 0
}

func cmdStatus(env Env, args []string) int {
	st, ok := loadArg(env, args)
	if !ok {
		return 2
	}
	fmt.Fprint(env.Stdout, statusText(env, st))
	return 0
}

func cmdReport(env Env, args []string) int {
	st, ok := loadArg(env, args)
	if !ok {
		return 2
	}
	fmt.Fprint(env.Stdout, reportText(env, st))
	return 0
}

func cmdCrews(env Env, args []string) int {
	if len(args) != 0 {
		return usageErr(env, fmt.Errorf("unexpected arguments"))
	}
	fmt.Fprint(env.Stdout, listText(env))
	return 0
}

func parseRunFlags(env Env, args []string) (runFlags, []string, error) {
	fs := flag.NewFlagSet("crew run", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var o runFlags
	fs.IntVar(&o.n, "n", 2, "tasks at once")
	fs.StringVar(&o.model, "m", "", "preset/model")
	fs.StringVar(&o.baseURL, "base-url", "", "API base URL")
	fs.StringVar(&o.keyEnv, "key-env", "", "API key env var")
	if err := fs.Parse(args); err != nil {
		return runFlags{}, nil, err
	}
	return o, fs.Args(), nil
}

func loadArg(env Env, args []string) (*State, bool) {
	id, ok := oneArg(env, args, "crew id required")
	if !ok {
		return nil, false
	}
	st, err := load(env.Getenv, id)
	if err != nil {
		exitErr(env, err)
		return nil, false
	}
	return st, true
}

func oneArg(env Env, args []string, msg string) (string, bool) {
	if len(args) != 1 {
		usageErr(env, fmt.Errorf("%s", msg))
		return "", false
	}
	return args[0], true
}

func statusWord(t *Task) string {
	if t.Status == StatusFailed && t.EndReason != "" {
		return "failed: " + t.EndReason
	}
	return string(t.Status)
}

func runID(t *Task) string {
	if t.RunID == "" {
		return "-"
	}
	return t.RunID
}

func resolvePath(cwd, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}

func execHP(ctx context.Context, env Env, args ...string) (string, error) {
	exe := env.Exe
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%s: %s", err, strings.TrimSpace(out.String()))
	}
	return out.String(), nil
}

func usage(env Env) int {
	fmt.Fprintln(env.Stderr, "usage: hp crew <run|resume|status|report> ... | hp crews")
	fmt.Fprintln(env.Stderr, "usage: hp crew run [-n N] [-m preset/model] [--base-url URL] [--key-env NAME] <plan.md>")
	return 2
}

func usageErr(env Env, err error) int {
	fmt.Fprintf(env.Stderr, "error: %v\n", err)
	return 2
}

func exitErr(env Env, err error) int {
	fmt.Fprintf(env.Stderr, "error: %v\n", err)
	return 1
}

func isCanceled(ctx context.Context) bool {
	return ctx.Err() != nil
}
