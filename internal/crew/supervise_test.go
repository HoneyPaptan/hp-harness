package crew

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type fakeHP struct {
	calls [][]string
	n     int
}

func (f *fakeHP) run(_ context.Context, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if len(args) > 0 && args[0] == "run" {
		f.n++
		return fmt.Sprintf("run-%d\nattach with: hp attach run-%d\n", f.n, f.n), nil
	}
	return "", nil
}

type fakeGit struct {
	calls [][]string
}

func (f *fakeGit) run(dir string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{dir}, args...))
	return "", nil
}

func advEnv(t *testing.T, state string) (Env, func(string) string, *fakeHP, *fakeGit) {
	t.Helper()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": state, "HOME": "/h"})
	hp := &fakeHP{}
	git := &fakeGit{}
	env := Env{
		Getenv: getenv, Cwd: "/proj",
		Alive: func(int) bool { return false },
		RunHP: hp.run, Git: git.run,
	}
	return env, getenv, hp, git
}

func seedEnd(t *testing.T, getenv func(string) string, id, reason string) {
	t.Helper()
	dir := runsDir(getenv)
	w, err := journal.Create(host.Native{}, filepath.Join(dir, id+".jsonl"), journal.RunStart{RunID: id, Dir: "/w"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("p"); err != nil {
		t.Fatal(err)
	}
	if err := w.End(reason); err != nil {
		t.Fatal(err)
	}
}

func TestAdvanceStartsThenFinishes(t *testing.T) {
	state := t.TempDir()
	env, getenv, hp, _ := advEnv(t, state)
	st := newState(getenv, "c1", "/p/plan.md", "/repo", "base", runFlags{n: 2}, []planTask{
		{title: "One", prompt: "one", slug: "one"},
		{title: "Two", prompt: "two", slug: "two"},
	})
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.activeCount() != 2 {
		t.Fatalf("running %d", st.activeCount())
	}
	for i := range st.Tasks {
		if st.Tasks[i].Status != StatusRunning || st.Tasks[i].RunID == "" {
			t.Fatalf("task %d %+v", i, st.Tasks[i])
		}
	}
	if len(hp.calls) != 2 {
		t.Fatalf("hp calls %d", len(hp.calls))
	}
	seedEnd(t, getenv, st.Tasks[0].RunID, "final")
	seedEnd(t, getenv, st.Tasks[1].RunID, "final")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if !st.done() || st.doneCount() != 2 {
		t.Fatalf("done %v %d", st.done(), st.doneCount())
	}
	for i := range st.Tasks {
		if st.Tasks[i].Status != StatusFinished {
			t.Fatalf("task %d status %s", i, st.Tasks[i].Status)
		}
	}
}

func TestAdvanceConcurrencyLimit(t *testing.T) {
	state := t.TempDir()
	env, getenv, _, _ := advEnv(t, state)
	st := newState(getenv, "c1", "/p", "/repo", "base", runFlags{n: 1}, []planTask{
		{title: "One", prompt: "one", slug: "one"},
		{title: "Two", prompt: "two", slug: "two"},
	})
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.activeCount() != 1 || st.Tasks[1].Status != StatusQueued {
		t.Fatalf("running %d queued %s", st.activeCount(), st.Tasks[1].Status)
	}
	seedEnd(t, getenv, st.Tasks[0].RunID, "final")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.activeCount() != 1 || st.Tasks[0].Status != StatusFinished || st.Tasks[1].Status != StatusRunning {
		t.Fatalf("after finish %+v", st.Tasks)
	}
}

func TestAdvanceResumesDeadRun(t *testing.T) {
	state := t.TempDir()
	env, getenv, hp, _ := advEnv(t, state)
	st := &State{ID: "c1", Plan: "/p", Repo: "/repo", Base: "base", Concurrency: 1,
		Tasks: []Task{{Title: "Dead", Slug: "dead", Prompt: "p", RunID: "old", Status: StatusRunning}}}
	writeUnfinished(t, getenv, "old")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusRunning {
		t.Fatalf("status %s", st.Tasks[0].Status)
	}
	if len(hp.calls) != 1 || hp.calls[0][0] != "resume" || hp.calls[0][2] != "old" {
		t.Fatalf("calls %v", hp.calls)
	}
}

func writeUnfinished(t *testing.T, getenv func(string) string, id string) {
	t.Helper()
	w, err := journal.Create(host.Native{}, filepath.Join(runsDir(getenv), id+".jsonl"), journal.RunStart{RunID: id, Dir: "/w"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("p"); err != nil {
		t.Fatal(err)
	}
}

func TestAdvanceDeadResumeFailureFails(t *testing.T) {
	state := t.TempDir()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": state, "HOME": "/h"})
	env := Env{
		Getenv: getenv, Cwd: "/proj",
		Alive: func(int) bool { return false },
		RunHP: func(context.Context, ...string) (string, error) {
			return "", fmt.Errorf("resume failed")
		},
		Git: func(string, ...string) (string, error) { return "", nil },
	}
	st := &State{ID: "c1", Plan: "/p", Repo: "/repo", Base: "b", Concurrency: 1,
		Tasks: []Task{{Title: "Dead", Slug: "d", Prompt: "p", RunID: "old", Status: StatusRunning}}}
	writeUnfinished(t, getenv, "old")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusFailed || !strings.Contains(st.Tasks[0].EndReason, "resume failed") {
		t.Fatalf("task %+v", st.Tasks[0])
	}
}

func TestStartFailureFails(t *testing.T) {
	state := t.TempDir()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": state, "HOME": "/h"})
	env := Env{
		Getenv: getenv, Cwd: "/proj",
		Alive: func(int) bool { return false },
		RunHP: func(context.Context, ...string) (string, error) {
			return "", fmt.Errorf("no key")
		},
		Git: func(string, ...string) (string, error) { return "", nil },
	}
	st := newState(getenv, "c1", "/p", "/repo", "b", runFlags{n: 1}, []planTask{
		{title: "One", prompt: "one", slug: "one"},
	})
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusFailed || !strings.Contains(st.Tasks[0].EndReason, "no key") {
		t.Fatalf("task %+v", st.Tasks[0])
	}
}

func TestFinalAnswer(t *testing.T) {
	st := journal.State{Messages: []provider.Message{
		{Role: provider.RoleUser, Text: "p"},
		{Role: provider.RoleAssistant, Text: "tool time"},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{CallID: "1", Text: "x"}}},
		{Role: provider.RoleAssistant, Text: "  the answer  "},
	}}
	if got := finalAnswer(st); got != "the answer" {
		t.Fatalf("got %q", got)
	}
	empty := journal.State{Messages: []provider.Message{{Role: provider.RoleUser, Text: "p"}}}
	if got := finalAnswer(empty); got != "" {
		t.Fatalf("got %q", got)
	}
}

func seedAnswer(t *testing.T, getenv func(string) string, id, reason, answer string) {
	t.Helper()
	w, err := journal.Create(host.Native{}, filepath.Join(runsDir(getenv), id+".jsonl"), journal.RunStart{RunID: id, Dir: "/w"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("p"); err != nil {
		t.Fatal(err)
	}
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, Text: answer}); err != nil {
		t.Fatal(err)
	}
	if err := w.End(reason); err != nil {
		t.Fatal(err)
	}
}

func TestStatusAndReport(t *testing.T) {
	state := t.TempDir()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": state, "HOME": "/h"})
	env := Env{
		Getenv: getenv, Cwd: "/proj",
		Alive: func(int) bool { return false },
		Git: func(dir string, args ...string) (string, error) {
			if len(args) >= 1 && args[0] == "status" {
				return "?? b.txt\n", nil
			}
			if len(args) >= 2 && args[0] == "diff" && args[1] == "--stat" {
				return " b.txt | 1 +\n", nil
			}
			return "", nil
		},
	}
	st := newState(getenv, "c1", "/p/plan.md", "/repo", "base", runFlags{n: 2}, []planTask{
		{title: "Done", prompt: "done", slug: "done"},
		{title: "Later", prompt: "later", slug: "later"},
	})
	st.Tasks[0].Status = StatusFinished
	st.Tasks[0].RunID = "r1"
	st.Tasks[0].EndReason = "final"
	st.Tasks[1].Status = StatusQueued
	seedAnswer(t, getenv, "r1", "final", "all good")
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}

	var status, report bytes.Buffer
	env.Stdout = &status
	env.Args = []string{"crew", "status", "c1"}
	if code := Main(context.Background(), env, env.Args); code != 0 {
		t.Fatalf("status code %d", code)
	}
	line := status.String()
	if !strings.Contains(line, "Done  finished  r1  crew/c1/done  1 file") {
		t.Fatalf("status %q", line)
	}
	if !strings.Contains(line, "Later  queued  -  crew/c1/later  -") {
		t.Fatalf("status %q", line)
	}

	env.Stdout = &report
	env.Args = []string{"crew", "report", "c1"}
	if code := Main(context.Background(), env, env.Args); code != 0 {
		t.Fatalf("report code %d", code)
	}
	rep := report.String()
	if !strings.Contains(rep, "## Done") || !strings.Contains(rep, "all good") || !strings.Contains(rep, "b.txt") {
		t.Fatalf("report %q", rep)
	}
	if !strings.Contains(rep, "## Later") || !strings.Contains(rep, "queued") {
		t.Fatalf("report %q", rep)
	}
}

func TestCrewsList(t *testing.T) {
	state := t.TempDir()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": state, "HOME": "/h"})
	env := Env{Getenv: getenv, Stdout: &bytes.Buffer{}}
	st := newState(getenv, "c1", "/p/plan.md", "/repo", "b", runFlags{n: 2}, []planTask{
		{title: "One", prompt: "one", slug: "one"},
		{title: "Two", prompt: "two", slug: "two"},
	})
	st.Tasks[0].Status = StatusFinished
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	if code := cmdCrews(env, nil); code != 0 {
		t.Fatalf("code %d", code)
	}
	out := env.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, "c1  plan.md  1/2 done") {
		t.Fatalf("crews %q", out)
	}
}

func writeStopped(t *testing.T, getenv func(string) string, id string) {
	t.Helper()
	w, err := journal.Create(host.Native{}, filepath.Join(runsDir(getenv), id+".jsonl"), journal.RunStart{RunID: id, Dir: "/w"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("p"); err != nil {
		t.Fatal(err)
	}
	if err := w.Stop("user"); err != nil {
		t.Fatal(err)
	}
}

func seedRunDir(t *testing.T, getenv func(string) string, id, dir string) {
	t.Helper()
	w, err := journal.Create(host.Native{}, filepath.Join(runsDir(getenv), id+".jsonl"), journal.RunStart{RunID: id, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("p"); err != nil {
		t.Fatal(err)
	}
}

func TestAdvanceStoppedRunStaysStopped(t *testing.T) {
	state := t.TempDir()
	env, getenv, hp, _ := advEnv(t, state)
	st := &State{ID: "c1", Plan: "/p", Repo: "/repo", Base: "b", Concurrency: 1,
		Tasks: []Task{{Title: "S", Slug: "s", Prompt: "p", RunID: "old", Status: StatusRunning}}}
	writeStopped(t, getenv, "old")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusStopped {
		t.Fatalf("status %s", st.Tasks[0].Status)
	}
	if len(hp.calls) != 0 {
		t.Fatalf("hp called %v", hp.calls)
	}
	if !st.done() {
		t.Fatal("crew should be done")
	}
}

func TestAdvanceCrashLoopFailsAfterLimit(t *testing.T) {
	state := t.TempDir()
	env, getenv, hp, _ := advEnv(t, state)
	st := &State{ID: "c1", Plan: "/p", Repo: "/repo", Base: "b", Concurrency: 1,
		Tasks: []Task{{Title: "C", Slug: "c", Prompt: "p", RunID: "old", Status: StatusRunning}}}
	writeUnfinished(t, getenv, "old")
	for i := 0; i < journal.CrashLimit; i++ {
		if err := advance(context.Background(), env, st); err != nil {
			t.Fatal(err)
		}
		if st.Tasks[0].Status != StatusRunning {
			t.Fatalf("attempt %d status %s", i, st.Tasks[0].Status)
		}
	}
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusFailed || !strings.Contains(st.Tasks[0].EndReason, "keeps crashing after 3 resumes") {
		t.Fatalf("task %+v", st.Tasks[0])
	}
	if len(hp.calls) != journal.CrashLimit {
		t.Fatalf("hp calls %d want %d", len(hp.calls), journal.CrashLimit)
	}
	for _, c := range hp.calls {
		if c[0] != "resume" {
			t.Fatalf("call %v", c)
		}
	}
}

func TestSettleStartingAdoptsExistingRun(t *testing.T) {
	state := t.TempDir()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": state, "HOME": "/h"})
	env := Env{
		Getenv: getenv, Cwd: "/proj",
		Alive: func(int) bool { return false },
		RunHP: func(context.Context, ...string) (string, error) {
			t.Fatal("must not launch a second run")
			return "", nil
		},
		Git: func(string, ...string) (string, error) { return "", nil },
	}
	st := newState(getenv, "c1", "/p", "/repo", "b", runFlags{n: 1}, []planTask{
		{title: "One", prompt: "one", slug: "one"},
	})
	task := &st.Tasks[0]
	task.Status = StatusStarting
	seedRunDir(t, getenv, "r7", task.Worktree)
	if err := settleStarting(context.Background(), env, st, task); err != nil {
		t.Fatal(err)
	}
	if task.RunID != "r7" || task.Status != StatusRunning {
		t.Fatalf("task %+v", task)
	}
	reloaded, err := load(getenv, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Tasks[0].RunID != "r7" || reloaded.Tasks[0].Status != StatusRunning {
		t.Fatalf("saved task %+v", reloaded.Tasks[0])
	}
}

func TestSettleStartingLaunchesWhenNoRun(t *testing.T) {
	state := t.TempDir()
	env, getenv, hp, _ := advEnv(t, state)
	st := newState(getenv, "c1", "/p", "/repo", "b", runFlags{n: 1}, []planTask{
		{title: "One", prompt: "one", slug: "one"},
	})
	task := &st.Tasks[0]
	task.Status = StatusStarting
	if err := settleStarting(context.Background(), env, st, task); err != nil {
		t.Fatal(err)
	}
	if task.RunID != "run-1" || task.Status != StatusRunning {
		t.Fatalf("task %+v", task)
	}
	if len(hp.calls) != 1 || hp.calls[0][0] != "run" {
		t.Fatalf("calls %v", hp.calls)
	}
}

func tearTail(t *testing.T, getenv func(string) string, id string) {
	t.Helper()
	p := filepath.Join(runsDir(getenv), id+".jsonl")
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(`{"seq":9,"ts":"x","type":"tool_result","data":{"result":{"Text":"partial`); err != nil {
		t.Fatal(err)
	}
}

func markLive(t *testing.T, getenv func(string) string, id string) {
	t.Helper()
	p := filepath.Join(runsDir(getenv), id+".pid")
	if err := os.WriteFile(p, []byte("4242"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runningCrew(id string) *State {
	return &State{ID: "c1", Plan: "/p", Repo: "/repo", Base: "b", Concurrency: 1,
		Tasks: []Task{{Title: "T", Slug: "t", Prompt: "p", RunID: id, Status: StatusRunning}}}
}

func TestAdvanceTornTailLiveRunWaits(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	env.Alive = func(pid int) bool { return pid == 4242 }
	st := runningCrew("old")
	writeUnfinished(t, getenv, "old")
	tearTail(t, getenv, "old")
	markLive(t, getenv, "old")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusRunning || len(hp.calls) != 0 {
		t.Fatalf("task %+v calls %v", st.Tasks[0], hp.calls)
	}
}

func TestAdvanceTornTailDeadRunResumes(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	st := runningCrew("old")
	writeUnfinished(t, getenv, "old")
	tearTail(t, getenv, "old")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusRunning || len(hp.calls) != 1 || hp.calls[0][0] != "resume" {
		t.Fatalf("task %+v calls %v", st.Tasks[0], hp.calls)
	}
}

func TestAdvanceUnreadableJournalLiveRunWaits(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	env.Alive = func(pid int) bool { return pid == 4242 }
	st := runningCrew("old")
	p := filepath.Join(runsDir(getenv), "old.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{\"seq\":0,\"ts\":\"x\",\"typ"), 0o600); err != nil {
		t.Fatal(err)
	}
	markLive(t, getenv, "old")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusRunning || len(hp.calls) != 0 {
		t.Fatalf("task %+v calls %v", st.Tasks[0], hp.calls)
	}
}

func TestAdvanceUnreadableJournalDeadRunIsBounded(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	st := runningCrew("old")
	p := filepath.Join(runsDir(getenv), "old.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{\"seq\":0,\"ts\":\"x\",\"typ"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < journal.CrashLimit+2; i++ {
		if err := advance(context.Background(), env, st); err != nil {
			t.Fatal(err)
		}
	}
	if st.Tasks[0].Status != StatusFailed || !strings.Contains(st.Tasks[0].EndReason, "keeps crashing") {
		t.Fatalf("task %+v", st.Tasks[0])
	}
	if len(hp.calls) != journal.CrashLimit {
		t.Fatalf("hp calls %d want %d", len(hp.calls), journal.CrashLimit)
	}
}

func TestAdvanceRecoverLockHeldSkipsResume(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	st := runningCrew("old")
	writeUnfinished(t, getenv, "old")
	unlock, err := host.Native{}.Lock(recoverLockPath(getenv))
	if err != nil {
		t.Fatal(err)
	}
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusRunning || len(hp.calls) != 0 {
		t.Fatalf("task %+v calls %v", st.Tasks[0], hp.calls)
	}
	if _, err := os.Stat(resumeStatePath(getenv, "old")); err == nil {
		t.Fatal("resume state written while lock was held")
	}
	unlock()
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if len(hp.calls) != 1 || hp.calls[0][0] != "resume" {
		t.Fatalf("calls after unlock %v", hp.calls)
	}
}

func TestAdvanceRechecksAliveUnderLock(t *testing.T) {
	env, getenv, hp, _ := advEnv(t, t.TempDir())
	checks := 0
	env.Alive = func(int) bool {
		checks++
		return checks > 1
	}
	st := runningCrew("old")
	writeUnfinished(t, getenv, "old")
	markLive(t, getenv, "old")
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if checks != 2 || len(hp.calls) != 0 || st.Tasks[0].Status != StatusRunning {
		t.Fatalf("checks %d calls %v task %+v", checks, hp.calls, st.Tasks[0])
	}
}

func TestAdvanceFailsARefusedRunAfterOneResume(t *testing.T) {
	state := t.TempDir()
	env, getenv, hp, _ := advEnv(t, state)
	st := &State{ID: "c1", Plan: "/p", Repo: "/repo", Base: "b", Concurrency: 1,
		Tasks: []Task{{Title: "C", Slug: "c", Prompt: "p", RunID: "old", Status: StatusRunning}}}
	writeUnfinished(t, getenv, "old")
	refusal := (&provider.HTTPError{Status: 400, Body: "model says no"}).Error()
	if err := os.WriteFile(runLogPath(getenv, "old"), []byte("error: "+refusal+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusRunning || len(hp.calls) != 1 {
		t.Fatalf("first pass: status %s, hp calls %d", st.Tasks[0].Status, len(hp.calls))
	}
	if err := advance(context.Background(), env, st); err != nil {
		t.Fatal(err)
	}
	if st.Tasks[0].Status != StatusFailed || st.Tasks[0].EndReason != refusal {
		t.Fatalf("task %+v", st.Tasks[0])
	}
	if len(hp.calls) != 1 {
		t.Fatalf("hp calls %d want 1", len(hp.calls))
	}
}

func TestRefused(t *testing.T) {
	cases := map[string]bool{
		(&provider.HTTPError{Status: 400, Body: "bad"}).Error():      true,
		(&provider.HTTPError{Status: 401, Body: "no key"}).Error():   true,
		(&provider.HTTPError{Status: 404, Body: "no model"}).Error(): true,
		(&provider.HTTPError{Status: 408, Body: "slow"}).Error():     false,
		(&provider.HTTPError{Status: 500, Body: "oops"}).Error():     false,
		(&provider.RateLimitError{Body: "slow down"}).Error():        false,
		"dial tcp: connection refused":                               false,
		"":                                                           false,
		"http":                                                       false,
	}
	for text, want := range cases {
		if got := refused(text); got != want {
			t.Errorf("refused(%q) = %v, want %v", text, got, want)
		}
	}
}
