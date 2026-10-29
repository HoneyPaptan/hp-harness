package cli

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestRunDetach(t *testing.T) {
	h := host.NewMemory()
	var spawnArgs []string
	var spawnLog string
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("nope")}}
	env, stdout, stderr := testEnv(t, h, fp, nil)
	env.Spawn = func(args []string, logPath string) (int, error) {
		spawnArgs = append([]string(nil), args...)
		spawnLog = logPath
		return 42, nil
	}
	env.Args = []string{"run", "-d", "-m", "xai/grok", "hi there"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	id := strings.TrimSpace(stdout.String())
	if id == "" {
		t.Fatal("no id")
	}
	if len(fp.Requests) != 0 {
		t.Fatalf("provider called %d times", len(fp.Requests))
	}
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	recs, err := journal.Read(h, jpath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	if st.Finished || firstPrompt(st) != "hi there" {
		t.Fatalf("state finished=%v prompt %q", st.Finished, firstPrompt(st))
	}
	wantLog := runLogPath(env.Getenv, id)
	if spawnLog != wantLog {
		t.Fatalf("log %q want %q", spawnLog, wantLog)
	}
	if len(spawnArgs) != 2 || spawnArgs[0] != "resume" || spawnArgs[1] != id {
		t.Fatalf("spawn %v", spawnArgs)
	}
	if !strings.Contains(stderr.String(), "attach with: hp attach "+id) {
		t.Fatal(stderr.String())
	}
	pidData, err := h.ReadFile(runPidPath(env.Getenv, id))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(pidData)) != "42" {
		t.Fatalf("pid file %q", pidData)
	}
}

func TestResumeLivePidRefused(t *testing.T) {
	h := host.NewMemory()
	live := map[int]bool{99: true}
	env, _, stderr := testEnv(t, h, nil, nil)
	env.Pid = 1
	env.Alive = func(pid int) bool { return live[pid] }
	id := "liveid"
	seedRun(t, h, env.Getenv, id, "p", "", false)
	_ = h.WriteFile(runPidPath(env.Getenv, id), []byte("99"))
	env.Args = []string{"resume", id}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "follow it with: hp attach "+id) {
		t.Fatal(stderr.String())
	}
}

func TestResumeStalePidIgnored(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	env, stdout, _ := testEnv(t, h, fp, nil)
	env.Pid = 7
	env.Alive = func(int) bool { return false }
	id := "stale"
	seedRun(t, h, env.Getenv, id, "p", "", false)
	_ = h.WriteFile(runPidPath(env.Getenv, id), []byte("999"))
	env.Args = []string{"resume", id}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
	data, err := h.ReadFile(runPidPath(env.Getenv, id))
	if err == nil {
		t.Fatalf("pid file still present %q", data)
	}
}

func TestRunsShowsRunning(t *testing.T) {
	h := host.NewMemory()
	env, stdout, _ := testEnv(t, h, nil, nil)
	env.Alive = func(int) bool { return true }
	id := "run1"
	seedRun(t, h, env.Getenv, id, "p", "", false)
	_ = h.WriteFile(runPidPath(env.Getenv, id), []byte("55"))
	env.Args = []string{"runs"}
	if Main(context.Background(), env) != 0 {
		t.Fatal()
	}
	if !strings.Contains(stdout.String(), id+"  running") {
		t.Fatal(stdout.String())
	}
}

func TestStopSignalsLive(t *testing.T) {
	h := host.NewMemory()
	var sig sync.Mutex
	var got int
	env, stdout, stderr := testEnv(t, h, nil, nil)
	env.Alive = func(int) bool { return true }
	env.Signal = func(pid int) error {
		sig.Lock()
		got = pid
		sig.Unlock()
		return nil
	}
	id := "stopme"
	_ = h.WriteFile(runPidPath(env.Getenv, id), []byte("88"))
	env.Args = []string{"stop", id}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if got != 88 {
		t.Fatalf("signal pid %d", got)
	}
	if stdout.String() != "stopping "+id+"\n" {
		t.Fatal(stdout.String())
	}
}

func TestStopNotRunning(t *testing.T) {
	h := host.NewMemory()
	env, _, stderr := testEnv(t, h, nil, nil)
	env.Args = []string{"stop", "ghost"}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "run ghost is not running") {
		t.Fatal(stderr.String())
	}
}

func TestResumeDetachSpawns(t *testing.T) {
	h := host.NewMemory()
	var spawnArgs []string
	env, stdout, stderr := testEnv(t, h, nil, nil)
	env.Spawn = func(args []string, _ string) (int, error) {
		spawnArgs = args
		return 1, nil
	}
	id := "rdet"
	seedRun(t, h, env.Getenv, id, "p", "", false)
	env.Args = []string{"resume", "-d", id}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	if strings.TrimSpace(stdout.String()) != id {
		t.Fatal(stdout.String())
	}
	if len(spawnArgs) != 2 || spawnArgs[1] != id {
		t.Fatalf("%v", spawnArgs)
	}
	if !strings.Contains(stderr.String(), "attach with:") {
		t.Fatal(stderr.String())
	}
}
