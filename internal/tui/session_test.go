package tui

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const waitLimit = 5 * time.Second

type screen struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (c *screen) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *screen) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

type fakeAgent struct {
	mu      sync.Mutex
	prompts []string
	model   string
	resets  int
	resumed int
	runs    []string
	script  []func(context.Context, func(Event)) (Outcome, error)
	started chan struct{}
}

func newFakeAgent(script ...func(context.Context, func(Event)) (Outcome, error)) *fakeAgent {
	return &fakeAgent{model: "xai/grok", script: script, started: make(chan struct{}, 8)}
}

func (a *fakeAgent) Send(ctx context.Context, prompt string, emit func(Event)) (Outcome, error) {
	a.mu.Lock()
	a.prompts = append(a.prompts, prompt)
	var turn func(context.Context, func(Event)) (Outcome, error)
	if len(a.script) > 0 {
		turn, a.script = a.script[0], a.script[1:]
	}
	a.mu.Unlock()
	a.started <- struct{}{}
	if turn == nil {
		return replyTurn("ok")(ctx, emit)
	}
	return turn(ctx, emit)
}

func (a *fakeAgent) Resume(ctx context.Context, emit func(Event)) (Outcome, error) {
	a.mu.Lock()
	a.resumed++
	a.mu.Unlock()
	return replyTurn("resumed")(ctx, emit)
}

func (a *fakeAgent) Model() string { return a.model }

func (a *fakeAgent) SetModel(spec string) error {
	if !strings.Contains(spec, "/") {
		return errors.New("bad model")
	}
	a.model = spec
	return nil
}

func (a *fakeAgent) Reset() error {
	a.resets++
	return nil
}

func (a *fakeAgent) Runs() ([]string, error) { return a.runs, nil }

func (a *fakeAgent) sent() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.prompts...)
}

func replyTurn(text string) func(context.Context, func(Event)) (Outcome, error) {
	return func(_ context.Context, emit func(Event)) (Outcome, error) {
		emit(Event{Kind: EventText, Text: text})
		return Outcome{Model: "xai/grok", Steps: 1, InputTokens: 8, OutputTokens: 4, TTFT: 7 * time.Millisecond}, nil
	}
}

func blockTurn(_ context.Context, emit func(Event)) (Outcome, error) {
	emit(Event{Kind: EventText, Text: "partial"})
	select {}
}

func cancelTurn(ctx context.Context, emit func(Event)) (Outcome, error) {
	emit(Event{Kind: EventText, Text: "partial"})
	<-ctx.Done()
	return Outcome{}, ctx.Err()
}

type step struct {
	keys string
	wait string
	turn bool
}

type harness struct {
	t      *testing.T
	agent  *fakeAgent
	out    *screen
	writer *io.PipeWriter
	exited chan struct{}
}

func startSession(t *testing.T, a *fakeAgent, color, resume bool) *harness {
	t.Helper()
	r, w := io.Pipe()
	h := &harness{t: t, agent: a, out: &screen{}, writer: w, exited: make(chan struct{})}
	s := NewSession(Config{
		In: r, Out: h.out, Width: func() int { return 80 }, Color: color, Agent: a, Resume: resume,
	})
	go func() {
		defer close(h.exited)
		s.Run(context.Background())
	}()
	t.Cleanup(func() { w.Close() })
	return h
}

func (h *harness) play(steps []step) {
	h.t.Helper()
	for _, st := range steps {
		if st.wait != "" {
			h.waitFor(st.wait)
		}
		if st.turn {
			h.waitTurn()
		}
		if st.keys != "" {
			if _, err := io.WriteString(h.writer, st.keys); err != nil {
				h.t.Fatalf("session gone before %q: %q", st.keys, h.out.String())
			}
		}
	}
}

func (h *harness) waitTurn() {
	h.t.Helper()
	select {
	case <-h.agent.started:
	case <-time.After(waitLimit):
		h.t.Fatalf("no turn started: %q", h.out.String())
	}
}

func (h *harness) waitFor(text string) {
	h.t.Helper()
	deadline := time.Now().Add(waitLimit)
	for !strings.Contains(h.out.String(), text) {
		if time.Now().After(deadline) {
			h.t.Fatalf("never saw %q in %q", text, h.out.String())
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *harness) waitForCount(text string, n int) {
	h.t.Helper()
	deadline := time.Now().Add(waitLimit)
	for strings.Count(h.out.String(), text) < n {
		if time.Now().After(deadline) {
			h.t.Fatalf("never saw %d of %q in %q", n, text, h.out.String())
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *harness) waitExit() {
	h.t.Helper()
	select {
	case <-h.exited:
	case <-time.After(waitLimit):
		h.t.Fatalf("session did not exit: %q", h.out.String())
	}
}

func TestSessionScripts(t *testing.T) {
	status := "xai/grok  steps 1  tokens 8/4  ttft 7ms"
	cases := []struct {
		name    string
		script  []func(context.Context, func(Event)) (Outcome, error)
		steps   []step
		prompts []string
		want    []string
		reject  []string
	}{
		{
			name:    "submit keeps the prompt in scrollback",
			steps:   []step{{keys: "hello\r"}, {wait: status + "\x1b[0m"}, {keys: "\x04"}},
			prompts: []string{"hello"},
			want:    []string{"› hello", "ok\r\n", status},
		},
		{
			name: "follow up sends a second turn",
			script: scriptOf(replyTurn("ok"), func(context.Context, func(Event)) (Outcome, error) {
				return Outcome{Model: "second"}, nil
			}),
			steps:   []step{{keys: "one\r"}, {turn: true}, {wait: status}, {keys: "two\r"}, {turn: true}, {wait: "second  steps"}, {keys: "\x04"}},
			prompts: []string{"one", "two"},
			want:    []string{"› one", "› two"},
		},
		{
			name:    "ctrl c interrupts and returns to the prompt",
			script:  scriptOf(cancelTurn),
			steps:   []step{{keys: "go\r"}, {wait: "partial"}, {keys: "\x03"}, {wait: "interrupted"}, {wait: "\r\n\r\n"}, {keys: "\x04"}},
			prompts: []string{"go"},
			want:    []string{"partial\r\n", "interrupted"},
			reject:  []string{"steps"},
		},
		{
			name:    "lone esc interrupts",
			script:  scriptOf(cancelTurn),
			steps:   []step{{keys: "go\r"}, {wait: "partial"}, {keys: "\x1b"}, {wait: "interrupted"}, {wait: "\r\n\r\n"}, {keys: "\x04"}},
			prompts: []string{"go"},
			want:    []string{"interrupted"},
		},
		{
			name:    "typed ahead text becomes the draft",
			script:  scriptOf(cancelTurn),
			steps:   []step{{keys: "go\r"}, {wait: "partial"}, {keys: "next"}, {keys: "\r"}, {keys: "\x03"}, {wait: "› next"}, {keys: "\r"}, {turn: true}, {turn: true}, {wait: status}, {keys: "\x04"}},
			prompts: []string{"go", "next"},
		},
		{
			name:  "double ctrl c quits",
			steps: []step{{keys: "\x03"}, {wait: quitHint}, {keys: "\x03"}},
			want:  []string{quitHint},
		},
		{
			name:  "ctrl c with text clears it",
			steps: []step{{keys: "draft"}, {wait: "draft"}, {keys: "\x03"}, {keys: "\x04"}},
		},
		{
			name:  "ctrl d on an empty prompt quits",
			steps: []step{{keys: "\x04"}},
		},
		{
			name:  "help is local",
			steps: []step{{keys: "/help\r"}, {wait: "/quit"}, {keys: "/quit\r"}},
			want:  []string{"/model", "/new", "/runs", "//text"},
		},
		{
			name:  "model shows the current model",
			steps: []step{{keys: "/model\r"}, {wait: "model xai/grok"}, {keys: "/quit\r"}},
		},
		{
			name:  "model switches",
			steps: []step{{keys: "/model groq/fast\r"}, {wait: "model groq/fast"}, {keys: "/model\r"}, {keys: "/quit\r"}},
		},
		{
			name:  "model error stays in session",
			steps: []step{{keys: "/model nope\r"}, {wait: "error: bad model"}, {keys: "/quit\r"}},
		},
		{
			name:  "new resets the run",
			steps: []step{{keys: "/new\r"}, {wait: "new run"}, {keys: "/quit\r"}},
		},
		{
			name:  "runs lists runs",
			steps: []step{{keys: "/runs\r"}, {wait: "run-1  final"}, {keys: "/quit\r"}},
		},
		{
			name:  "unknown slash hints",
			steps: []step{{keys: "/nope\r"}, {wait: "unknown command /nope"}, {keys: "/quit\r"}},
		},
		{
			name:    "double slash sends literal",
			steps:   []step{{keys: "//text more\r"}, {turn: true}, {wait: status}, {keys: "/quit\r"}},
			prompts: []string{"/text more"},
		},
		{
			name:    "turn error stays in session",
			script:  scriptOf(func(context.Context, func(Event)) (Outcome, error) { return Outcome{}, errors.New("boom\nmore") }),
			steps:   []step{{keys: "go\r"}, {wait: "error: boom"}, {keys: "\x04"}},
			prompts: []string{"go"},
			reject:  []string{"more"},
		},
		{
			name: "tools and notes print one line each",
			script: scriptOf(func(_ context.Context, emit func(Event)) (Outcome, error) {
				emit(Event{Kind: EventStep})
				emit(Event{Kind: EventTool, Text: "read a.go"})
				emit(Event{Kind: EventToolError, Text: "no such file"})
				emit(Event{Kind: EventNote, Text: "limited on xai/grok"})
				emit(Event{Kind: EventText, Text: "bad\x1b[2Jout\nnext"})
				return Outcome{Model: "xai/grok", Jev: "jev requests 1 tokens 2/3", Note: "step limit"}, nil
			}),
			steps:   []step{{keys: "go\r"}, {wait: "step limit"}, {keys: "\x04"}},
			prompts: []string{"go"},
			want:    []string{"  read a.go", "  no such file", "limited on xai/grok", "bad[2Jout\r\nnext", "jev requests 1 tokens 2/3  step limit"},
			reject:  []string{"\x1b[2J"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAgent(tc.script...)
			a.runs = []string{"run-1  final"}
			h := startSession(t, a, true, false)
			h.play(tc.steps)
			h.waitExit()
			if got := a.sent(); !reflect.DeepEqual(got, tc.prompts) {
				t.Fatalf("prompts %q want %q", got, tc.prompts)
			}
			out := h.out.String()
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in %q", want, out)
				}
			}
			for _, reject := range tc.reject {
				if strings.Contains(out, reject) {
					t.Fatalf("found %q in %q", reject, out)
				}
			}
		})
	}
}

func scriptOf(turns ...func(context.Context, func(Event)) (Outcome, error)) []func(context.Context, func(Event)) (Outcome, error) {
	return turns
}

func TestSessionActivityErasedByFirstText(t *testing.T) {
	gate := make(chan struct{})
	a := newFakeAgent(func(_ context.Context, emit func(Event)) (Outcome, error) {
		<-gate
		emit(Event{Kind: EventText, Text: "hi"})
		return Outcome{Model: "m"}, nil
	})
	h := startSession(t, a, true, false)
	h.play([]step{{keys: "go\r"}, {wait: activityText}})
	close(gate)
	h.play([]step{{wait: "m  steps"}, {keys: "\x04"}})
	h.waitExit()
	if !strings.Contains(h.out.String(), activityText+sgrReset+eraseLine+"hi") {
		t.Fatalf("output %q", h.out.String())
	}
}

func TestSessionKeyDisarmsQuit(t *testing.T) {
	h := startSession(t, newFakeAgent(), false, false)
	h.play([]step{{keys: "\x03"}, {wait: quitHint}, {keys: "a"}, {wait: "› a"}, {keys: "\x7f\x03"}})
	h.waitForCount(quitHint, 2)
	h.play([]step{{keys: "\x04"}})
	h.waitExit()
}

func TestSessionNoColor(t *testing.T) {
	a := newFakeAgent(func(_ context.Context, emit func(Event)) (Outcome, error) {
		emit(Event{Kind: EventToolError, Text: "bad"})
		return replyTurn("ok")(context.Background(), emit)
	})
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "go\r"}, {wait: "ttft"}, {keys: "/help\r"}, {wait: "/quit"}, {keys: "/quit\r"}})
	h.waitExit()
	for _, sgr := range []string{sgrDim, sgrRed, sgrReset} {
		if strings.Contains(h.out.String(), sgr) {
			t.Fatalf("colour %q in %q", sgr, h.out.String())
		}
	}
}

func TestSessionOutputUsesCRLF(t *testing.T) {
	a := newFakeAgent(replyTurn("a\nb\n"))
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "go\r"}, {wait: "ttft"}, {keys: "\x04"}})
	h.waitExit()
	if out := strings.ReplaceAll(h.out.String(), "\r\n", ""); strings.Contains(out, "\n") {
		t.Fatalf("bare newline in %q", h.out.String())
	}
}

func TestSessionResumesFirst(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, true)
	h.play([]step{{wait: "resumed"}, {wait: "› "}, {keys: "\x04"}})
	h.waitExit()
	if a.resumed != 1 || len(a.sent()) != 0 {
		t.Fatalf("resumed %d prompts %q", a.resumed, a.sent())
	}
}

func TestSessionHungTurnStillQuits(t *testing.T) {
	a := newFakeAgent(blockTurn)
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "go\r"}, {wait: "partial"}, {keys: "\x03"}, {wait: "interrupted"}, {keys: "\x03"}})
	h.waitExit()
}

func TestSessionContextCancelQuitsHungTurn(t *testing.T) {
	a := newFakeAgent(blockTurn)
	r, w := io.Pipe()
	defer w.Close()
	out := &screen{}
	s := NewSession(Config{In: r, Out: out, Width: func() int { return 80 }, Agent: a})
	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		s.Run(ctx)
	}()
	if _, err := io.WriteString(w, "go\r"); err != nil {
		t.Fatal(err)
	}
	<-a.started
	cancel()
	select {
	case <-exited:
	case <-time.After(waitLimit):
		t.Fatalf("no exit: %q", out.String())
	}
}

func TestSessionInputEOFQuits(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{{wait: "› "}})
	h.writer.Close()
	h.waitExit()
}

func TestSessionPanicInTurnReachesCaller(t *testing.T) {
	a := newFakeAgent(func(context.Context, func(Event)) (Outcome, error) { panic("kaboom") })
	r, w := io.Pipe()
	defer w.Close()
	s := NewSession(Config{In: r, Out: &screen{}, Width: func() int { return 80 }, Agent: a})
	got := make(chan any, 1)
	go func() {
		defer func() { got <- recover() }()
		s.Run(context.Background())
	}()
	if _, err := io.WriteString(w, "go\r"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-got:
		if p != "kaboom" {
			t.Fatalf("panic %v", p)
		}
	case <-time.After(waitLimit):
		t.Fatal("panic did not reach Run")
	}
}
