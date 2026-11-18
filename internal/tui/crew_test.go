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
	edits     []CrewPlan
	editSteps []editStep
	actions   []string
	actionErr error
	lastCheck string
	diffs     map[string][]string
	landAsk   string
	landErr   error
	checkErr  error
}

type editStep struct {
	titles []string
	err    error
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
	return CrewPlan{Path: arg, Titles: titles, Verify: a.lastCheck}, ok, nil
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

func (a *crewFake) CrewEdit(plan CrewPlan) (CrewPlan, error) {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	a.edits = append(a.edits, plan)
	step := a.editSteps[0]
	if len(a.editSteps) > 1 {
		a.editSteps = a.editSteps[1:]
	}
	plan.Titles = step.titles
	return plan, step.err
}

func (a *crewFake) act(call string) error {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	a.actions = append(a.actions, call)
	return a.actionErr
}

func (a *crewFake) acted() []string {
	a.cmu.Lock()
	defer a.cmu.Unlock()
	return append([]string(nil), a.actions...)
}

func (a *crewFake) CrewDiff(id, task string) ([]string, error) {
	if err := a.act("diff " + id + " " + task); err != nil {
		return nil, err
	}
	return append([]string(nil), a.diffs[task]...), nil
}

func (a *crewFake) CrewLandCheck(id, task string) (string, error) {
	return a.landAsk, a.checkErr
}

func (a *crewFake) CrewLand(id, task string) (string, error) {
	a.cmu.Lock()
	a.actions = append(a.actions, "land "+id+" "+task)
	a.cmu.Unlock()
	if a.landErr != nil {
		return "", a.landErr
	}
	return "landed crew/" + id + "/" + task + " into main", nil
}

func (a *crewFake) CrewDiscard(id, task string) error { return a.act("discard " + id + " " + task) }

func (a *crewFake) CrewStop(id, task string) error  { return a.act("stop " + id + " " + task) }
func (a *crewFake) CrewRetry(id, task string) error { return a.act("retry " + id + " " + task) }
func (a *crewFake) CrewPause(id string) error       { return a.act("pause " + id) }
func (a *crewFake) CrewUnpause(id string) error     { return a.act("unpause " + id) }

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
	h.play([]step{{keys: "hello\r"}, {turn: true}, {wait: "partial"}, {keys: "\x1b"}, {wait: escStopHint}, {keys: "\x1b"}, {wait: "interrupted"}, {keys: "/crew plan.md\r"}, {wait: crewBusyNote}})
	if a.startedCount() != 0 {
		t.Fatalf("started %d", a.startedCount())
	}
}

func TestCrewEditUpdatesTasksAndPrompt(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	a.editSteps = []editStep{{titles: []string{"a", "b", "c", "d"}}}
	h := startCrewSession(t, a)
	h.play([]step{
		{keys: "/crew plan.md\r"},
		{wait: "2 tasks · agents at once (1-2): "},
		{wait: crewAskNote},
		{keys: "e"},
		{wait: "4. d"},
		{wait: "4 tasks · agents at once (1-4): "},
		{keys: "\r"},
		{wait: "crew crew-1 started, 2 at once"},
	})
	if len(a.edits) != 1 || a.edits[0].Path != "plan.md" {
		t.Fatalf("edits %v", a.edits)
	}
	if a.startedCount() != 1 || a.counts[0] != 2 {
		t.Fatalf("started %v", a.counts)
	}
}

func TestCrewEditKeepsTypedCountWhenItStillFits(t *testing.T) {
	cases := []struct {
		name   string
		titles []string
		want   int
	}{
		{"fits", []string{"a", "b", "c"}, 3},
		{"too large falls back to default", []string{"a"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newCrewFake()
			a.files["plan.md"] = []string{"a", "b", "c"}
			a.editSteps = []editStep{{titles: tc.titles}}
			h := startCrewSession(t, a)
			h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: "\x7f3e"}, {wait: "agents at once (1-" + string(rune('0'+len(tc.titles))) + ")"}, {keys: "\r"}, {wait: "crew crew-1 started"}})
			if a.counts[0] != tc.want {
				t.Fatalf("count %d want %d", a.counts[0], tc.want)
			}
		})
	}
}

func TestCrewEditInvalidPlanStaysOnConfirm(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	a.editSteps = []editStep{{err: errors.New("plan has no ## tasks")}, {titles: []string{"only"}}}
	h := startCrewSession(t, a)
	h.play([]step{
		{keys: "/crew plan.md\r"},
		{wait: "agents at once"},
		{keys: "e"},
		{wait: "plan has no ## tasks"},
		{wait: crewBadPlanNote},
		{wait: "plan has no tasks · e edits · Esc cancels"},
		{keys: "\r"},
	})
	if a.startedCount() != 0 {
		t.Fatalf("started with an invalid plan: %d", a.startedCount())
	}
	h.play([]step{{keys: "e"}, {wait: "1 task · agents at once (1-1): "}, {keys: "\r"}, {wait: "crew crew-1 started, 1 at once"}})
	if len(a.edits) != 2 {
		t.Fatalf("edits %d", len(a.edits))
	}
}

func TestCrewEditorFailureKeepsPlanUsable(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	a.editSteps = []editStep{{titles: []string{"a", "b"}, err: errors.New("vi did not finish: exit status 1")}}
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: "e"}, {wait: "vi did not finish"}, {wait: "2 tasks · agents at once (1-2): "}, {keys: "\r"}, {wait: "crew crew-1 started"}})
}

func TestCrewEditOnlyAtTheCountStep(t *testing.T) {
	a := newCrewFake()
	a.draftWait = true
	a.editSteps = []editStep{{titles: []string{"a"}}}
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew make it fast\r"}, {wait: crewDraftNote}, {keys: "e"}, {keys: "\x1b"}, {wait: crewCancelNote}})
	if len(a.edits) != 0 {
		t.Fatalf("edited while drafting: %v", a.edits)
	}
}

func TestCrewVerifyField(t *testing.T) {
	cases := []struct {
		name   string
		last   string
		keys   string
		verify string
	}{
		{"skipped by default", "", "\r", ""},
		{"typed after Tab", "", "\tgo test ./...\r", "go test ./..."},
		{"prefilled from the last crew", "make test", "\r", "make test"},
		{"prefilled value can be changed", "make test", "\t\x15go vet ./...\r", "go vet ./..."},
		{"prefilled value can be cleared", "make test", "\t\x15\r", ""},
		{"surrounding spaces are dropped", "", "\t  go test  \r", "go test"},
		{"Tab twice goes back to the count", "", "\t\t\x7f3\r", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newCrewFake()
			a.files["plan.md"] = []string{"a", "b", "c"}
			a.lastCheck = tc.last
			h := startCrewSession(t, a)
			h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: tc.keys}, {wait: "crew crew-1 started"}})
			if a.started[0].Verify != tc.verify {
				t.Fatalf("verify %q want %q", a.started[0].Verify, tc.verify)
			}
		})
	}
}

func TestCrewVerifyFieldPromptsAndKeepsTheCount(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b", "c"}
	a.lastCheck = "make test"
	h := startCrewSession(t, a)
	h.play([]step{
		{keys: "/crew plan.md\r"},
		{wait: "verify: make test"},
		{wait: crewAskNote},
		{keys: "\x7f3\t"},
		{wait: "verify command, empty skips (Tab goes back): make test"},
		{keys: "\t"},
		{wait: "3 tasks · agents at once (1-3): 3"},
		{keys: "\r"},
		{wait: "crew crew-1 started, 3 at once"},
	})
	if a.counts[0] != 3 || a.started[0].Verify != "make test" {
		t.Fatalf("started %v %v", a.counts, a.started)
	}
}

func TestCrewEditIsOnlyAtTheCountField(t *testing.T) {
	a := newCrewFake()
	a.files["plan.md"] = []string{"a", "b"}
	a.editSteps = []editStep{{titles: []string{"a"}}}
	h := startCrewSession(t, a)
	h.play([]step{{keys: "/crew plan.md\r"}, {wait: "agents at once"}, {keys: "\tdev\r"}, {wait: "crew crew-1 started"}})
	if len(a.edits) != 0 || a.started[0].Verify != "dev" {
		t.Fatalf("edits %v started %v", a.edits, a.started)
	}
}
