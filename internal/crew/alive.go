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

func supervisorPid(env Env, id string) (int, bool) {
	data, err := os.ReadFile(pidPath(env.Getenv, id))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || env.Alive == nil || !env.Alive(pid) {
		return 0, false
	}
	return pid, true
}

func beginSupervise(env Env, id string) (func(), error) {
	if pid, ok := supervisorPid(env, id); ok && pid != selfPid(env) {
		return nil, fmt.Errorf("crew %s is already running (pid %d)", id, pid)
	}
	path := pidPath(env.Getenv, id)
	if err := os.WriteFile(path, []byte(strconv.Itoa(selfPid(env))), 0o600); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}
