package crew

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func getenvMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestStateDirs(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"xdg", map[string]string{"XDG_STATE_HOME": "/state", "HOME": "/home/u"}, "/state/hp/crews"},
		{"default", map[string]string{"HOME": "/home/u"}, "/home/u/.local/state/hp/crews"},
		{"no home", map[string]string{}, ".local/state/hp/crews"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := crewsDir(getenvMap(tc.env)); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	if got := runsDir(getenvMap(map[string]string{"XDG_STATE_HOME": "/state"})); got != filepath.Join("/state", "hp", "runs") {
		t.Fatalf("runsDir %q", got)
	}
}

func TestStateRoundTrip(t *testing.T) {
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": t.TempDir(), "HOME": "/home/u"})
	tasks := []planTask{
		{title: "One", prompt: "do one", slug: "one"},
		{title: "Two", prompt: "do two", slug: "two"},
	}
	st := newState(getenv, "c1", "/p/plan.md", "/repo", "abc123", runFlags{n: 3, model: "groq/gpt"}, tasks)
	st.Tasks[1].Status = StatusFinished
	st.Tasks[1].RunID = "r1"
	st.Tasks[1].EndReason = "final"
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	got, err := load(getenv, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "c1" || got.Repo != "/repo" || got.Base != "abc123" || got.Concurrency != 3 || got.Model != "groq/gpt" {
		t.Fatalf("state %+v", got)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("tasks %d", len(got.Tasks))
	}
	if got.Tasks[0].Status != StatusQueued || got.Tasks[0].Branch != "crew/c1/one" {
		t.Fatalf("task0 %+v", got.Tasks[0])
	}
	if got.Tasks[1].Status != StatusFinished || got.Tasks[1].RunID != "r1" {
		t.Fatalf("task1 %+v", got.Tasks[1])
	}
	wantWt := filepath.Join(crewsDir(getenv), "c1", "one")
	if got.Tasks[0].Worktree != wantWt {
		t.Fatalf("worktree %q want %q", got.Tasks[0].Worktree, wantWt)
	}
	if got.done() {
		t.Fatal("crew should not be done with one task queued")
	}
	got.Tasks[0].Status = StatusFinished
	if !got.done() {
		t.Fatal("crew should be done")
	}
}

func TestStateCounts(t *testing.T) {
	st := &State{Tasks: []Task{
		{Status: StatusQueued},
		{Status: StatusRunning},
		{Status: StatusFinished},
		{Status: StatusFailed},
	}}
	if st.done() {
		t.Fatal("crew not done")
	}
	if st.runningCount() != 1 || st.doneCount() != 1 {
		t.Fatalf("counts %d %d", st.runningCount(), st.doneCount())
	}
	st.Tasks[0].Status = StatusFinished
	st.Tasks[1].Status = StatusFinished
	if !st.done() {
		t.Fatal("crew done")
	}
}

func TestNewCrewID(t *testing.T) {
	id := newCrewID()
	parts := strings.Split(id, "-")
	if len(parts) != 3 || len(parts[2]) != 4 {
		t.Fatalf("id %q", id)
	}
	if _, err := time.Parse("20060102", parts[0]); err != nil {
		t.Fatalf("date part %q: %v", parts[0], err)
	}
}

func TestLoadDamaged(t *testing.T) {
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": t.TempDir()})
	st := newState(getenv, "x", "/p", "/r", "b", runFlags{n: 2}, nil)
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath(getenv, "x"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(getenv, "x"); err == nil {
		t.Fatal("expected error for damaged state")
	}
}
