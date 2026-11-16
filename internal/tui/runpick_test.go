package tui

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
)

type runPickRig struct {
	*harness
	opened []string
	agent  *fakeAgent
}

func startRunPick(t *testing.T, resume bool, openErr error) *runPickRig {
	t.Helper()
	rig := &runPickRig{agent: newFakeAgent()}
	r, w := io.Pipe()
	rig.harness = &harness{t: t, out: &screen{}, writer: w, exited: make(chan struct{})}
	s := NewSession(Config{
		In: r, Out: rig.out, Width: func() int { return 80 },
		RunChoices: []RunChoice{
			{ID: "run-new", Label: "2m ago   fix the failing test"},
			{ID: "run-old", Label: "1h ago   rename the parser"},
		},
		OpenRun: func(id string) (Agent, bool, string, error) {
			rig.opened = append(rig.opened, id)
			return rig.agent, resume, "continuing " + id, openErr
		},
	})
	go func() {
		defer close(rig.exited)
		s.Run(context.Background())
	}()
	t.Cleanup(func() { w.Close() })
	return rig
}

func TestRunPickResumesTheNewestOnEnter(t *testing.T) {
	rig := startRunPick(t, false, nil)
	rig.play([]step{{wait: "fix the failing test"}, {keys: "\r"}, {wait: "continuing run-new"}, {keys: "go\r"}, {wait: "8 in"}, {keys: "\x04"}})
	rig.waitExit()
	if !reflect.DeepEqual(rig.opened, []string{"run-new"}) || !reflect.DeepEqual(rig.agent.prompts, []string{"go"}) {
		t.Fatalf("opened %v prompts %v", rig.opened, rig.agent.prompts)
	}
}

func TestRunPickMovesAndFilters(t *testing.T) {
	tests := []struct {
		name string
		keys string
		want string
	}{
		{"arrow down", "\x1b[B\r", "run-old"},
		{"filter by prompt text", "parser\r", "run-old"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rig := startRunPick(t, false, nil)
			rig.play([]step{{wait: "rename the parser"}, {keys: tc.keys}, {wait: "continuing " + tc.want}, {keys: "\x04"}})
			rig.waitExit()
			if !reflect.DeepEqual(rig.opened, []string{tc.want}) {
				t.Fatalf("opened %v", rig.opened)
			}
		})
	}
}

func TestRunPickEscCancelsWithoutOpening(t *testing.T) {
	rig := startRunPick(t, false, nil)
	rig.play([]step{{wait: "fix the failing test"}, {keys: "\x1b"}})
	rig.waitExit()
	if len(rig.opened) != 0 {
		t.Fatalf("opened %v", rig.opened)
	}
}

func TestRunPickResumesAnUnfinishedRun(t *testing.T) {
	rig := startRunPick(t, true, nil)
	rig.play([]step{{wait: "fix the failing test"}, {keys: "\r"}, {wait: "resumed"}, {keys: "\x04"}})
	rig.waitExit()
	if rig.agent.resumed != 1 {
		t.Fatalf("resumed %d", rig.agent.resumed)
	}
}

func TestRunPickShowsAnOpenError(t *testing.T) {
	rig := startRunPick(t, false, errors.New("journal is damaged"))
	rig.play([]step{{wait: "fix the failing test"}, {keys: "\r"}, {wait: "error: journal is damaged"}})
	rig.waitExit()
}
