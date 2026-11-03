package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

const recoverID = "20260101-000000-1111"

func seedDetachedRun(t *testing.T, h *host.Memory, getenv func(string) string, id string) {
	t.Helper()
	start := journal.RunStart{
		RunID: id, Model: "m", System: "s", Dir: "/w", StepLimit: 50,
		Preset: "xai", BaseURL: "https://api.x.ai/v1", KeyEnv: "XAI_API_KEY", Detached: true,
	}
	w, err := journal.Create(h, runPath(getenv, id), start)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("long task"); err != nil {
		t.Fatal(err)
	}
}

func markRunFinished(t *testing.T, h *host.Memory, getenv func(string) string, id string) {
	t.Helper()
	w, _, err := journal.Open(h, runPath(getenv, id))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.End("final"); err != nil {
		t.Fatal(err)
	}
}

func markRunStopped(t *testing.T, h *host.Memory, getenv func(string) string, id string) {
	t.Helper()
	if err := journal.MarkStop(h, runPath(getenv, id), "user"); err != nil {
		t.Fatal(err)
	}
}

func setRunAge(t *testing.T, h *host.Memory, getenv func(string) string, id string, old time.Time, now time.Time) {
	t.Helper()
	h.Clock = old
	seedDetachedRun(t, h, getenv, id)
	h.Clock = now
}

func writeRecoverState(t *testing.T, h *host.Memory, env Env, id string, st recoverState) {
	t.Helper()
	data := fmt.Sprintf(`{"records":%d,"attempts":%d}`, st.Records, st.Attempts)
	if err := h.WriteFile(recoverStatePath(env.Getenv, id), []byte(data)); err != nil {
		t.Fatal(err)
	}
}

func readRecoverState(t *testing.T, h *host.Memory, env Env, id string) recoverState {
	t.Helper()
	return loadRecoverState(env, id)
}

func TestRecover(t *testing.T) {
	fixed := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		args        []string
		extraEnv    map[string]string
		seed        func(t *testing.T, h *host.Memory, env Env, id string)
		live        bool
		wantSpawned int
		wantStdout  string
		wantNote    string
		wantCode    int
		check       func(t *testing.T, h *host.Memory, env Env, stdout, stderr fmt.Stringer)
	}{
		{
			name: "crash mid turn",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
			},
			wantSpawned: 1,
			wantStdout:  "recovered " + recoverID,
			check: func(t *testing.T, h *host.Memory, env Env, stdout, stderr fmt.Stringer) {
				st := readRecoverState(t, h, env, recoverID)
				if st.Records != 2 || st.Attempts != 1 {
					t.Fatalf("state %d/%d", st.Records, st.Attempts)
				}
				pid, ok := readRunPid(env, recoverID)
				if !ok || pid != 101 {
					t.Fatalf("pid file %d %v", pid, ok)
				}
			},
		},
		{
			name: "finished run",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
				markRunFinished(t, h, env.Getenv, id)
			},
		},
		{
			name: "hp stop",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
				markRunStopped(t, h, env.Getenv, id)
			},
		},
		{
			name: "ctrl-c stop",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
				markRunStopped(t, h, env.Getenv, id)
			},
		},
		{
			name: "live run",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
				if err := h.WriteFile(runPidPath(env.Getenv, id), []byte("55")); err != nil {
					t.Fatal(err)
				}
			},
			live: true,
		},
		{
			name: "stale pid file",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
				if err := h.WriteFile(runPidPath(env.Getenv, id), []byte("999")); err != nil {
					t.Fatal(err)
				}
			},
			wantSpawned: 1,
			wantStdout:  "recovered " + recoverID,
		},
		{
			name: "reused pid that is not hp",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
				if err := h.WriteFile(runPidPath(env.Getenv, id), []byte("321")); err != nil {
					t.Fatal(err)
				}
			},
			wantSpawned: 1,
			wantStdout:  "recovered " + recoverID,
		},
		{
			name: "too old",
			args: []string{"recover", "--max-age", "24h"},
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				setRunAge(t, h, env.Getenv, id, time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC), fixed)
			},
			wantNote: "skipping " + recoverID + ": older than 24h",
		},
		{
			name: "crash loop counts marks as no progress",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
				if err := journal.MarkDetached(h, runPath(env.Getenv, id)); err != nil {
					t.Fatal(err)
				}
				writeRecoverState(t, h, env, id, recoverState{Records: 2, Attempts: 3})
			},
			wantNote: "skipping " + recoverID + ": keeps crashing after 3 recoveries",
			check: func(t *testing.T, h *host.Memory, env Env, stdout, stderr fmt.Stringer) {
				st := readRecoverState(t, h, env, recoverID)
				if st.Attempts != 3 {
					t.Fatalf("attempts moved to %d", st.Attempts)
				}
			},
		},
		{
			name: "crash loop resets on progress",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
				if err := journal.MarkDetached(h, runPath(env.Getenv, id)); err != nil {
					t.Fatal(err)
				}
				writeRecoverState(t, h, env, id, recoverState{Records: 1, Attempts: 3})
			},
			wantSpawned: 1,
			wantStdout:  "recovered " + recoverID,
			check: func(t *testing.T, h *host.Memory, env Env, stdout, stderr fmt.Stringer) {
				st := readRecoverState(t, h, env, recoverID)
				if st.Records != 2 || st.Attempts != 1 {
					t.Fatalf("state %d/%d", st.Records, st.Attempts)
				}
			},
		},
		{
			name:     "missing api key",
			extraEnv: map[string]string{"XAI_API_KEY": ""},
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
			},
			wantNote: "skipping " + recoverID + ": XAI_API_KEY is not set",
		},
		{
			name: "dry run lists",
			args: []string{"recover", "--dry-run"},
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedDetachedRun(t, h, env.Getenv, id)
			},
			wantStdout: "would recover " + recoverID,
			check: func(t *testing.T, h *host.Memory, env Env, stdout, stderr fmt.Stringer) {
				if _, err := env.Host.Stat(recoverStatePath(env.Getenv, recoverID)); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("state written in dry run: %v", err)
				}
			},
		},
		{
			name: "not a detached run",
			seed: func(t *testing.T, h *host.Memory, env Env, id string) {
				seedRun(t, h, env.Getenv, id, "p", "", false)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			env, stdout, stderr := testEnv(t, h, nil, tc.extraEnv)
			h.Clock = fixed
			seeded := false
			if tc.seed != nil {
				tc.seed(t, h, env, recoverID)
				seeded = true
			}
			env.Alive = func(int) bool { return tc.live }
			spawned := 0
			env.Spawn = func(args []string, logPath string) (int, error) {
				spawned++
				if len(args) != 2 || args[0] != "resume" || args[1] != recoverID {
					t.Errorf("spawn args %v", args)
				}
				return 100 + spawned, nil
			}
			if tc.args == nil {
				tc.args = []string{"recover"}
			}
			env.Args = tc.args
			code := Main(context.Background(), env)
			if !seeded {
				if code != 0 {
					t.Fatalf("code %d", code)
				}
				return
			}
			if code != tc.wantCode {
				t.Fatalf("code %d stderr %s", code, stderr.String())
			}
			if spawned != tc.wantSpawned {
				t.Fatalf("spawned %d want %d", spawned, tc.wantSpawned)
			}
			if tc.wantStdout != "" && !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Fatalf("stdout %q", stdout.String())
			}
			if tc.wantStdout == "" && stdout.String() != "" {
				t.Fatalf("unexpected stdout %q", stdout.String())
			}
			if tc.wantNote != "" && !strings.Contains(stderr.String(), tc.wantNote) {
				t.Fatalf("stderr %q", stderr.String())
			}
			if tc.wantNote == "" && strings.Contains(stderr.String(), "skipping") {
				t.Fatalf("unexpected note %q", stderr.String())
			}
			if tc.check != nil {
				tc.check(t, h, env, stdout, stderr)
			}
		})
	}
}

func TestRecoverNeedsSpawnForFreshRun(t *testing.T) {
	h := host.NewMemory()
	env, stdout, _ := testEnv(t, h, nil, nil)
	env.Args = []string{"recover"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatal()
	}
	if stdout.String() != "" {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestRecoverSecondInvocationStartsNothing(t *testing.T) {
	h := host.NewMemory()
	env, stdout, _ := testEnv(t, h, nil, nil)
	seedDetachedRun(t, h, env.Getenv, recoverID)
	running := false
	env.Alive = func(int) bool { return running }
	spawned := 0
	env.Spawn = func([]string, string) (int, error) {
		spawned++
		running = true
		return 42, nil
	}
	env.Args = []string{"recover"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatal()
	}
	env.Args = []string{"recover"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatal()
	}
	if spawned != 1 {
		t.Fatalf("spawned %d", spawned)
	}
	if strings.Contains(strings.TrimSpace(stdout.String()), "\n") {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestRecoverConcurrentInvocation(t *testing.T) {
	h := host.NewMemory()
	env, _, stderr := testEnv(t, h, nil, nil)
	seedDetachedRun(t, h, env.Getenv, recoverID)
	spawned := 0
	env.Spawn = func([]string, string) (int, error) {
		spawned++
		return 42, nil
	}
	env.Args = []string{"recover"}
	unlocker, err := h.Lock(filepath.Join(runsDir(env.Getenv), "recover.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlocker()
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	if spawned != 0 {
		t.Fatalf("spawned %d", spawned)
	}
	if !strings.Contains(stderr.String(), "already running") {
		t.Fatal(stderr.String())
	}
}

type readdirFailHost struct {
	*host.Memory
}

func (r *readdirFailHost) ReadDir(string) ([]host.Entry, error) {
	return nil, errors.New("disk gone")
}

func TestRecoverScanFailureFails(t *testing.T) {
	m := host.NewMemory()
	env, _, stderr := testEnv(t, m, nil, nil)
	env.Host = &readdirFailHost{Memory: m}
	env.Args = []string{"recover"}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
}

func TestRecoverLockFailureFails(t *testing.T) {
	h := host.NewMemory()
	env, _, stderr := testEnv(t, h, nil, nil)
	env.Lock = func(string) (func(), error) { return nil, errors.New("read-only file system") }
	env.Args = []string{"recover"}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d", code)
	}
	if strings.Contains(stderr.String(), "already running") {
		t.Fatal(stderr.String())
	}
}

func TestRecoverAllFinishedSaysNothing(t *testing.T) {
	h := host.NewMemory()
	env, _, stderr := testEnv(t, h, nil, nil)
	seedDetachedRun(t, h, env.Getenv, recoverID)
	markRunFinished(t, h, env.Getenv, recoverID)
	env.Args = []string{"recover"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "nothing to recover") {
		t.Fatal(stderr.String())
	}
}
