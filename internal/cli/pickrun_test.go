package cli

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func seedThreeRuns(t *testing.T, h *host.Memory, getenv func(string) string) {
	t.Helper()
	seedDirRun(t, h, getenv, "20260101-000000-aaaa", "/proj", true)
	seedDirRun(t, h, getenv, "20260102-000000-bbbb", "/proj", true)
	seedDirRun(t, h, getenv, "20260103-000000-cccc", "/elsewhere", true)
	seedDirRun(t, h, getenv, "20260104-000000-dddd", "/proj", true)
}

func TestPickerListsRunsInThisDirectoryNewestFirst(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, nil, nil)
	seedThreeRuns(t, h, env.Getenv)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-r"}
	code := startHP(t, env)
	term.await(t, "old 20260104-000000-dddd", 1)
	screen := term.text()
	newest, older := strings.Index(screen, "20260104-000000-dddd"), strings.Index(screen, "20260101-000000-aaaa")
	if newest < 0 || older < 0 || newest > older || strings.Contains(screen, "20260103-000000-cccc") {
		t.Fatalf("picker %q", screen)
	}
	term.send(t, "\x1b")
	awaitCode(t, code)
}

func TestPickerEnterResumesTheNewestRun(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("more")}}
	env, _, _ := testEnv(t, h, fp, nil)
	seedThreeRuns(t, h, env.Getenv)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-r"}
	code := startHP(t, env)
	term.await(t, "old 20260104-000000-dddd", 1)
	term.send(t, "\r")
	term.await(t, "continuing 20260104-000000-dddd", 1)
	term.send(t, "next\r")
	term.await(t, "0 steps", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	want := []string{"old 20260104-000000-dddd", "answer", "next"}
	if got := userTexts(fp.Requests[0].Messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("history %q", got)
	}
}

func TestPickerResumesAnOlderRunAfterArrowDown(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("more")}}
	env, _, _ := testEnv(t, h, fp, nil)
	seedThreeRuns(t, h, env.Getenv)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-r"}
	code := startHP(t, env)
	term.await(t, "old 20260104-000000-dddd", 1)
	term.send(t, "\x1b[B\x1b[B\r")
	term.await(t, "continuing 20260101-000000-aaaa", 1)
	term.send(t, "next\r")
	term.await(t, "0 steps", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	want := []string{"old 20260101-000000-aaaa", "answer", "next"}
	if got := userTexts(fp.Requests[0].Messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("history %q", got)
	}
}

func TestPickerSkippedWithASingleRun(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, nil, nil)
	seedDirRun(t, h, env.Getenv, "20260101-000000-aaaa", "/proj", true)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-r"}
	code := startHP(t, env)
	term.await(t, "continuing 20260101-000000-aaaa", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
}

func TestPickerShowsALiveRunAsAnError(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, nil, nil)
	seedThreeRuns(t, h, env.Getenv)
	if err := writeRunPidFor(env, "20260104-000000-dddd", 99); err != nil {
		t.Fatal(err)
	}
	env.Alive = func(int) bool { return true }
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-r"}
	code := startHP(t, env)
	term.await(t, "old 20260104-000000-dddd", 1)
	term.send(t, "\r")
	term.await(t, "is running", 1)
	awaitCode(t, code)
}

func TestContinueFlagsConflict(t *testing.T) {
	env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
	env.Term = newFakeTerm(t)
	env.Args = []string{"-c", "-r"}
	if code := Main(context.Background(), env); code != 2 || !strings.Contains(stderr.String(), "-c or -r") {
		t.Fatalf("code %d stderr %q", code, stderr.String())
	}
}

func TestRunAge(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		id   string
		want string
	}{
		{"20260110-115950-aaaa", "now"},
		{"20260110-115500-aaaa", "5m ago"},
		{"20260110-090000-aaaa", "3h ago"},
		{"20260107-120000-aaaa", "3d ago"},
		{"junk", ""},
	}
	for _, tc := range tests {
		if got := runAge(tc.id, now); got != tc.want {
			t.Errorf("runAge(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}
