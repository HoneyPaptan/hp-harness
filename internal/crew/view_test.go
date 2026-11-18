package crew

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflight(t *testing.T) {
	getenv := getenvMap(map[string]string{"HOME": "/h"})
	empty := t.TempDir()
	unborn := t.TempDir()
	runGit(t, unborn, "init", "-q")
	clean := initRepo(t)
	dirty := initRepo(t)
	for _, name := range []string{"x.txt", "y.txt"} {
		if err := os.WriteFile(filepath.Join(dirty, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dirty, "a.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		dir  string
		want int
		err  string
	}{
		{"not a repository", empty, 0, "/crew needs a git repository here (run git init first)"},
		{"no commit yet", unborn, 0, "/crew needs a commit in this repository"},
		{"clean", clean, 0, ""},
		{"three changes", dirty, 3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testEnvGit(getenv)
			env.Cwd = tc.dir
			n, err := Service{Env: env}.Preflight()
			if tc.err == "" && err != nil || tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
				t.Fatalf("err %v want %q", err, tc.err)
			}
			if n != tc.want {
				t.Fatalf("changes %d want %d", n, tc.want)
			}
		})
	}
}

func seedCrew(t *testing.T, getenv func(string) string, id, label, plan string, statuses ...Status) *State {
	t.Helper()
	var tasks []planTask
	for i := range statuses {
		tasks = append(tasks, planTask{title: "task " + string(rune('a'+i)), prompt: "p", slug: "t" + string(rune('a'+i))})
	}
	st := newState(getenv, id, plan, "/repo", "base", runFlags{n: 2}, tasks)
	st.Label = label
	for i, s := range statuses {
		st.Tasks[i].Status = s
	}
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestViewPicksNewestOrNamed(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	env.Git = func(string, ...string) (string, error) { return "", nil }
	svc := Service{Env: env}
	if _, found, err := svc.View(""); found || err != nil {
		t.Fatalf("empty state: %v %v", found, err)
	}
	seedCrew(t, getenv, "20260101-000000-aaaa", "old goal", "/p/old.md", StatusFinished)
	seedCrew(t, getenv, "20260102-000000-bbbb", "new goal", "/p/new.md", StatusRunning, StatusQueued)
	cases := []struct {
		name  string
		id    string
		want  string
		found bool
	}{
		{"newest", "", "20260102-000000-bbbb", true},
		{"older by id", "20260101-000000-aaaa", "20260101-000000-aaaa", true},
		{"unknown id", "20250101-000000-zzzz", "", false},
		{"a plan name", "plan.md", "", false},
		{"a path", "../x", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap, found, err := svc.View(tc.id)
			if err != nil || found != tc.found || snap.ID != tc.want {
				t.Fatalf("%+v %v %v", snap, found, err)
			}
		})
	}
	snap, _, _ := svc.View("")
	if snap.Label != "new goal" || snap.Done || len(snap.Rows) != 2 {
		t.Fatalf("snapshot %+v", snap)
	}
	if r := snap.Rows[0]; r.Title != "task a" || r.Status != "running" || r.RunID != "-" {
		t.Fatalf("row %+v", r)
	}
	if r := snap.Rows[1]; r.Status != "queued" || r.Changed != "-" {
		t.Fatalf("row %+v", r)
	}
	if err := os.WriteFile(statePath(getenv, "20260103-000000-cccc"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.View(""); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("damaged state: %v", err)
	}
}

func TestViewDoneAndChangedFiles(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	env.Git = func(_ string, args ...string) (string, error) {
		if args[0] == "status" {
			return "?? a\n M b\n", nil
		}
		return "", nil
	}
	st := seedCrew(t, getenv, "c1", "", "/p/plan.md", StatusFinished, StatusFailed)
	st.Tasks[1].EndReason = "boom"
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	snap, found, err := Service{Env: env}.View("c1")
	if err != nil || !found || !snap.Done || snap.Label != "plan.md" {
		t.Fatalf("%+v %v %v", snap, found, err)
	}
	if snap.Rows[0].Changed != "2 files" || snap.Rows[1].Status != "failed" || snap.Rows[1].Detail != "boom" {
		t.Fatalf("rows %+v", snap.Rows)
	}
}

func writeLog(t *testing.T, getenv func(string) string, id, text string) {
	t.Helper()
	if err := os.MkdirAll(runsDir(getenv), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runLogPath(getenv, id), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFollow(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	env.Git = func(_ string, args ...string) (string, error) {
		if args[0] == "diff" {
			return " a.txt | 1 +\n", nil
		}
		return "", nil
	}
	svc := Service{Env: env}
	st := seedCrew(t, getenv, "c1", "goal", "/p/plan.md", StatusQueued, StatusRunning, StatusFinished, StatusFailed, StatusStopped)
	st.Tasks[1].RunID = "run-live"
	st.Tasks[2].RunID = "run-done"
	st.Tasks[3].EndReason = "no key"
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	writeLog(t, getenv, "run-live", "step one\n")
	writeLog(t, getenv, "run-done", "did the work\nall good\n")
	seedAnswer(t, getenv, "run-done", "final", "all good")
	markLive(t, getenv, "run-live")
	env.Alive = func(int) bool { return true }
	svc = Service{Env: env}

	if c, err := svc.Follow("c1", 0, 0); err != nil || !c.Waiting || c.Ended || c.Text != "" {
		t.Fatalf("queued %+v %v", c, err)
	}
	c, err := svc.Follow("c1", 1, 0)
	if err != nil || c.Text != "step one\n" || c.Offset != 9 || c.Ended || c.Waiting {
		t.Fatalf("live %+v %v", c, err)
	}
	writeLog(t, getenv, "run-live", "step one\nstep two\n")
	if c, _ := svc.Follow("c1", 1, 9); c.Text != "step two\n" || c.Offset != 18 {
		t.Fatalf("live next %+v", c)
	}
	if c, _ := svc.Follow("c1", 1, 18); c.Text != "" || c.Offset != 18 {
		t.Fatalf("live idle %+v", c)
	}
	if c, _ := svc.Follow("c1", 1, 500); c.Text != "step one\nstep two\n" {
		t.Fatalf("offset past the end restarts: %+v", c)
	}

	c, err = svc.Follow("c1", 2, 0)
	if err != nil || !c.Ended || c.Text != "did the work\nall good\n" {
		t.Fatalf("done %+v %v", c, err)
	}
	if got := strings.Join(c.Trailer, "|"); got != "finished · branch crew/c1/tc|a.txt | 1 +" {
		t.Fatalf("done trailer %q", got)
	}
	writeLog(t, getenv, "run-done", "did the work\n")
	c, _ = svc.Follow("c1", 2, 0)
	if got := strings.Join(c.Trailer, "|"); got != "finished · branch crew/c1/tc|a.txt | 1 +||all good" {
		t.Fatalf("answer missing from the log is appended: %q", got)
	}

	if c, _ := svc.Follow("c1", 3, 0); !c.Ended || strings.Join(c.Trailer, "|") != "failed: no key" || c.Waiting {
		t.Fatalf("failed %+v", c)
	}
	if c, _ := svc.Follow("c1", 4, 0); !c.Ended || strings.Join(c.Trailer, "|") != "stopped" {
		t.Fatalf("stopped %+v", c)
	}
	if _, err := svc.Follow("c1", 9, 0); err == nil {
		t.Fatal("bad index accepted")
	}
	if _, err := svc.Follow("nope", 0, 0); err == nil {
		t.Fatal("missing crew accepted")
	}
}

func TestFollowEndsWhenJournalEndsBeforeState(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	env.Git = func(string, ...string) (string, error) { return "", nil }
	st := seedCrew(t, getenv, "c1", "goal", "/p/plan.md", StatusRunning)
	st.Tasks[0].RunID = "r1"
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	writeLog(t, getenv, "r1", "out\n")
	seedAnswer(t, getenv, "r1", "final", "done")
	c, err := Service{Env: env}.Follow("c1", 0, 0)
	if err != nil || !c.Ended || c.Text != "out\n" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestLabels(t *testing.T) {
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": "/s", "HOME": "/h"})
	long := strings.Repeat("word ", 30)
	cases := []struct {
		name  string
		label string
		plan  string
		want  string
	}{
		{"goal", "add  a cache\nand docs", "/s/hp/crews/plans/x.md", "add a cache and docs"},
		{"long goal is cut", long, "/s/hp/crews/plans/x.md", strings.TrimSpace(strings.Repeat("word ", 7)) + " wor…"},
		{"drafted without label uses the first task", "", "/s/hp/crews/plans/x.md", "task a"},
		{"plan file keeps its name", "", "/work/plan.md", "plan.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newState(getenv, "c1", tc.plan, "/repo", "base", runFlags{n: 1}, []planTask{{title: "task a", prompt: "p", slug: "a"}})
			st.Label = tc.label
			got := st.label(getenv)
			if tc.name == "long goal is cut" {
				if len([]rune(got)) != maxLabelRunes || !strings.HasSuffix(got, "…") {
					t.Fatalf("label %q", got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("label %q want %q", got, tc.want)
			}
		})
	}
}

func TestListShowsLabelNotGeneratedName(t *testing.T) {
	env, getenv, _ := serviceEnv(t)
	seedCrew(t, getenv, "c1", "add a cache", filepath.Join(plansDir(getenv), "20261116-135126-928a.md"), StatusFinished, StatusQueued)
	lines, err := Service{Env: env}.List()
	if err != nil || len(lines) != 1 || lines[0] != "c1  add a cache  1/2 done" {
		t.Fatalf("%v %v", lines, err)
	}
}

func TestReadLogFromReturnsOnlyNewBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		path     string
		offset   int
		wantText string
		wantEnd  int
	}{
		{"from the start", path, 0, "one\ntwo\n", 8},
		{"from the middle", path, 4, "two\n", 8},
		{"nothing new", path, 8, "", 8},
		{"log shrank", path, 99, "one\ntwo\n", 8},
		{"no log yet", path + ".missing", 3, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, end := readLogFrom(tc.path, tc.offset)
			if text != tc.wantText || end != tc.wantEnd {
				t.Fatalf("got %q end %d, want %q end %d", text, end, tc.wantText, tc.wantEnd)
			}
		})
	}
}

func TestCrashReasonQuotesTheLastErrorOfTheRun(t *testing.T) {
	home := t.TempDir()
	getenv := getenvMap(map[string]string{"HOME": home})
	env := Env{Getenv: getenv}
	if err := os.MkdirAll(runsDir(getenv), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := crashReason(env, "r1").Error(); got != "keeps crashing after 3 resumes" {
		t.Fatalf("no log: %q", got)
	}
	log := "working\nerror: http 400: first\nresume with: hp resume r1\nerror: http 400: model says no\nresume with: hp resume r1\n"
	if err := os.WriteFile(runLogPath(getenv, "r1"), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := crashReason(env, "r1").Error(); got != "http 400: model says no (gave up after 3 resumes)" {
		t.Fatalf("with log: %q", got)
	}
}
