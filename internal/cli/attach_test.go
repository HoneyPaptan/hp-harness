package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

func TestAttachFinishedRun(t *testing.T) {
	h := host.NewMemory()
	env, stdout, _ := testEnv(t, h, nil, nil)
	id := "done1"
	seedRun(t, h, env.Getenv, id, "p", "final", true)
	logPath := runLogPath(env.Getenv, id)
	_ = h.WriteFile(logPath, []byte("hello log\n"))
	env.Args = []string{"attach", id}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	if stdout.String() != "hello log\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestAttachFollowsAppends(t *testing.T) {
	h := host.NewMemory()
	id := "grow"
	logPath := runLogPath(getenvMap(map[string]string{"HOME": "/home/t", "XDG_STATE_HOME": "/state"}), id)
	polls := 0
	running := true
	hook := &sleepHookHost{Memory: h, onSleep: func() {
		polls++
		switch polls {
		case 1:
			_ = h.Append(logPath, []byte("part1"))
		case 2:
			_ = h.Append(logPath, []byte("part2\n"))
			running = false
			seedRunEnd(t, h, id)
		}
	}}
	env, stdout, _ := testEnv(t, h, nil, nil)
	env.Host = hook
	seedRun(t, h, env.Getenv, id, "p", "", false)
	_ = h.WriteFile(runLogPath(env.Getenv, id), []byte("start\n"))
	_ = h.WriteFile(runPidPath(env.Getenv, id), []byte("9"))
	env.Alive = func(int) bool { return running }
	env.Args = []string{"attach", id}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d stdout %q", code, stdout.String())
	}
	want := "start\npart1part2\n"
	if stdout.String() != want {
		t.Fatalf("stdout %q want %q", stdout.String(), want)
	}
}

func TestAttachLiveRunFinishes(t *testing.T) {
	h := host.NewMemory()
	id := "livefin"
	checks := 0
	running := true
	hook := &sleepHookHost{Memory: h, onSleep: func() {
		running = false
		seedRunEnd(t, h, id)
	}}
	env, _, _ := testEnv(t, h, nil, nil)
	env.Host = hook
	env.Alive = func(int) bool {
		checks++
		return running
	}
	seedRun(t, h, env.Getenv, id, "p", "", false)
	_ = h.WriteFile(runPidPath(env.Getenv, id), []byte("42"))
	env.Args = []string{"attach", id}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	if checks < 1 {
		t.Fatal("alive not checked while running")
	}
}

func TestAttachCtxCancel(t *testing.T) {
	h := host.NewMemory()
	block := &blockSleepHost{Memory: h}
	env, _, stderr := testEnv(t, h, nil, nil)
	env.Host = block
	id := "run"
	seedRun(t, h, env.Getenv, id, "p", "", false)
	_ = h.WriteFile(runPidPath(env.Getenv, id), []byte("2"))
	env.Alive = func(int) bool { return true }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env.Args = []string{"attach", id}
	if code := Main(ctx, env); code != 130 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "detached, the run keeps going") {
		t.Fatal(stderr.String())
	}
}

type sleepHookHost struct {
	*host.Memory
	onSleep func()
}

func (s *sleepHookHost) Sleep(ctx context.Context, d time.Duration) error {
	if s.onSleep != nil {
		s.onSleep()
	}
	return s.Memory.Sleep(ctx, d)
}

type blockSleepHost struct {
	*host.Memory
}

func (b *blockSleepHost) Sleep(ctx context.Context, d time.Duration) error {
	return ctx.Err()
}

func seedRunEnd(t *testing.T, h *host.Memory, id string) {
	t.Helper()
	getenv := getenvMap(map[string]string{"HOME": "/home/t", "XDG_STATE_HOME": "/state"})
	jpath := filepath.Join(runsDir(getenv), id+".jsonl")
	w, _, err := journal.Open(h, jpath)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.End("final"); err != nil {
		t.Fatal(err)
	}
}
