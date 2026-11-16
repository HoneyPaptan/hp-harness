package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

const (
	keyUp   = "\x1b[A"
	keyDown = "\x1b[B"
)

func twoTaskSnapshot() crewview.Snapshot {
	return crewview.Snapshot{ID: "c1", Label: "add a cache", Rows: []crewview.Row{
		{Slug: "a", Title: "task a", Status: "running", RunID: "run-a", Changed: "1 file", Keys: "s"},
		{Slug: "b", Title: "task b", Status: "finished", RunID: "run-b", Changed: "2 files"},
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

func controlSnapshot() crewview.Snapshot {
	return crewview.Snapshot{ID: "c1", Label: "add a cache", Rows: []crewview.Row{
		{Slug: "a", Title: "task a", Status: "running", RunID: "run-a", Changed: "1 file", Keys: "s"},
		{Slug: "b", Title: "task b", Status: "failed: keeps crashing", RunID: "run-b", Changed: "2 files", Keys: "r"},
		{Slug: "c", Title: "task c", Status: "finished", RunID: "run-c", Changed: "3 files"},
	}}
}

func controlFake() *crewFake {
	a := newCrewFake()
	a.snaps["c1"] = controlSnapshot()
	a.newest = "c1"
	return a
}

func TestCrewViewHintFollowsTheSelectedTask(t *testing.T) {
	a := controlFake()
	h := startCrewSession(t, a)
	h.play([]step{
		{keys: "/crew\r"},
		{wait: crewViewHint + " · s stop · p pause · Esc closes"},
		{keys: keyDown},
		{wait: crewViewHint + " · r retry · p pause · Esc closes"},
		{keys: keyDown},
		{wait: crewViewHint + " · p pause · Esc closes"},
	})
}

func TestCrewViewStopAsksFirst(t *testing.T) {
	cases := []struct {
		name  string
		keys  string
		calls []string
	}{
		{"y stops", "sy", []string{"stop c1 a"}},
		{"capital Y stops", "sY", []string{"stop c1 a"}},
		{"n keeps it", "sn", nil},
		{"Esc keeps it", "s\x1b", nil},
		{"other keys do nothing", "sxq", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := controlFake()
			h := startCrewSession(t, a)
			h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: "s"}, {wait: "stop task a? y/n"}})
			h.play([]step{{keys: tc.keys[1:]}})
			time.Sleep(50 * time.Millisecond)
			got := a.acted()
			if len(got) != len(tc.calls) || len(got) == 1 && got[0] != tc.calls[0] {
				t.Fatalf("actions %v want %v", got, tc.calls)
			}
			if tc.keys == "sxq" {
				h.waitFor("stop task a? y/n")
				h.play([]step{{keys: "y"}})
				time.Sleep(50 * time.Millisecond)
				if got := a.acted(); len(got) != 1 || got[0] != "stop c1 a" {
					t.Fatalf("after y %v", got)
				}
			}
		})
	}
}

func TestCrewViewEscAtTheQuestionKeepsTheView(t *testing.T) {
	a := controlFake()
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: "s"}, {wait: "stop task a? y/n"}})
	before := strings.Count(h.out.String(), crewViewHint)
	h.play([]step{{keys: "\x1b"}})
	h.waitForCount(crewViewHint, before+1)
	h.play([]step{{keys: keyDown}, {wait: "r retry"}})
}

func TestCrewViewRetryNeedsNoConfirm(t *testing.T) {
	a := controlFake()
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: keyDown}, {keys: "r"}})
	deadline := time.Now().Add(2 * time.Second)
	for len(a.acted()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := a.acted(); len(got) != 1 || got[0] != "retry c1 b" {
		t.Fatalf("actions %v", got)
	}
	if strings.Contains(h.out.String(), "retry task b?") {
		t.Fatal("retry asked a question")
	}
}

func TestCrewViewKeysThatDoNotApplyExplainThemselves(t *testing.T) {
	cases := []struct {
		name string
		keys string
		want string
	}{
		{"stop a failed task", keyDown + "s", "nothing to stop: task b is failed: keeps crashing"},
		{"retry a running task", "r", "nothing to retry: task a is running"},
		{"stop a finished task", keyDown + keyDown + "s", "nothing to stop: task c is finished"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := controlFake()
			h := startCrewSession(t, a)
			h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: tc.keys}, {wait: tc.want}})
			if got := a.acted(); len(got) != 0 {
				t.Fatalf("actions %v", got)
			}
		})
	}
}

func TestCrewViewPauseAndResume(t *testing.T) {
	a := controlFake()
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: "p"}})
	deadline := time.Now().Add(2 * time.Second)
	for len(a.acted()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := a.acted(); len(got) != 1 || got[0] != "pause c1" {
		t.Fatalf("actions %v", got)
	}
	a.cmu.Lock()
	paused := controlSnapshot()
	paused.Paused = true
	a.snaps["c1"] = paused
	a.cmu.Unlock()
	h.waitFor("crew c1 · add a cache · paused")
	h.waitFor("p resume")
	h.waitFor("· crew paused")
	h.play([]step{{keys: "p"}})
	deadline = time.Now().Add(2 * time.Second)
	for len(a.acted()) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := a.acted(); len(got) != 2 || got[1] != "unpause c1" {
		t.Fatalf("actions %v", got)
	}
}

func TestCrewViewActionErrorIsShown(t *testing.T) {
	a := controlFake()
	a.actionErr = errors.New("task a is not running")
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: "sy"}, {wait: "task a is not running"}, {keys: "p"}, {wait: crewViewHint}})
}

func TestCrewViewControlKeysDoNothingWhileFollowing(t *testing.T) {
	a := controlFake()
	a.chunks[0] = []crewview.Chunk{{Text: "alpha\n", Offset: 6}}
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew\r"}, {wait: crewViewHint}, {keys: "\r"}, {wait: "alpha"}, {keys: "spr"}})
	time.Sleep(50 * time.Millisecond)
	if got := a.acted(); len(got) != 0 {
		t.Fatalf("actions while following: %v", got)
	}
}
