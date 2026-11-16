package crew

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

func verifyCrew(t *testing.T, command string, timeout time.Duration) (Env, func(string) string, *State) {
	t.Helper()
	env, getenv, _, _ := advEnv(t, t.TempDir())
	st := controlCrew(StatusFinished)
	st.Verify, st.VerifyTimeout = command, timeout
	st.Tasks[0].Worktree = t.TempDir()
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(crewsDir(getenv), "c1"), 0o700); err != nil {
		t.Fatal(err)
	}
	return env, getenv, st
}

func settleVerify(t *testing.T, env Env, st *State) {
	t.Helper()
	v := newVerifier(len(st.Tasks))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for {
		if err := v.step(ctx, env, st); err != nil {
			t.Fatal(err)
		}
		if !st.verifyPending(&st.Tasks[0]) {
			return
		}
		if err := v.sleep(ctx, time.Second); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVerifyVerdicts(t *testing.T) {
	cases := []struct {
		name    string
		command string
		timeout time.Duration
		check   string
		word    string
		output  string
	}{
		{"passes", "echo fine", 0, checkPassed, "verified", "fine\nexit code: 0"},
		{"fails", "echo bad; exit 3", 0, checkFailed, "verify failed", "bad\nexit code: 3"},
		{"times out", "sleep 5", 200 * time.Millisecond, checkFailed, "verify failed", "timed out after 200ms"},
		{"runs in the worktree", "pwd", 0, checkPassed, "verified", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, getenv, st := verifyCrew(t, tc.command, tc.timeout)
			settleVerify(t, env, st)
			task := &st.Tasks[0]
			if task.Check != tc.check || statusWord(task) != tc.word || task.Status != StatusFinished {
				t.Fatalf("task %+v word %q", task, statusWord(task))
			}
			data, err := os.ReadFile(verifyLogPath(getenv, "c1", "a"))
			if err != nil {
				t.Fatal(err)
			}
			log := string(data)
			want := tc.output
			if tc.name == "runs in the worktree" {
				want = filepath.Base(task.Worktree)
			}
			if !strings.HasPrefix(log, "$ "+tc.command+"\n") || !strings.Contains(log, want) {
				t.Fatalf("log %q", log)
			}
			saved, _ := load(getenv, "c1")
			if saved.Tasks[0].Check != tc.check {
				t.Fatalf("saved %+v", saved.Tasks[0])
			}
		})
	}
}

func TestVerifySkippedWithoutCommandOrBeforeFinish(t *testing.T) {
	cases := []struct {
		name    string
		command string
		status  Status
	}{
		{"no command", "", StatusFinished},
		{"still running", "echo x", StatusRunning},
		{"failed", "echo x", StatusFailed},
		{"stopped", "echo x", StatusStopped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, _, st := verifyCrew(t, tc.command, 0)
			st.Tasks[0].Status = tc.status
			if err := newVerifier(1).step(context.Background(), env, st); err != nil {
				t.Fatal(err)
			}
			if st.Tasks[0].Check != "" || len(newVerifier(1).running) != 0 {
				t.Fatalf("task %+v", st.Tasks[0])
			}
		})
	}
}

func TestCrewIsNotDoneUntilVerified(t *testing.T) {
	cases := []struct {
		check string
		done  bool
	}{{"", false}, {checkRunning, false}, {checkPassed, true}, {checkFailed, true}}
	for _, tc := range cases {
		st := controlCrew(StatusFinished)
		st.Verify = "true"
		st.Tasks[0].Check = tc.check
		if st.done() != tc.done {
			t.Fatalf("check %q done %v want %v", tc.check, st.done(), tc.done)
		}
	}
	st := controlCrew(StatusFinished)
	if !st.done() {
		t.Fatal("a crew without verify is done when its tasks are")
	}
}

func TestSuperviseRunsVerifyBeforeItReturns(t *testing.T) {
	env, getenv, st := verifyCrew(t, "echo from verify", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := supervise(ctx, env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Check != checkPassed {
		t.Fatalf("task %+v", st.Tasks[0])
	}
	saved, _ := load(getenv, "c1")
	if saved.Tasks[0].Check != checkPassed {
		t.Fatalf("saved %+v", saved.Tasks[0])
	}
}

func TestVerifyRestartsAfterASupervisorDied(t *testing.T) {
	env, _, st := verifyCrew(t, "echo again", 0)
	st.Tasks[0].Check = checkRunning
	settleVerify(t, env, st)
	if st.Tasks[0].Check != checkPassed {
		t.Fatalf("task %+v", st.Tasks[0])
	}
}

func TestVerifyShownInViewFollowAndReport(t *testing.T) {
	env, getenv, st := verifyCrew(t, "echo one; echo two", 0)
	svc := Service{Env: env}
	st.Tasks[0].RunID = "r1"
	st.Tasks[0].Branch = "crew/c1/a"
	seedEnd(t, getenv, "r1", "final")
	st.Tasks[0].Check = checkRunning
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	chunk, err := svc.Follow("c1", 0, 0)
	if err != nil || chunk.Ended {
		t.Fatalf("follow ended while verifying: %+v %v", chunk, err)
	}
	snap, _, _ := svc.View("c1")
	if snap.Rows[0].Status != "verifying" || snap.Done {
		t.Fatalf("row %+v done %v", snap.Rows[0], snap.Done)
	}
	settleVerify(t, env, st)
	chunk, err = svc.Follow("c1", 0, 0)
	if err != nil || !chunk.Ended {
		t.Fatalf("follow %+v %v", chunk, err)
	}
	trailer := strings.Join(chunk.Trailer, "\n")
	if !strings.Contains(trailer, "verified · echo one; echo two") || !strings.Contains(trailer, "one\ntwo\nexit code: 0") {
		t.Fatalf("trailer %q", trailer)
	}
	if finished := strings.Index(trailer, "finished · branch"); finished < 0 || finished > strings.Index(trailer, "verified ·") {
		t.Fatalf("verify output must follow the run: %q", trailer)
	}
	report, _ := svc.Report("c1")
	if joined := strings.Join(report, "\n"); !strings.Contains(joined, "verified · echo one") || !strings.Contains(joined, "two") {
		t.Fatalf("report %q", joined)
	}
	snap, _, _ = svc.View("c1")
	if snap.Rows[0].Status != "verified" {
		t.Fatalf("row %+v", snap.Rows[0])
	}
}

func TestVerifyOutputIsTrimmedToTheTail(t *testing.T) {
	env, getenv, st := verifyCrew(t, "true", 0)
	st.Tasks[0].Check = checkFailed
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("line\n")
	}
	if err := os.WriteFile(verifyLogPath(getenv, "c1", "a"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	lines := verifyLines(env, st, &st.Tasks[0])
	if len(lines) != verifyTailLines+2 || !strings.Contains(lines[1], "earlier lines in") {
		t.Fatalf("%d lines, second %q", len(lines), lines[1])
	}
}

func TestBriefCountsVerifying(t *testing.T) {
	st := controlCrew(StatusFinished, StatusRunning)
	st.Tasks[0].Check = checkRunning
	if got := st.brief(); got != "1 running · 1 verifying" {
		t.Fatalf("brief %q", got)
	}
}

func TestStartKeepsVerifyAndDefaultTimeout(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	writePlan(t, env.Cwd, "plan.md", "## one\nfirst\n")
	id, err := Service{Env: env}.Start(crewview.Start{Plan: "plan.md", N: 1, Verify: "  go test ./...  "})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := load(getenv, id)
	if st.Verify != "go test ./..." || st.VerifyTimeout != defaultVerifyTimeout {
		t.Fatalf("state %+v", st)
	}
}

func TestRunFlagsVerify(t *testing.T) {
	env, _, _ := serviceEnv(t)
	var errOut bytes.Buffer
	env.Stderr = &errOut
	o, rest, err := parseRunFlags(env, []string{"--verify", "make test", "--verify-timeout", "90s", "plan.md"})
	if err != nil || o.verify != "make test" || o.verifyTimeout != 90*time.Second || len(rest) != 1 {
		t.Fatalf("%+v %v %v", o, rest, err)
	}
	o, _, _ = parseRunFlags(env, []string{"plan.md"})
	if o.verify != "" || o.verifyTimeout != defaultVerifyTimeout {
		t.Fatalf("defaults %+v", o)
	}
}

func TestLastVerifyIsTheNewestForThisRepo(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	save := func(id, repo, verify string) {
		st := controlCrew(StatusFinished)
		st.ID, st.Repo, st.Verify = id, repo, verify
		if err := st.save(getenv); err != nil {
			t.Fatal(err)
		}
	}
	svc := Service{Env: env}
	if got := svc.LastVerify(); got != "" {
		t.Fatalf("empty state gave %q", got)
	}
	save("20260101-000000-aaaa", "/repo", "old check")
	save("20260102-000000-bbbb", "/other", "other repo check")
	save("20260103-000000-cccc", "/repo", "")
	save("20260104-000000-dddd", "/repo", "newest check")
	save("20260105-000000-eeee", "/other", "newer other repo check")
	if got := svc.LastVerify(); got != "newest check" {
		t.Fatalf("got %q", got)
	}
	svc.Env.Git = func(string, ...string) (string, error) { return "", os.ErrNotExist }
	if got := svc.LastVerify(); got != "" {
		t.Fatalf("outside a repo gave %q", got)
	}
}
