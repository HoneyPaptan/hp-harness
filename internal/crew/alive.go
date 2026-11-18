package crew

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func pidPath(getenv func(string) string, id string) string {
	return filepath.Join(crewsDir(getenv), id+".pid")
}

func selfPid(env Env) int {
	if env.Pid != 0 {
		return env.Pid
	}
	return os.Getpid()
}

func procStart(env Env, pid int) string {
	if env.ProcStart == nil {
		return ""
	}
	return env.ProcStart(pid)
}

func pidRecord(env Env, pid int) []byte {
	data := strconv.Itoa(pid)
	if start := procStart(env, pid); start != "" {
		data += " " + start
	}
	return []byte(data)
}

func livePid(env Env, path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pidText, start, _ := strings.Cut(strings.TrimSpace(string(data)), " ")
	pid, err := strconv.Atoi(pidText)
	if err != nil || env.Alive == nil || !env.Alive(pid) {
		return 0, false
	}
	if start != "" && procStart(env, pid) != start {
		return 0, false
	}
	return pid, true
}

func supervisorPid(env Env, id string) (int, bool) {
	return livePid(env, pidPath(env.Getenv, id))
}

func beginSupervise(env Env, id string) (func(), error) {
	if pid, ok := supervisorPid(env, id); ok && pid != selfPid(env) {
		return nil, fmt.Errorf("crew %s is already running (pid %d)", id, pid)
	}
	path := pidPath(env.Getenv, id)
	if err := os.WriteFile(path, pidRecord(env, selfPid(env)), 0o600); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}
