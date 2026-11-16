package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

const (
	keyUp   = "\x1b[A"
	keyDown = "\x1b[B"
)

func twoTaskSnapshot() crewview.Snapshot {
	return crewview.Snapshot{ID: "c1", Label: "add a cache", Rows: []crewview.Row{
		{Title: "task a", Status: "running", RunID: "run-a", Changed: "1 file"},
		{Title: "task b", Status: "finished", RunID: "run-b", Changed: "2 files"},
	}}
}

func viewFake() *crewFake {
	a := newCrewFake()
	a.snaps["c1"] = twoTaskSnapshot()
	a.newest = "c1"
	a.chunks[0] = []crewview.Chunk{{Text: "alpha output\n", Offset: 13}, {Text: "alpha more\n", Offset: 24}}
	a.chunks[1] = []crewview.Chunk{{
		Text: "bravo output\n", Offset: 13, Ended: true,
		Trailer: []string{"finished · branch crew/c1/b", " b.txt | 1 +", "", "the final answer"},
	}}
	return a
}

func TestCrewViewSelectOpenBack(t *testing.T) {
	a := viewFake()
	h := startCrewSession(t, a)
	h.play([]step{
		{keys: "/crew\r"},
		{wait: "crew c1 · add a cache"},
		{wait: "task a                   running"},
		{wait: "task b                   finished       run-b"},
		{wait: crewViewHint},
		{keys: "\r"},
		{wait: "── task a · running · run run-a"},
		{wait: "alpha output"},
		{wait: "alpha more"},
		{wait: "following task a"},
	})
	before := strings.Count(h.out.String(), crewViewHint)
	h.play([]step{{keys: "\x1b"}})
	h.waitForCount(crewViewHint, before+1)
	h.play([]step{
		{keys: keyDown},
		{keys: "\r"},
		{wait: "── task b · finished · run run-b"},
		{wait: "bravo output"},
		{wait: "the final answer"},
		{wait: crewBackNote},
		{keys: "\x1b"},
	})
	h.waitForCount(crewViewHint, before+2)
	h.play([]step{{keys: "\x1b"}, {keys: "hi\r"}, {turn: true}})
	h.waitFor("ok")
	if got := a.sent(); len(got) != 1 || got[0] != "hi" {
		t.Fatalf("prompts after the view %v", got)
	}
	calls := a.followed()
	if calls[0] != [2]int{0, 0} {
		t.Fatalf("first follow %v", calls)
	}
	var sawOffset, sawSecond bool
	for _, c := range calls {
		sawOffset = sawOffset || c == [2]int{0, 13}
		sawSecond = sawSecond || c[0] == 1
	}
	if !sawOffset || !sawSecond {
		t.Fatalf("follows %v", calls)
	}
}

func TestCrewViewSelectionWraps(t *testing.T) {
	a := viewFake()
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: keyUp}, {keys: "\r"}, {wait: "── task b"}, {wait: "bravo output"}})
}

func TestCrewViewOpensOlderCrewByID(t *testing.T) {
	a := viewFake()
	old := twoTaskSnapshot()
	old.ID, old.Label = "c0", "older goal"
	a.snaps["c0"] = old
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew c0\r"}, {wait: "crew c0 · older goal"}, {keys: "\x1b"}, {keys: "\x04"}})
	h.waitExit()
	if strings.Contains(h.out.String(), "crew c1 ·") {
		t.Fatalf("opened the newest instead: %q", h.out.String())
	}
	if len(a.goals) != 0 {
		t.Fatalf("a crew id was drafted as a goal: %v", a.goals)
	}
}

func TestCrewViewNoCrews(t *testing.T) {
	a := newCrewFake()
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewNoCrews}, {keys: "\x04"}})
	h.waitExit()
}

func TestCrewViewCtrlCClosesBothLevels(t *testing.T) {
	a := viewFake()
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: "\r"}, {wait: "alpha more"}})
	before := strings.Count(h.out.String(), crewViewHint)
	h.play([]step{{keys: "\x03"}})
	h.waitForCount(crewViewHint, before+1)
	h.play([]step{{keys: "\x03"}, {keys: "hi\r"}, {turn: true}})
	if got := a.sent(); len(got) != 1 || got[0] != "hi" {
		t.Fatalf("prompts %v", got)
	}
}

func TestCrewViewRefreshesRows(t *testing.T) {
	a := viewFake()
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: "task a                   running"}})
	a.cmu.Lock()
	next := twoTaskSnapshot()
	next.Rows[0].Status = "finished"
	next.Done = true
	a.snaps["c1"] = next
	a.cmu.Unlock()
	h.waitFor("task a                   finished")
	h.play([]step{{keys: "\x1b"}, {keys: "\x04"}})
	h.waitExit()
}

func TestCrewViewWaitsForQueuedTask(t *testing.T) {
	a := viewFake()
	a.chunks[0] = []crewview.Chunk{{Waiting: true}, {Waiting: true}, {Text: "started\n", Offset: 8}}
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: "\r"}, {wait: crewWaitNote}, {wait: "started"}})
	if n := strings.Count(h.out.String(), crewWaitNote); n != 1 {
		t.Fatalf("wait note printed %d times", n)
	}
}

func TestCrewViewDoesNotOpenWhileTurnRuns(t *testing.T) {
	a := viewFake()
	a.script = scriptOf(blockTurn)
	h := startCrewSession(t, a)
	h.play([]step{{keys: "hello\r"}, {turn: true}, {wait: "partial"}, {keys: "\x1b"}, {wait: "interrupted"}, {keys: "/crew\r"}, {wait: crewBusyNote}})
	if strings.Contains(h.out.String(), crewViewHint) {
		t.Fatalf("view opened during a turn")
	}
}

func TestCrewPreflightBlocksDraftAndFile(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"goal", "/crew make it fast\r"},
		{"plan file", "/crew plan.md\r"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newCrewFake()
			a.files["plan.md"] = []string{"a", "b"}
			a.preflight = errors.New("/crew needs a git repository here (run git init first)")
			h := startCrewSession(t, a)
			h.play([]step{{keys: tc.input}, {wait: "/crew needs a git repository here (run git init first)"}, {keys: "hi\r"}, {turn: true}})
			if len(a.goals) != 0 || a.startedCount() != 0 {
				t.Fatalf("goals %v started %d", a.goals, a.startedCount())
			}
			out := h.out.String()
			if strings.Contains(out, "drafting") || strings.Contains(out, "agents at once") {
				t.Fatalf("flow began: %q", out)
			}
		})
	}
}

func TestCrewDirtyNote(t *testing.T) {
	cases := []struct {
		name  string
		dirty int
		want  string
	}{
		{"several", 3, "note: 3 uncommitted changes are not in the crew's worktrees"},
		{"one", 1, "note: 1 uncommitted change is not in the crew's worktrees"},
		{"none", 0, ""},
	}
	for _, tc := range cases {
		for _, flow := range []string{"file", "draft"} {
			t.Run(tc.name+" "+flow, func(t *testing.T) {
				a := newCrewFake()
				a.dirty = tc.dirty
				a.files["plan.md"] = []string{"a", "b"}
				a.draft = CrewPlan{Path: "/p/d.md", Text: "## a\nx\n## b\ny", Titles: []string{"a", "b"}}
				input := "/crew plan.md\r"
				if flow == "draft" {
					input = "/crew make it fast\r"
				}
				h := startCrewSession(t, a)
				h.play([]step{{keys: input}, {wait: "agents at once"}})
				out := h.out.String()
				if tc.want == "" {
					if strings.Contains(out, "uncommitted") {
						t.Fatalf("note shown for a clean tree: %q", out)
					}
				} else if !strings.Contains(out, tc.want) {
					t.Fatalf("missing %q in %q", tc.want, out)
				}
				h.play([]step{{keys: "\r"}, {wait: "crew crew-1 started"}})
				if a.startedCount() != 1 {
					t.Fatalf("the user could not go ahead: %d", a.startedCount())
				}
			})
		}
	}
}
