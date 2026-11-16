package crew

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

type spawnCall struct {
	args []string
	log  string
}

func serviceEnv(t *testing.T) (Env, func(string) string, *[]spawnCall) {
	t.Helper()
	cwd := t.TempDir()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": t.TempDir(), "HOME": "/h"})
	var calls []spawnCall
	env := Env{
		Getenv: getenv, Cwd: cwd,
		Alive: func(int) bool { return false },
		Git:   func(string, ...string) (string, error) { return "/repo\n", nil },
		Spawn: func(args []string, log string) (int, error) {
			calls = append(calls, spawnCall{args, log})
			return 4242, nil
		},
	}
	return env, getenv, &calls
}

func writePlan(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestServicePlanTitles(t *testing.T) {
	env, _, _ := serviceEnv(t)
	writePlan(t, env.Cwd, "plan.md", "# Plan\n## one\nfirst\n## two\nsecond\n")
	writePlan(t, env.Cwd, "empty.md", "just text\n")
	if err := os.Mkdir(filepath.Join(env.Cwd, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		arg       string
		titles    []string
		found     bool
		wantError bool
	}{
		{"relative file", "plan.md", []string{"one", "two"}, true, false},
		{"absolute file", filepath.Join(env.Cwd, "plan.md"), []string{"one", "two"}, true, false},
		{"missing file is a goal", "add a cache", nil, false, false},
		{"long goal is a goal", strings.Repeat("word ", 200), nil, false, false},
		{"directory is a goal", "dir", nil, false, false},
		{"file without tasks", "empty.md", nil, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			titles, found, err := Service{Env: env}.PlanTitles(tc.arg)
			if found != tc.found || (err != nil) != tc.wantError || strings.Join(titles, ",") != strings.Join(tc.titles, ",") {
				t.Fatalf("got %v %v %v", titles, found, err)
			}
		})
	}
}

func TestServiceStart(t *testing.T) {
	env, getenv, calls := serviceEnv(t)
	writePlan(t, env.Cwd, "plan.md", "## one\nfirst\n## two\nsecond\n## three\nthird\n")
	id, err := Service{Env: env}.Start(crewview.Start{Plan: "plan.md", Label: "  add   a cache ", N: 3, Model: "custom/m", BaseURL: "http://x/v1", KeyEnv: "K"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := load(getenv, id)
	if err != nil {
		t.Fatal(err)
	}
	if st.Label != "add a cache" || st.Concurrency != 3 || st.Model != "custom/m" || st.BaseURL != "http://x/v1" || st.KeyEnv != "K" || len(st.Tasks) != 3 || st.Repo != "/repo" {
		t.Fatalf("state %+v", st)
	}
	if len(*calls) != 1 || strings.Join((*calls)[0].args, " ") != "crew resume "+id || (*calls)[0].log != logPath(getenv, id) {
		t.Fatalf("spawn %+v", *calls)
	}
	data, err := os.ReadFile(pidPath(getenv, id))
	if err != nil || strings.TrimSpace(string(data)) != strconv.Itoa(4242) {
		t.Fatalf("pid file %q %v", data, err)
	}
}

func TestServiceStartRefusals(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	writePlan(t, env.Cwd, "plan.md", "## one\nfirst\n")
	writePlan(t, env.Cwd, "bad.md", "no tasks\n")
	cases := []struct {
		name string
		plan string
		n    int
		edit func(*Env)
		want string
	}{
		{"zero agents", "plan.md", 0, nil, "at least 1"},
		{"bad plan", "bad.md", 1, nil, "no ## tasks"},
		{"missing plan", "none.md", 1, nil, "no such file"},
		{"not a git repo", "plan.md", 1, func(e *Env) {
			e.Git = func(string, ...string) (string, error) { return "", errors.New("fatal") }
		}, "needs a git repository here"},
		{"spawn fails", "plan.md", 1, func(e *Env) {
			e.Spawn = func([]string, string) (int, error) { return 0, errors.New("boom") }
		}, "supervisor did not start"},
		{"no spawn", "plan.md", 1, func(e *Env) { e.Spawn = nil }, "detaching is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := env
			if tc.edit != nil {
				tc.edit(&e)
			}
			_, err := Service{Env: e}.Start(crewview.Start{Plan: tc.plan, N: tc.n})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v want %q", err, tc.want)
			}
		})
	}
	if _, err := os.Stat(getenv("XDG_STATE_HOME")); err != nil {
		t.Fatal(err)
	}
}

func TestServiceBrief(t *testing.T) {
	cases := []struct {
		name     string
		statuses []Status
		alive    bool
		want     string
		done     bool
	}{
		{"all queued", []Status{StatusQueued, StatusQueued}, true, "2 queued", false},
		{"mixed", []Status{StatusRunning, StatusStarting, StatusQueued, StatusFinished}, true, "1 queued · 2 running · 1 finished", false},
		{"dead supervisor", []Status{StatusRunning}, false, "1 running · supervisor not running", false},
		{"all done", []Status{StatusFinished, StatusFailed, StatusStopped}, false, "1 finished · 1 failed · 1 stopped", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, getenv, _ := serviceEnv(t)
			env.Alive = func(int) bool { return tc.alive }
			var tasks []planTask
			for range tc.statuses {
				tasks = append(tasks, planTask{title: "t", prompt: "p", slug: "t"})
			}
			st := newState(getenv, "c1", "/p/plan.md", "/repo", "base", runFlags{n: 2}, tasks)
			for i, s := range tc.statuses {
				st.Tasks[i].Status = s
			}
			if err := st.save(getenv); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pidPath(getenv, "c1"), []byte("77"), 0o600); err != nil {
				t.Fatal(err)
			}
			got, done, err := Service{Env: env}.Brief("c1")
			if err != nil || got != tc.want || done != tc.done {
				t.Fatalf("got %q %v %v", got, done, err)
			}
		})
	}
	env, _, _ := serviceEnv(t)
	if _, _, err := (Service{Env: env}).Brief("nope"); err == nil {
		t.Fatal("missing crew not reported")
	}
}

func TestServiceTextMatchesCommands(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	env.Git = func(_ string, args ...string) (string, error) { return "", nil }
	st := newState(getenv, "c1", "/p/plan.md", "/repo", "base", runFlags{n: 2}, []planTask{
		{title: "Done", prompt: "done", slug: "done"},
		{title: "Later", prompt: "later", slug: "later"},
	})
	st.Tasks[0].Status = StatusFinished
	st.Tasks[0].RunID = "r1"
	st.Tasks[1].Status = StatusFailed
	st.Tasks[1].EndReason = "boom"
	seedAnswer(t, getenv, "r1", "final", "all good")
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	svc := Service{Env: env}
	cases := []struct {
		name string
		args []string
		get  func() ([]string, error)
	}{
		{"status", []string{"crew", "status", "c1"}, func() ([]string, error) { return svc.Status("c1") }},
		{"report", []string{"crew", "report", "c1"}, func() ([]string, error) { return svc.Report("c1") }},
		{"list", []string{"crews"}, svc.List},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			e := env
			e.Stdout = &out
			if code := Main(context.Background(), e, tc.args); code != 0 {
				t.Fatalf("code %d", code)
			}
			lines, err := tc.get()
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(lines, "\n") + "\n"; strings.TrimRight(got, "\n") != strings.TrimRight(out.String(), "\n") {
				t.Fatalf("service %q cli %q", got, out.String())
			}
		})
	}
	if lines, _ := svc.List(); len(lines) != 1 || !strings.Contains(lines[0], "1/2 done") {
		t.Fatalf("list %v", lines)
	}
	if _, err := svc.Status("nope"); err == nil {
		t.Fatal("status of a missing crew succeeded")
	}
	if _, err := svc.Report("nope"); err == nil {
		t.Fatal("report of a missing crew succeeded")
	}
}

func TestServiceListEmpty(t *testing.T) {
	env, _, _ := serviceEnv(t)
	lines, err := Service{Env: env}.List()
	if err != nil || len(lines) != 1 || lines[0] != "no crews yet" {
		t.Fatalf("%v %v", lines, err)
	}
}

func TestSavePlan(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		text  string
		bad   bool
	}{
		{"plain", "## a\ndo a\n## b\ndo b\n", "## a\ndo a\n## b\ndo b", false},
		{"fenced", "```markdown\n## a\ndo a\n```\n", "## a\ndo a", false},
		{"preamble", "Here you go:\n## a\ndo a\n", "Here you go:\n## a\ndo a", false},
		{"no tasks", "I cannot do that.", "", true},
		{"empty", "  ", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, getenv, _ := serviceEnv(t)
			path, text, err := Service{Env: env}.SavePlan(tc.reply)
			if tc.bad {
				if err == nil {
					t.Fatalf("saved %q", text)
				}
				return
			}
			if err != nil || text != tc.text {
				t.Fatalf("got %q %v", text, err)
			}
			if filepath.Dir(path) != filepath.Join(crewsDir(getenv), "plans") {
				t.Fatalf("path %q", path)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.text+"\n" {
				t.Fatalf("file %q %v", data, err)
			}
			if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
				t.Fatalf("mode %v", info.Mode())
			}
			if strings.HasPrefix(path, env.Cwd) {
				t.Fatalf("plan written inside the working tree: %q", path)
			}
		})
	}
}

func TestDraftSystemNamesTheFormat(t *testing.T) {
	sys := Service{}.DraftSystem()
	if !strings.Contains(sys, "'## ' heading per task") {
		t.Fatalf("system %q", sys)
	}
}

func TestResumeRefusesSecondSupervisor(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	env.Alive = func(pid int) bool { return pid == 99 }
	env.Pid = 7
	var stderr bytes.Buffer
	env.Stderr = &stderr
	st := newState(getenv, "c1", "/p/plan.md", "/repo", "base", runFlags{n: 1}, []planTask{{title: "t", prompt: "p", slug: "t"}})
	st.Tasks[0].Status = StatusFinished
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidPath(getenv, "c1"), []byte("99"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := cmdResume(context.Background(), env, []string{"c1"}); code != 1 || !strings.Contains(stderr.String(), "already running") {
		t.Fatalf("code %d stderr %q", code, stderr.String())
	}
	if err := os.WriteFile(pidPath(getenv, "c1"), []byte("7"), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := cmdResume(context.Background(), env, []string{"c1"}); code != 0 {
		t.Fatalf("own pid refused: code %d stderr %q", code, stderr.String())
	}
	if _, err := os.Stat(pidPath(getenv, "c1")); !os.IsNotExist(err) {
		t.Fatalf("pid file left behind: %v", err)
	}
}
