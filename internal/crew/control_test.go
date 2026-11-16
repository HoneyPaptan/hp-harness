package crew

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

func TestEnqueueLosesNothingUnderConcurrentWriters(t *testing.T) {
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": t.TempDir(), "HOME": "/h"})
	const writers = 40
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := enqueue(getenv, "c1", request{Op: opStop, Task: "a"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got := queuedRequests(getenv, "c1")
	if len(got) != writers {
		t.Fatalf("queued %d want %d", len(got), writers)
	}
	for _, q := range got {
		if !q.ok || q.req.Op != opStop || q.req.Task != "a" {
			t.Fatalf("request %+v", q)
		}
	}
}

func TestQueuedRequestsKeepOrderAndSkipTempFiles(t *testing.T) {
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": t.TempDir(), "HOME": "/h"})
	for _, r := range []request{{Op: opPause}, {Op: opUnpause}, {Op: opStop, Task: "x"}} {
		if err := enqueue(getenv, "c1", r); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(controlDir(getenv, "c1"), "half.tmp"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	var ops []op
	for _, q := range queuedRequests(getenv, "c1") {
		ops = append(ops, q.req.Op)
	}
	if want := []op{opPause, opUnpause, opStop}; !reflect.DeepEqual(ops, want) {
		t.Fatalf("ops %v want %v", ops, want)
	}
}

func controlCrew(statuses ...Status) *State {
	st := &State{ID: "c1", Plan: "/p", Repo: "/repo", Base: "b", Concurrency: 2}
	for i, s := range statuses {
		slug := string(rune('a' + i))
		st.Tasks = append(st.Tasks, Task{Slug: slug, Title: "task " + slug, Prompt: "p", Status: s})
	}
	return st
}

func TestApplyControlsPause(t *testing.T) {
	cases := []struct {
		name   string
		start  bool
		ops    []op
		paused bool
	}{
		{"pause", false, []op{opPause}, true},
		{"unpause", true, []op{opUnpause}, false},
		{"pause twice stays paused", false, []op{opPause, opPause}, true},
		{"unpause when running does nothing", false, []op{opUnpause}, false},
		{"last request wins", false, []op{opPause, opUnpause, opPause}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, getenv, _, _ := advEnv(t, t.TempDir())
			st := controlCrew(StatusRunning)
			st.Paused = tc.start
			if err := st.save(getenv); err != nil {
				t.Fatal(err)
			}
			for _, o := range tc.ops {
				if err := enqueue(getenv, "c1", request{Op: o}); err != nil {
					t.Fatal(err)
				}
			}
			if err := applyControls(env, st); err != nil {
				t.Fatal(err)
			}
			if st.Paused != tc.paused {
				t.Fatalf("paused %v want %v", st.Paused, tc.paused)
			}
			saved, err := load(getenv, "c1")
			if err != nil || saved.Paused != tc.paused {
				t.Fatalf("saved %+v %v", saved, err)
			}
			if hasControls(getenv, "c1") {
				t.Fatal("requests left behind")
			}
		})
	}
}

func TestPausedCrewStartsNothingButSettlesRunning(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	st := controlCrew(StatusRunning, StatusQueued)
	st.Tasks[0].RunID = "r1"
	st.Paused = true
	seedEnd(t, getenv, "r1", "final")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusFinished || st.Tasks[1].Status != StatusQueued || len(hp.calls) != 0 {
		t.Fatalf("tasks %+v calls %v", st.Tasks, hp.calls)
	}
	st.Paused = false
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[1].Status != StatusRunning || len(hp.calls) != 1 {
		t.Fatalf("after unpause %+v calls %v", st.Tasks, hp.calls)
	}
}

func TestApplyStop(t *testing.T) {
	cases := []struct {
		name     string
		status   Status
		alive    bool
		want     Status
		signaled bool
	}{
		{"running run is signalled", StatusRunning, true, StatusStopped, true},
		{"running but already dead", StatusRunning, false, StatusStopped, false},
		{"queued never starts", StatusQueued, false, StatusStopped, false},
		{"finished is left alone", StatusFinished, false, StatusFinished, false},
		{"failed is left alone", StatusFailed, false, StatusFailed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, getenv, _, _ := advEnv(t, t.TempDir())
			var signalled []int
			env.Alive = func(pid int) bool { return tc.alive && pid == 4242 }
			env.Signal = func(pid int) error { signalled = append(signalled, pid); return nil }
			st := controlCrew(tc.status)
			st.Tasks[0].RunID = "r1"
			if err := os.MkdirAll(runsDir(getenv), 0o700); err != nil {
				t.Fatal(err)
			}
			markLive(t, getenv, "r1")
			if err := enqueue(getenv, "c1", request{Op: opStop, Task: "a"}); err != nil {
				t.Fatal(err)
			}
			if err := applyControls(env, st); err != nil {
				t.Fatal(err)
			}
			if st.Tasks[0].Status != tc.want {
				t.Fatalf("status %s want %s", st.Tasks[0].Status, tc.want)
			}
			if got := len(signalled) == 1 && signalled[0] == 4242; got != tc.signaled {
				t.Fatalf("signalled %v want %v", signalled, tc.signaled)
			}
		})
	}
}

func TestApplyRetry(t *testing.T) {
	cases := []struct {
		name      string
		status    Status
		seed      func(*testing.T, func(string) string)
		alive     bool
		want      Status
		wantRun   string
		wantKept  bool
		crashed   bool
		wantFresh bool
	}{
		{"crash loop resumes the same run with a clean counter", StatusFailed, func(t *testing.T, g func(string) string) { writeUnfinished(t, g, "r1") }, false, StatusRunning, "r1", false, true, false},
		{"stopped run starts a fresh run", StatusStopped, func(t *testing.T, g func(string) string) { writeStopped(t, g, "r1") }, false, StatusQueued, "", false, false, true},
		{"never launched starts fresh", StatusFailed, func(*testing.T, func(string) string) {}, false, StatusQueued, "", false, false, true},
		{"old run still exiting waits", StatusStopped, func(t *testing.T, g func(string) string) { writeStopped(t, g, "r1"); markLive(t, g, "r1") }, true, StatusStopped, "r1", true, false, false},
		{"finished is left alone", StatusFinished, func(*testing.T, func(string) string) {}, false, StatusFinished, "r1", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, getenv, _, _ := advEnv(t, t.TempDir())
			env.Alive = func(int) bool { return tc.alive }
			st := controlCrew(tc.status)
			st.Tasks[0].RunID = "r1"
			st.Tasks[0].EndReason = "keeps crashing"
			if tc.name == "never launched starts fresh" {
				st.Tasks[0].RunID = ""
			}
			tc.seed(t, getenv)
			if tc.crashed {
				if err := saveResumeState(env, "r1", journal.ResumeState{Records: 2, Attempts: journal.CrashLimit}); err != nil {
					t.Fatal(err)
				}
			}
			if err := enqueue(getenv, "c1", request{Op: opRetry, Task: "a"}); err != nil {
				t.Fatal(err)
			}
			if err := applyControls(env, st); err != nil {
				t.Fatal(err)
			}
			got := st.Tasks[0]
			if got.Status != tc.want || got.RunID != tc.wantRun {
				t.Fatalf("task %+v want %s run %q", got, tc.want, tc.wantRun)
			}
			if hasControls(getenv, "c1") != tc.wantKept {
				t.Fatalf("kept %v want %v", hasControls(getenv, "c1"), tc.wantKept)
			}
			if tc.crashed && loadResumeState(env, "r1") != (journal.ResumeState{}) {
				t.Fatalf("crash counter not reset: %+v", loadResumeState(env, "r1"))
			}
			if (tc.wantFresh || tc.crashed) && got.EndReason != "" {
				t.Fatalf("end reason kept: %q", got.EndReason)
			}
		})
	}
}

func TestRetryAfterCrashLoopResumesTheRun(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	st := controlCrew(StatusFailed)
	st.Tasks[0].RunID = "r1"
	writeUnfinished(t, getenv, "r1")
	if err := saveResumeState(env, "r1", journal.ResumeState{Records: 2, Attempts: journal.CrashLimit}); err != nil {
		t.Fatal(err)
	}
	if err := enqueue(getenv, "c1", request{Op: opRetry, Task: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := applyControls(env, st); err != nil {
		t.Fatal(err)
	}
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if len(hp.calls) != 1 || hp.calls[0][0] != "resume" || hp.calls[0][2] != "r1" || st.Tasks[0].Status != StatusRunning {
		t.Fatalf("calls %v task %+v", hp.calls, st.Tasks[0])
	}
}

func TestRetryStoppedStartsFreshRunInTheSameWorktree(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	st := controlCrew(StatusStopped)
	st.Tasks[0].RunID = "r1"
	st.Tasks[0].Worktree = t.TempDir()
	if err := os.MkdirAll(filepath.Join(st.Tasks[0].Worktree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeStopped(t, getenv, "r1")
	if err := enqueue(getenv, "c1", request{Op: opRetry, Task: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := applyControls(env, st); err != nil {
		t.Fatal(err)
	}
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if len(hp.calls) != 1 || hp.calls[0][0] != "run" || hp.calls[0][3] != st.Tasks[0].Worktree {
		t.Fatalf("calls %v", hp.calls)
	}
	if st.Tasks[0].RunID != "run-1" || st.Tasks[0].Status != StatusRunning {
		t.Fatalf("task %+v", st.Tasks[0])
	}
}

func TestApplyControlsDropsUnusableRequests(t *testing.T) {
	env, getenv, _, _ := advEnv(t, t.TempDir())
	st := controlCrew(StatusRunning)
	for _, r := range []request{{Op: opStop, Task: "nope"}, {Op: opRetry, Task: "a"}, {Op: "explode"}} {
		if err := enqueue(getenv, "c1", r); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(controlDir(getenv, "c1"), "9-bad.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := applyControls(env, st); err != nil {
		t.Fatal(err)
	}
	if hasControls(getenv, "c1") || st.Tasks[0].Status != StatusRunning {
		t.Fatalf("left %v task %+v", hasControls(getenv, "c1"), st.Tasks[0])
	}
}

func TestSuperviseAppliesRequestsBeforeItDecidesItIsDone(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	st := controlCrew(StatusFailed)
	st.Tasks[0].Worktree = t.TempDir()
	if err := os.MkdirAll(filepath.Join(st.Tasks[0].Worktree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := enqueue(getenv, "c1", request{Op: opRetry, Task: "a"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := supervise(ctx, env, st); err != context.DeadlineExceeded {
		t.Fatalf("supervise returned %v", err)
	}
	if st.Tasks[0].Status != StatusRunning || len(hp.calls) != 1 {
		t.Fatalf("task %+v calls %v", st.Tasks[0], hp.calls)
	}
}

func TestRunSupervisorTakesRequestsThatArriveAsItExits(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	env.Pid = 7
	st := controlCrew(StatusFinished)
	st.Tasks[0].Status = StatusFinished
	st.Tasks[0].Worktree = t.TempDir()
	if err := os.MkdirAll(filepath.Join(st.Tasks[0].Worktree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	if err := enqueue(getenv, "c1", request{Op: opPause}); err != nil {
		t.Fatal(err)
	}
	if err := runSupervisor(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if hasControls(getenv, "c1") || !st.Paused || len(hp.calls) != 0 {
		t.Fatalf("paused %v calls %v", st.Paused, hp.calls)
	}
}

func controlService(t *testing.T) (Service, func(string) string, *[]spawnCall) {
	t.Helper()
	env, getenv, calls := serviceEnv(t)
	return Service{Env: env}, getenv, calls
}

func TestServiceControlsValidateAndSubmit(t *testing.T) {
	cases := []struct {
		name     string
		statuses []Status
		paused   bool
		do       func(Service) error
		wantErr  string
		wantOp   op
	}{
		{"stop a running task", []Status{StatusRunning}, false, func(s Service) error { return s.Stop("c1", "ta") }, "", opStop},
		{"stop by number", []Status{StatusQueued, StatusRunning}, false, func(s Service) error { return s.Stop("c1", "2") }, "", opStop},
		{"stop by title", []Status{StatusRunning}, false, func(s Service) error { return s.Stop("c1", "task a") }, "", opStop},
		{"stop a finished task", []Status{StatusFinished}, false, func(s Service) error { return s.Stop("c1", "ta") }, "is not running", ""},
		{"stop an unknown task", []Status{StatusRunning}, false, func(s Service) error { return s.Stop("c1", "zz") }, "has no task", ""},
		{"retry a failed task", []Status{StatusFailed}, false, func(s Service) error { return s.Retry("c1", "ta") }, "", opRetry},
		{"retry a stopped task", []Status{StatusStopped}, false, func(s Service) error { return s.Retry("c1", "ta") }, "", opRetry},
		{"retry a running task", []Status{StatusRunning}, false, func(s Service) error { return s.Retry("c1", "ta") }, "did not fail or stop", ""},
		{"pause", []Status{StatusRunning}, false, func(s Service) error { return s.Pause("c1") }, "", opPause},
		{"pause twice", []Status{StatusRunning}, true, func(s Service) error { return s.Pause("c1") }, "already paused", ""},
		{"pause a done crew", []Status{StatusFinished}, false, func(s Service) error { return s.Pause("c1") }, "is done", ""},
		{"unpause", []Status{StatusRunning}, true, func(s Service) error { return s.Unpause("c1") }, "", opUnpause},
		{"unpause when not paused", []Status{StatusRunning}, false, func(s Service) error { return s.Unpause("c1") }, "is not paused", ""},
		{"unknown crew", nil, false, func(s Service) error { return s.Pause("nope") }, "no such file", ""},
		{"unsafe crew id", nil, false, func(s Service) error { return s.Pause("../x") }, "not found", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, getenv, _ := controlService(t)
			if tc.statuses != nil {
				st := seedCrew(t, getenv, "c1", "", "/p", tc.statuses...)
				st.Paused = tc.paused
				if err := st.save(getenv); err != nil {
					t.Fatal(err)
				}
			}
			err := tc.do(svc)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err %v want %q", err, tc.wantErr)
				}
				if hasControls(getenv, "c1") {
					t.Fatal("a refused request was queued")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			reqs := queuedRequests(getenv, "c1")
			if len(reqs) != 1 || reqs[0].req.Op != tc.wantOp {
				t.Fatalf("queued %+v", reqs)
			}
		})
	}
}

func TestServiceControlStartsSupervisorOnlyWhenNoneRuns(t *testing.T) {
	cases := []struct {
		name  string
		alive bool
		spawn int
	}{
		{"no supervisor", false, 1},
		{"supervisor running", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, getenv, calls := controlService(t)
			svc.Env.Alive = func(int) bool { return tc.alive }
			seedCrew(t, getenv, "c1", "", "/p", StatusFailed)
			if err := os.WriteFile(pidPath(getenv, "c1"), []byte("4242"), 0o600); err != nil {
				t.Fatal(err)
			}
			if !tc.alive {
				_ = os.Remove(pidPath(getenv, "c1"))
			}
			if err := svc.Retry("c1", "ta"); err != nil {
				t.Fatal(err)
			}
			if len(*calls) != tc.spawn {
				t.Fatalf("spawns %v want %d", *calls, tc.spawn)
			}
			if tc.spawn == 1 && !reflect.DeepEqual((*calls)[0].args, []string{"crew", "resume", "c1"}) {
				t.Fatalf("spawn %v", (*calls)[0].args)
			}
		})
	}
}

func TestServiceRefusesAnotherRequestForATaskWithOneWaiting(t *testing.T) {
	svc, getenv, _ := controlService(t)
	seedCrew(t, getenv, "c1", "", "/p", StatusRunning)
	if err := svc.Stop("c1", "ta"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Stop("c1", "ta"); err == nil || !strings.Contains(err.Error(), "already has a request waiting") {
		t.Fatalf("err %v", err)
	}
}

func TestViewShowsPendingRequestsAndPause(t *testing.T) {
	svc, getenv, _ := controlService(t)
	seedCrew(t, getenv, "c1", "goal", "/p", StatusFinished, StatusRunning, StatusFailed)
	snap, _, err := svc.View("c1")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{snap.Rows[0].Keys, snap.Rows[1].Keys, snap.Rows[2].Keys}
	if want := []string{"dax", "s", "rdx"}; !reflect.DeepEqual(keys, want) || snap.Paused || snap.Done {
		t.Fatalf("keys %v paused %v done %v", keys, snap.Paused, snap.Done)
	}
	if err := svc.Stop("c1", "tb"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Retry("c1", "tc"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Pause("c1"); err != nil {
		t.Fatal(err)
	}
	snap, _, err = svc.View("c1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Rows[1].Status != "stopping" || snap.Rows[2].Status != "retrying" || snap.Rows[1].Keys != "" || snap.Rows[2].Keys != "" || !snap.Paused {
		t.Fatalf("snapshot %+v", snap)
	}
}

func TestViewIsNotDoneWhileARequestWaits(t *testing.T) {
	svc, getenv, _ := controlService(t)
	seedCrew(t, getenv, "c1", "", "/p", StatusFailed)
	if err := svc.Retry("c1", "ta"); err != nil {
		t.Fatal(err)
	}
	snap, _, _ := svc.View("c1")
	if snap.Done {
		t.Fatal("view stopped refreshing before the request was applied")
	}
}

func TestPausedShowsInStatusBriefAndList(t *testing.T) {
	svc, getenv, _ := controlService(t)
	st := seedCrew(t, getenv, "c1", "goal", "/p", StatusRunning, StatusQueued)
	st.Paused = true
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	lines, err := svc.Status("c1")
	if err != nil || lines[0] != "paused" {
		t.Fatalf("status %v %v", lines, err)
	}
	brief, _, err := svc.Brief("c1")
	if err != nil || !strings.Contains(brief, "· paused") {
		t.Fatalf("brief %q %v", brief, err)
	}
	list, _ := svc.List()
	if !strings.HasSuffix(list[0], "paused") {
		t.Fatalf("list %v", list)
	}
}

func TestCommandsQueueRequests(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
		op     op
	}{
		{"stop", []string{"crew", "stop", "c1", "ta"}, 0, "stopping ta\n", "", opStop},
		{"retry", []string{"crew", "retry", "c1", "tb"}, 0, "retrying tb\n", "", opRetry},
		{"pause", []string{"crew", "pause", "c1"}, 0, "pausing crew c1\n", "", opPause},
		{"unpause refused", []string{"crew", "unpause", "c1"}, 1, "", "is not paused", ""},
		{"stop needs both arguments", []string{"crew", "stop", "c1"}, 2, "", "crew id and task required", ""},
		{"pause needs an id", []string{"crew", "pause"}, 2, "", "crew id required", ""},
		{"retry a running task", []string{"crew", "retry", "c1", "ta"}, 1, "", "did not fail or stop", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, getenv, _ := serviceEnv(t)
			seedCrew(t, getenv, "c1", "", "/p", StatusRunning, StatusFailed)
			var out, errOut bytes.Buffer
			env.Stdout, env.Stderr = &out, &errOut
			if code := Main(context.Background(), env, tc.args); code != tc.code {
				t.Fatalf("code %d want %d stderr %q", code, tc.code, errOut.String())
			}
			if out.String() != tc.stdout || !strings.Contains(errOut.String(), tc.stderr) {
				t.Fatalf("stdout %q stderr %q", out.String(), errOut.String())
			}
			reqs := queuedRequests(getenv, "c1")
			if (tc.op == "") != (len(reqs) == 0) || len(reqs) > 0 && reqs[0].req.Op != tc.op {
				t.Fatalf("queued %+v", reqs)
			}
		})
	}
}
