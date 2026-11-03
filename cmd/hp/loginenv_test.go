//go:build unix

package main

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/cli"
)

func TestParseLoginEnv(t *testing.T) {
	raw := "motd\nA=1\x00B=2\x00PWD=/x\x00C=3\x00"
	got := parseLoginEnv([]byte(raw))
	want := []string{"A=1", "B=2", "C=3"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestStartLoginEnvFakeShell(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-shell")
	body := "#!/bin/sh\n[ \"$1\" = \"-l\" ] && shift\n[ \"$1\" = \"-c\" ] && shift\nprintf 'motd\\nA=1\\0B=2\\0PWD=/x\\0C=3\\0'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	env := cli.Env{Getenv: testGetenv(map[string]string{"SHELL": script})}
	get := startLoginEnv(env, dir)
	waitLoginEnv(get, 2*time.Second)
	if !slices.Equal(get(), []string{"A=1", "B=2", "C=3"}) {
		t.Fatalf("got %v", get())
	}
}

func TestStartLoginEnvTimeout(t *testing.T) {
	old := loginEnvWait
	loginEnvWait = 50 * time.Millisecond
	dir := t.TempDir()
	script := filepath.Join(dir, "slow-shell")
	body := "#!/bin/sh\n[ \"$1\" = \"-l\" ] && shift\n[ \"$1\" = \"-c\" ] && shift\nsleep 2\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	env := cli.Env{Getenv: testGetenv(map[string]string{"SHELL": script})}
	get := startLoginEnv(env, dir)
	time.Sleep(300 * time.Millisecond)
	if get() != nil {
		t.Fatalf("got %v", get())
	}
	loginEnvWait = old
}

func TestStartLoginEnvExitFail(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bad-shell")
	body := "#!/bin/sh\n[ \"$1\" = \"-l\" ] && shift\n[ \"$1\" = \"-c\" ] && shift\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	env := cli.Env{Getenv: testGetenv(map[string]string{"SHELL": script})}
	get := startLoginEnv(env, dir)
	time.Sleep(200 * time.Millisecond)
	if get() != nil {
		t.Fatal("expected nil")
	}
}

func TestStartLoginEnvEmptyShell(t *testing.T) {
	env := cli.Env{Getenv: testGetenv(nil)}
	if startLoginEnv(env, t.TempDir())() != nil {
		t.Fatal("expected nil")
	}
}

func TestStartLoginEnvDisabled(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'A=1\\0B=2\\0C=3\\0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := cli.Env{Getenv: testGetenv(map[string]string{"SHELL": script, "HP_LOGIN_ENV": "0"})}
	if startLoginEnv(env, dir)() != nil {
		t.Fatal("expected nil")
	}
}

func TestStartLoginEnvNoBlock(t *testing.T) {
	old := loginEnvWait
	loginEnvWait = 2 * time.Second
	defer func() { loginEnvWait = old }()
	dir := t.TempDir()
	script := filepath.Join(dir, "slow-ok")
	body := "#!/bin/sh\n[ \"$1\" = \"-l\" ] && shift\n[ \"$1\" = \"-c\" ] && shift\nsleep 1\nprintf 'A=1\\0B=2\\0C=3\\0'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	env := cli.Env{Getenv: testGetenv(map[string]string{"SHELL": script})}
	get := startLoginEnv(env, dir)
	ch := make(chan []string, 1)
	go func() { ch <- get() }()
	select {
	case v := <-ch:
		if v != nil {
			t.Fatalf("expected nil before ready, got %v", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("get blocked")
	}
	waitLoginEnv(get, 3*time.Second)
	if get() == nil {
		t.Fatal("expected env")
	}
}

func waitLoginEnv(get func() []string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		if v := get(); v != nil {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func testGetenv(m map[string]string) func(string) string {
	return func(key string) string {
		if m == nil {
			return ""
		}
		return m[key]
	}
}

func TestMemoLoginEnvOnce(t *testing.T) {
	env := cli.Env{Getenv: testGetenv(map[string]string{"HP_LOGIN_ENV": "0"})}
	login := memoLoginEnv(&env)
	var n sync.WaitGroup
	n.Add(2)
	go func() {
		defer n.Done()
		login("/w")
	}()
	go func() {
		defer n.Done()
		login("/w")
	}()
	n.Wait()
}
