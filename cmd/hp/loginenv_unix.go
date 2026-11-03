//go:build unix

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/cli"
)

const loginEnvTimeout = 3 * time.Second

var loginEnvWait = loginEnvTimeout

func memoLoginEnv(env *cli.Env) func(string) func() []string {
	var once sync.Once
	var get func() []string
	return func(dir string) func() []string {
		once.Do(func() {
			get = startLoginEnv(*env, dir)
		})
		return get
	}
}

func startLoginEnv(env cli.Env, dir string) func() []string {
	if env.Getenv("HP_LOGIN_ENV") == "0" {
		return func() []string { return nil }
	}
	shell := env.Getenv("SHELL")
	if shell == "" || !filepath.IsAbs(shell) {
		return func() []string { return nil }
	}
	cap := &loginEnvCapture{ready: make(chan struct{})}
	go cap.run(shell, dir, os.Environ(), loginEnvWait)
	return cap.get
}

type loginEnvCapture struct {
	ready chan struct{}
	env   []string
}

func (c *loginEnvCapture) get() []string {
	<-c.ready
	return c.env
}

func (c *loginEnvCapture) run(shell, dir string, procEnv []string, wait time.Duration) {
	c.env = captureLoginEnv(shell, dir, procEnv, wait)
	close(c.ready)
}

func captureLoginEnv(shell, dir string, procEnv []string, wait time.Duration) []string {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", "env -0")
	cmd.Dir = dir
	cmd.Env = procEnv
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	return parseLoginEnv(out)
}

func parseLoginEnv(out []byte) []string {
	var env []string
	s := string(out)
	for _, part := range splitNul(s) {
		for _, line := range splitLines(part) {
			key, val, ok := cutEq(line)
			if !ok || !loginEnvKeyOK(key) {
				continue
			}
			if key == "PWD" || key == "OLDPWD" || key == "SHLVL" || key == "_" {
				continue
			}
			env = append(env, key+"="+val)
		}
	}
	if len(env) < 3 {
		return nil
	}
	return env
}

func splitNul(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != 0 {
			continue
		}
		parts = append(parts, s[start:i])
		start = i + 1
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '\n' {
			continue
		}
		lines = append(lines, s[start:i])
		start = i + 1
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func cutEq(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] != '=' {
			continue
		}
		return s[:i], s[i+1:], true
	}
	return "", "", false
}

func loginEnvKeyOK(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if i == 0 {
			if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
				return false
			}
			continue
		}
		if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
