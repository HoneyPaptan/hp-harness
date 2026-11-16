package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

type briefStep struct {
	text string
	done bool
}

type crewFake struct {
	*fakeAgent
	cmu       sync.Mutex
	files     map[string][]string
	planErr   error
	draft     CrewPlan
	draftErr  error
	draftWait bool
	goals     []string
	started   []CrewPlan
	counts    []int
	startErr  error
	list      []string
	status    []string
	report    []string
	briefs    []briefStep
	briefErr  error
	asked     []string
	dirty     int
	preflight error
	snaps     map[string]crewview.Snapshot
	newest    string
	chunks    map[int][]crewview.Chunk
	follows   [][2]int
}

func (a *crewFake) CrewPreflight() (int, error) { return a.dirty, a.preflight }

func (a *crewFake) CrewView(id string) (crewview.Snapshot, bool, error) {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	if id == "" {
		id = a.newest
	}
	snap, ok := a.snaps[id]
	return snap, ok, nil
}

func (a *crewFake) CrewFollow(_ string, index, offset int) (crewview.Chunk, error) {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	a.follows = append(a.follows, [2]int{index, offset})
	queue := a.chunks[index]
	if len(queue) == 0 {
		return crewview.Chunk{Offset: offset}, nil
	}
	a.chunks[index] = queue[1:]
	return queue[0], nil
}

func (a *crewFake) followed() [][2]int {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	return append([][2]int(nil), a.follows...)
}

func newCrewFake() *crewFake {
	return &crewFake{
		fakeAgent: newFakeAgent(), files: map[string][]string{},
		snaps: map[string]crewview.Snapshot{}, chunks: map[int][]crewview.Chunk{},
	}
}

func (a *crewFake) CrewPlan(arg string) (CrewPlan, bool, error) {
	if a.planErr != nil {
		return CrewPlan{}, false, a.planErr
	}
	titles, ok := a.files[arg]
	return CrewPlan{Path: arg, Titles: titles}, ok, nil
}

func (a *crewFake) CrewDraft(ctx context.Context, goal string) (CrewPlan, error) {
	a.cmu.Lock()
	a.goals = append(a.goals, goal)
	a.cmu.Unlock()
	if a.draftWait {
		<-ctx.Done()
		return CrewPlan{}, ctx.Err()
	}
	return a.draft, a.draftErr
}

func (a *crewFake) CrewStart(plan CrewPlan, n int) (string, error) {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	if a.startErr != nil {
		return "", a.startErr
	}
	a.started = append(a.started, plan)
	a.counts = append(a.counts, n)
	return "crew-1", nil
}

func (a *crewFake) CrewList() ([]string, error) { return a.list, nil }

func (a *crewFake) CrewStatus(id string) ([]string, error) {
	a.record("status " + id)
	return a.status, nil
}

func (a *crewFake) CrewReport(id string) ([]string, error) {
	a.record("report " + id)
	return a.report, nil
}

func (a *crewFake) record(call string) {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	a.asked = append(a.asked, call)
}

func (a *crewFake) CrewBrief(string) (string, bool, error) {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	if a.briefErr != nil {
		return "", false, a.briefErr
	}
	if len(a.briefs) == 0 {
		return "", false, nil
	}
	step := a.briefs[0]
	if len(a.briefs) > 1 {
		a.briefs = a.briefs[1:]
	}
	return step.text, step.done, nil
}

func (a *crewFake) startedCount() int {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	return len(a.started)
}

func startCrewSession(t *testing.T, a *crewFake) *harness {
	t.Helper()
	r, w := io.Pipe()
	h := &harness{t: t, agent: a.fakeAgent, out: &screen{}, writer: w, exited: make(chan struct{})}
	s := NewSession(Config{In: r, Out: h.out, Agent: a, Width: func() int { return 80 }, CrewPoll: time.Millisecond})
	go func() {
		defer close(h.exited)
		s.Run(context.Background())
	}()
	t.Cleanup(func() { w.Close() })
	return h
}

func TestCrewCountPrompt(t *testing.T) {
	cases := []struct {
		name   string
		titles []string
		keys   string
		want   int
		prompt string
	}{
		{"default is two", []string{"a", "b", "c"}, "\r", 2, "3 tasks · agents at once (1-3): "},
		{"typed number", []string{"a", "b", "c"}, "\x7f3\r", 3, "3 tasks · agents at once (1-3): "},
		{"one task defaults to one", []string{"only"}, "\r", 1, "1 task · agents at once (1-1): "},
		{"two tasks default to two", []string{"a", "b"}, "\r", 2, "2 tasks · agents at once (1-2): "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newCrewFake()
			a.files["plan.md"] = tc.titles
			h := startCrewSession(t, a)
			h.play([]step{{keys: "/crew plan.md\r"}, {wait: tc.prompt}, {keys: tc.keys}, {wait: "crew crew-1 started"}})
			if a.startedCount() != 1 || a.counts[0] != tc.want {
				t.Fatalf("counts %v want %d", a.counts, tc.want)
			}
			if a.started[0].Path != "plan.md" {
				t.Fatalf("plan %q", a.started[0].Path)
			}
		})
	}
}

func TestCrewCountRejectsBadNumbers(t *testing.T) {
	for _, keys := range []string{"\x7f0\r", "\x7f4\r", "\x7fabc\r", "\x7f-1\r", "\x7f\r"} {
		t.Run(keys, func(t *testing.T) {
			a := newCrewFake()
			a.files["plan.md"] = []string{"a", "b", "c"}
			h := startCrewSession(t, a)
			h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: keys}, {wait: "enter a number from 1 to 3"}, {keys: "\x7f\x7f\x7f\x7f2\r"}, {wait: "started, 2 at once"}})
			if a.startedCount() != 1 || a.counts[0] != 2 {
				t.Fatalf("counts %v", a.counts)
			}
		})
	}
}

func TestCrewEscCancelsCount(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: "\x1b"}, {wait: crewCancelNote}, {keys: "\x04"}})
	h.waitExit()
	if a.startedCount() != 0 {
		t.Fatalf("started %d", a.startedCount())
	}
}

func TestCrewDraftFromGoal(t *testing.T) {
	a := newCrewFake()
	a.draft = CrewPlan{Path: "/state/plans/p.md", Text: "## add cache\ndo it\n## add docs\nwrite", Titles: []string{"add cache", "add docs"}}
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew add a cache and docs\r"}, {wait: "## add cache"}, {wait: "2 tasks · agents at once (1-2): "}, {keys: "\r"}, {wait: "crew crew-1 started"}})
	if len(a.goals) != 1 || a.goals[0] != "add a cache and docs" {
		t.Fatalf("goals %v", a.goals)
	}
	if a.started[0].Path != "/state/plans/p.md" || a.counts[0] != 2 {
		t.Fatalf("start %v %v", a.started, a.counts)
	}
}

func TestCrewDraftCancel(t *testing.T) {
	a := newCrewFake()
	a.draftWait = true
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew make it fast\r"}, {wait: crewDraftNote}, {keys: "\x1b"}, {wait: crewCancelNote}, {keys: "\x04"}})
	h.waitExit()
	if a.startedCount() != 0 {
		t.Fatalf("started %d", a.startedCount())
	}
}

func TestCrewDraftError(t *testing.T) {
	a := newCrewFake()
	a.draftErr = errors.New("the model reply was not a plan")
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew make it fast\r"}, {wait: "the model reply was not a plan"}, {keys: "\x04"}})
	h.waitExit()
	if a.startedCount() != 0 || strings.Contains(h.out.String(), "agents at once") {
		t.Fatalf("screen %q", h.out.String())
	}
}

func TestCrewDraftNeedsModel(t *testing.T) {
	a := newCrewFake()
	a.model = ""
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew make it fast\r"}, {wait: "pick a model first"}, {keys: "\x1b"}, {wait: pickerNote}})
	if len(a.goals) != 0 {
		t.Fatalf("drafted without a model: %v", a.goals)
	}
}

func TestCrewStartError(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	a.startErr = errors.New("not a git repository")
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: "\r"}, {wait: "not a git repository"}, {keys: "\x04"}})
	h.waitExit()
	if strings.Contains(h.out.String(), "started") {
		t.Fatalf("screen %q", h.out.String())
	}
}

func TestCrewShowCommands(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		asked string
	}{
		{"list", "/crew list\r", "c1  plan.md  1/2 done", ""},
		{"status", "/crew status c1\r", "task a  running", "status c1"},
		{"report", "/crew report c1\r", "## task a", "report c1"},
		{"status without id", "/crew status\r", crewUsage, ""},
		{"report without id", "/crew report\r", crewUsage, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newCrewFake()
			a.list = []string{"c1  plan.md  1/2 done"}
			a.status = []string{"task a  running"}
			a.report = []string{"## task a", "", "done"}
			h := startCrewSession(t, a)
			h.play([]step{{keys: tc.input}, {wait: tc.want}, {keys: "\x04"}})
			h.waitExit()
			if tc.asked != "" && (len(a.asked) != 1 || a.asked[0] != tc.asked) {
				t.Fatalf("asked %v", a.asked)
			}
			if tc.asked == "" && len(a.asked) != 0 {
				t.Fatalf("asked %v", a.asked)
			}
		})
	}
}

func TestCrewLiveStatusInFooter(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	a.briefs = []briefStep{{"2 queued", false}, {"1 running · 1 queued", false}, {"2 finished", true}}
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: "\r"}, {wait: "crew 2 queued"}, {wait: "crew 1 running · 1 queued"}, {wait: "crew crew-1 done: 2 finished"}})
	h.waitFor("/crew report crew-1")
}

func TestCrewStatusErrorStopsWatching(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	a.briefErr = errors.New("state is damaged")
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: "\r"}, {wait: "state is damaged"}})
}

func TestCrewUnavailableAgent(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "/crew\r"}, {wait: crewUnavailable}, {keys: "\x04"}})
	h.waitExit()
}

func TestCrewErrorFromAgent(t *testing.T) {
	a := newCrewFake()
	a.planErr = errors.New("/crew is not available in the browser")
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew plan.md\r"}, {wait: "/crew is not available in the browser"}, {keys: "\x04"}})
	h.waitExit()
}

func TestCrewBusyTurn(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	a.script = scriptOf(blockTurn)
	h := startCrewSession(t, a)
	h.play([]step{{keys: "hello\r"}, {turn: true}, {wait: "partial"}, {keys: "\x1b"}, {wait: "interrupted"}, {keys: "/crew plan.md\r"}, {wait: crewBusyNote}})
	if a.startedCount() != 0 {
		t.Fatalf("started %d", a.startedCount())
	}
}
