package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/core"
)

func runLogPath(getenv func(string) string, id string) string {
	return filepath.Join(runsDir(getenv), id+".log")
}

func runPidPath(getenv func(string) string, id string) string {
	return filepath.Join(runsDir(getenv), id+".pid")
}

func detachRun(_ context.Context, env Env, o runOpts, prompt string) int {
	ep, _, err := resolveRunSpecs(o, env.Getenv)
	if err != nil {
		return exitUsage(env, err)
	}
	dir, err := resolveWorkDir(env, o.dir)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	id, err := newRunID(env.Host.Now(), env.Rand)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	skills, err := discoverSkills(env, dir)
	if err != nil {
		return exitErr(env, id, jpath, err)
	}
	cfg := runConfig(id, ep, dir, jpath, o, prompt, skills, env)
	if err := core.Seed(core.Deps{Host: env.Host}, cfg); err != nil {
		return exitErr(env, id, jpath, err)
	}
	return spawnDetached(env, id)
}

func spawnDetached(env Env, id string) int {
	logPath := runLogPath(env.Getenv, id)
	pid, err := env.Spawn([]string{"resume", id}, logPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	if err := writeRunPidFor(env, id, pid); err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(env.Stdout, "%s\n", id)
	fmt.Fprintf(env.Stderr, "attach with: hp attach %s\n", id)
	return 0
}

func writeRunPidFor(env Env, id string, pid int) error {
	data := []byte(strconv.Itoa(pid))
	return env.Host.WriteFile(runPidPath(env.Getenv, id), data)
}

func writeRunPid(env Env, id string) error {
	return writeRunPidFor(env, id, env.Pid)
}

func readRunPid(env Env, id string) (int, bool) {
	data, err := env.Host.ReadFile(runPidPath(env.Getenv, id))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

func removeRunPid(env Env, id string) {
	_ = env.Host.Remove(runPidPath(env.Getenv, id))
}

func runAlive(env Env, id string) bool {
	pid, ok := readRunPid(env, id)
	if !ok {
		return false
	}
	return env.Alive(pid)
}

func beginResume(env Env, id string) (func(), int) {
	if pid, ok := readRunPid(env, id); ok && env.Alive(pid) && pid != env.Pid {
		fmt.Fprintf(env.Stderr, "run %s is running, follow it with: hp attach %s\n", id, id)
		return nil, 1
	}
	if err := writeRunPid(env, id); err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return nil, 1
	}
	return func() { removeRunPid(env, id) }, -1
}

func cmdStop(env Env, args []string) int {
	if len(args) != 1 {
		return exitUsage(env, usagef("run id required"))
	}
	id := args[0]
	pid, ok := readRunPid(env, id)
	if !ok || !env.Alive(pid) {
		fmt.Fprintf(env.Stderr, "run %s is not running\n", id)
		return 1
	}
	if err := env.Signal(pid); err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(env.Stdout, "stopping %s\n", id)
	return 0
}
