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
	mu         sync.Mutex
	prompts    []string
	keys       []string
	model      string
	providers  []string
	modelErr   error
	models     []string
	modelsErr  error
	modelsWait chan struct{}
	saveErr    error
	jevOn      bool
	jevErr     error
	resets     int
	resumed    int
	runs       []string
	files      []string
	filesErr   error
	filesCalls int
	script     []func(context.Context, func(Event)) (Outcome, error)
	started    chan struct{}
}

func newFakeAgent(script ...func(context.Context, func(Event)) (Outcome, error)) *fakeAgent {
	return &fakeAgent{
		model:     "xai/grok",
		providers: []string{"groq", "openrouter", "ollama"},
		script:    script,
		started:   make(chan struct{}, 8),
	}
}

func (a *fakeAgent) Send(ctx context.Context, prompt string, emit func(Event)) (Outcome, error) {
	if a.model == "" {
		return Outcome{}, NoModelError{}
	}
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

func (a *fakeAgent) Models(ctx context.Context, preset string) ([]string, error) {
	if a.modelsWait != nil {
		select {
		case <-a.modelsWait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if a.modelsErr != nil {
		return nil, a.modelsErr
	}
	return a.models, nil
}

func (a *fakeAgent) Providers() []string { return a.providers }

func (a *fakeAgent) Jev() bool { return a.jevOn }

func (a *fakeAgent) SetModel(spec string) error {
	if a.modelErr != nil {
		return a.modelErr
	}
	if !strings.Contains(spec, "/") {
		return errors.New("bad model")
	}
	a.model = spec
	return nil
}

func (a *fakeAgent) SetJev(on bool) error {
	if a.jevErr != nil {
		return a.jevErr
	}
	a.jevOn = on
	return nil
}

func (a *fakeAgent) SaveKey(name, value string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.saveErr != nil {
		return a.saveErr
	}
	a.keys = append(a.keys, name+"="+value)
	return nil
}

func (a *fakeAgent) Reset() error {
	a.resets++
	return nil
}

func (a *fakeAgent) Runs() ([]string, error) { return a.runs, nil }

func (a *fakeAgent) Files() ([]string, error) {
	a.mu.Lock()
	a.filesCalls++
	a.mu.Unlock()
	if a.filesErr != nil {
		return nil, a.filesErr
	}
	return append([]string(nil), a.files...), nil
}

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
	return startSessionWith(t, a, Config{Color: color, Resume: resume})
}

func startSessionWith(t *testing.T, a *fakeAgent, cfg Config) *harness {
	t.Helper()
	r, w := io.Pipe()
	h := &harness{t: t, agent: a, out: &screen{}, writer: w, exited: make(chan struct{})}
	cfg.In, cfg.Out, cfg.Agent = r, h.out, a
	if cfg.Width == nil {
		cfg.Width = func() int { return 80 }
	}
	s := NewSession(cfg)
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
	status := "8 in · 4 out · ttft 7 ms · 1 steps"
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
			steps:   []step{{keys: "one\r"}, {turn: true}, {wait: status}, {keys: "two\r"}, {turn: true}, {wait: "0 steps"}, {keys: "\x04"}},
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
			steps: []step{{keys: "/help \r"}, {wait: "/quit"}, {keys: "/quit \r"}},
			want:  []string{"/model", "/jev on|off", "/new", "/runs", "//text"},
		},
		{
			name:  "model switches",
			steps: []step{{keys: "/model groq/fast\r"}, {wait: "model groq/fast"}, {keys: "/quit \r"}},
		},
		{
			name:  "model error stays in session",
			steps: []step{{keys: "/model nope\r"}, {wait: "error: bad model"}, {keys: "/quit \r"}},
		},
		{
			name:  "new resets the run",
			steps: []step{{keys: "/new \r"}, {wait: "new run"}, {keys: "/quit \r"}},
		},
		{
			name:  "runs lists runs",
			steps: []step{{keys: "/runs \r"}, {wait: "run-1  final"}, {keys: "/quit \r"}},
		},
		{
			name:  "unknown slash hints",
			steps: []step{{keys: "/nope\r"}, {wait: "unknown command /nope"}, {keys: "/quit \r"}},
		},
		{
			name:    "double slash sends literal",
			steps:   []step{{keys: "//text more\r"}, {turn: true}, {wait: status}, {keys: "/quit \r"}},
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
			want:    []string{"  read a.go", "  no such file", "limited on xai/grok", "bad[2Jout\r\nnext", "jev requests 1 tokens 2/3 · step limit"},
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

func TestSessionModelPickerFilter(t *testing.T) {
	a := newFakeAgent()
	a.providers = []string{"openai", "groq", "xai"}
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/model \r"},
		{wait: providerPrompt},
		{keys: "gr\r"},
		{wait: "model for groq: "},
		{keys: "fast\r"},
		{wait: "model groq/fast"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "groq/fast" {
		t.Fatalf("model %q", got)
	}
	a2 := newFakeAgent()
	a2.providers = []string{"openai", "groq", "xai"}
	h2 := startSession(t, a2, false, false)
	h2.play([]step{
		{keys: "/model \r"},
		{wait: providerPrompt},
		{keys: "zz\r\x1b"},
		{wait: pickerNote},
		{keys: "\x04"},
	})
	h2.waitExit()
	if got := a2.Model(); got != "xai/grok" {
		t.Fatalf("model %q", got)
	}
	a3 := newFakeAgent()
	a3.providers = []string{"openai", "groq", "xai"}
	h3 := startSession(t, a3, false, false)
	h3.play([]step{
		{keys: "/model \r"},
		{wait: providerPrompt},
		{keys: "gr\x1b"},
		{wait: pickerNote},
		{keys: "\x04"},
	})
	h3.waitExit()
	if got := a3.Model(); got != "xai/grok" {
		t.Fatalf("model %q", got)
	}
}

func TestSessionModelPicker(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/model \r"},
		{wait: providerPrompt},
		{keys: "\x1b[B\r"},
		{wait: "model for openrouter: "},
		{keys: "m2\r"},
		{wait: "model openrouter/m2"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "openrouter/m2" {
		t.Fatalf("model %q", got)
	}
	if !strings.Contains(h.out.String(), "openrouter/m2 · jev off") {
		t.Fatalf("footer missing: %q", h.out.String())
	}
}

func TestSessionModelPickerCancel(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
	}{
		{"esc on the provider", []step{{keys: "/model \r"}, {wait: providerPrompt}, {keys: "\x1b"}}},
		{"esc on the model", []step{{keys: "/model \r"}, {wait: providerPrompt}, {keys: "\r"}, {wait: "model for groq: "}, {keys: "\x1b"}}},
		{"empty model", []step{{keys: "/model \r"}, {wait: providerPrompt}, {keys: "\r"}, {wait: "model for groq: "}, {keys: "\r"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAgent()
			h := startSession(t, a, false, false)
			h.play(append(tc.steps, step{wait: pickerNote}, step{keys: "\x04"}))
			h.waitExit()
			if got := a.Model(); got != "xai/grok" {
				t.Fatalf("model %q", got)
			}
		})
	}
}

func TestSessionModelPickerError(t *testing.T) {
	a := newFakeAgent()
	a.modelErr = errors.New("no such model")
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/model \r"},
		{wait: providerPrompt},
		{keys: "\r"},
		{wait: "model for groq: "},
		{keys: "m2\r"},
		{wait: "error: no such model"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "xai/grok" {
		t.Fatalf("model %q", got)
	}
}

func TestSessionModelPickerWithoutProviders(t *testing.T) {
	a := newFakeAgent()
	a.providers = nil
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "/model \r"}, {wait: pickerEmpty}, {keys: "\x04"}})
	h.waitExit()
	if strings.Contains(h.out.String(), providerPrompt) {
		t.Fatalf("picker opened: %q", h.out.String())
	}
}

func TestStatusLine(t *testing.T) {
	tests := []struct {
		name string
		in   Outcome
		want string
	}{
		{
			"plain",
			Outcome{Model: "xai/grok", Steps: 1, InputTokens: 10, OutputTokens: 2, TTFT: 300 * time.Millisecond},
			"10 in · 2 out · ttft 300 ms · 1 steps",
		},
		{
			"jev",
			Outcome{InputTokens: 1, OutputTokens: 2, TTFT: 3 * time.Millisecond, Jev: "jev requests 1"},
			"1 in · 2 out · ttft 3 ms · 0 steps · jev requests 1",
		},
		{
			"note",
			Outcome{InputTokens: 1, OutputTokens: 2, TTFT: 3 * time.Millisecond, Note: "step limit"},
			"1 in · 2 out · ttft 3 ms · 0 steps · step limit",
		},
		{
			"jev and note",
			Outcome{Steps: 2, Jev: "jev requests 1", Note: "step limit"},
			"0 in · 0 out · ttft 0 ms · 2 steps · jev requests 1 · step limit",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusLine(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSessionHeader(t *testing.T) {
	header := "hp v0.0.2 · /help for commands"
	h := startSessionWith(t, newFakeAgent(), Config{Header: header})
	h.play([]step{{wait: header + "\r\n\r\n"}, {keys: "\x04"}})
	h.waitExit()
	if got := strings.Count(h.out.String(), header); got != 1 {
		t.Fatalf("header printed %d times in %q", got, h.out.String())
	}
}

func TestSessionHeaderThenIntro(t *testing.T) {
	h := startSessionWith(t, newFakeAgent(), Config{Header: "hp v · /help", Intro: "continuing run-1"})
	h.play([]step{{wait: "hp v · /help\r\ncontinuing run-1\r\n\r\n"}, {keys: "\x04"}})
	h.waitExit()
}

func TestSessionFooter(t *testing.T) {
	tests := []struct {
		name   string
		jevOn  bool
		want   string
		reject string
	}{
		{"jev on", true, "xai/grok · jev", "jev off"},
		{"jev off", false, "xai/grok · jev off", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAgent()
			a.jevOn = tc.jevOn
			h := startSession(t, a, false, false)
			h.play([]step{{wait: tc.want}, {keys: "\x04"}})
			h.waitExit()
			if tc.reject != "" && strings.Contains(h.out.String(), tc.reject) {
				t.Fatalf("found %q in %q", tc.reject, h.out.String())
			}
		})
	}
}

func TestSessionJevSwitch(t *testing.T) {
	const onFooter = "xai/grok · jev"
	const offFooter = "xai/grok · jev off"
	tests := []struct {
		name    string
		jevOn   bool
		jevErr  error
		keys    string
		note    string
		wantOn  bool
		footer  string
		minDraw int
	}{
		{"on switches", false, nil, "/jev on\r", "jev on\r\n", true, onFooter, 1},
		{"on without a key stays off", false, errors.New("jev needs a key: hp key set TYPESAFE_API_KEY"), "/jev on\r", "error: jev needs a key: hp key set TYPESAFE_API_KEY\r\n", false, offFooter, 2},
		{"off switches", true, nil, "/jev off\r", "jev off\r\n", false, offFooter, 1},
		{"status off", false, nil, "/jev \r", "jev off\r\n", false, offFooter, 2},
		{"status on", true, nil, "/jev \r", "jev on\r\n", true, onFooter, 2},
		{"bad argument", false, nil, "/jev maybe\r", jevUsage + "\r\n", false, offFooter, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAgent()
			a.jevOn, a.jevErr = tc.jevOn, tc.jevErr
			h := startSession(t, a, false, false)
			h.play([]step{{keys: tc.keys}, {wait: tc.note}, {keys: "\x04"}})
			h.waitExit()
			if a.Jev() != tc.wantOn {
				t.Fatalf("jev %v want %v", a.Jev(), tc.wantOn)
			}
			if got := footerLine(a); got != tc.footer {
				t.Fatalf("footer %q want %q", got, tc.footer)
			}
			if got := strings.Count(h.out.String(), tc.footer+"\x1b[2A"); got < tc.minDraw {
				t.Fatalf("footer drawn %d times, want %d: %q", got, tc.minDraw, h.out.String())
			}
		})
	}
}

func TestSessionFooterStaysOutOfScrollback(t *testing.T) {
	h := startSession(t, newFakeAgent(), false, false)
	footer := "xai/grok · jev off"
	h.play([]step{{keys: "go\r"}, {wait: "1 steps"}, {keys: "\x04"}})
	h.waitExit()
	out := h.out.String()
	if !strings.Contains(out, footer) {
		t.Fatalf("footer never drawn: %q", out)
	}
	if strings.Contains(out, footer+rowSeparator) {
		t.Fatalf("footer left in the transcript: %q", out)
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
	h.play([]step{{wait: "0 steps"}, {keys: "\x04"}})
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
	h.play([]step{{keys: "go\r"}, {wait: "ttft"}, {keys: "/help \r"}, {wait: "/quit"}, {keys: "/quit \r"}})
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

func TestSessionKeyEntry(t *testing.T) {
	tests := []struct {
		name    string
		saveErr error
		keys    string
		note    string
		saved   string
	}{
		{"saves masked value", nil, "/key FOO_KEY\rs3cr3t\r", "saved FOO_KEY\r\n", "FOO_KEY=s3cr3t"},
		{"esc cancels", nil, "/key FOO_KEY\rs3cr3t\x1b", keyNotSaved + "\r\n", ""},
		{"empty enter cancels", nil, "/key FOO_KEY\r\r", keyNotSaved + "\r\n", ""},
		{"no name", nil, "/key \r", keyUsage + "\r\n", ""},
		{"save error shown", errors.New("invalid key name \"foo\""), "/key foo\rs3cr3t\r", "error: invalid key name \"foo\"\r\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAgent()
			a.saveErr = tc.saveErr
			h := startSession(t, a, false, false)
			h.play([]step{{keys: tc.keys}, {wait: tc.note}, {keys: "\x1b[A"}, {keys: "\x15\x04"}})
			h.waitExit()
			if got := strings.Join(a.keys, ","); got != tc.saved {
				t.Fatalf("saved %q want %q", got, tc.saved)
			}
			if strings.Contains(h.out.String(), "s3cr3t") {
				t.Fatalf("secret reached the terminal: %q", h.out.String())
			}
		})
	}
}

func TestSessionPickModelAtStart(t *testing.T) {
	a := newFakeAgent()
	a.model = ""
	h := startSessionWith(t, a, Config{PickModel: true})
	h.play([]step{{wait: providerPrompt}, {keys: "\x1b"}, {keys: "\x04"}})
	h.waitExit()
}

func TestSessionFooterNoModel(t *testing.T) {
	a := newFakeAgent()
	a.model = ""
	h := startSessionWith(t, a, Config{})
	h.play([]step{{wait: "no model · /model"}, {keys: "\x04"}})
	h.waitExit()
}

func TestSessionPromptWithoutModelRestoresAfterPick(t *testing.T) {
	a := newFakeAgent()
	a.model = ""
	a.models = []string{"fast"}
	h := startSessionWith(t, a, Config{})
	h.play([]step{
		{wait: "no model · /model"},
		{keys: "hello\r"},
		{wait: "pick a model first"},
		{wait: providerPrompt},
		{keys: "gr\r"},
		{wait: "model for groq: "},
		{keys: "fast\r"},
		{wait: "model groq/fast"},
		{wait: "› hello"},
		{keys: "\x15\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "groq/fast" {
		t.Fatalf("model %q", got)
	}
}

func TestSessionPromptWithoutModelRestoresOnCancel(t *testing.T) {
	a := newFakeAgent()
	a.model = ""
	h := startSessionWith(t, a, Config{})
	h.play([]step{
		{keys: "hello\r"},
		{wait: "pick a model first"},
		{wait: providerPrompt},
		{keys: "\x1b"},
		{wait: "› hello"},
		{keys: "\x15\x04"},
	})
	h.waitExit()
}

func TestSessionSlashWorksWithoutModel(t *testing.T) {
	a := newFakeAgent()
	a.model = ""
	h := startSessionWith(t, a, Config{})
	h.play([]step{
		{keys: "/help \r"},
		{wait: "/model         pick a provider and model"},
		{keys: "/quit \r"},
	})
	h.waitExit()
}

func TestSessionKeyWithoutModel(t *testing.T) {
	a := newFakeAgent()
	a.model = ""
	h := startSessionWith(t, a, Config{})
	h.play([]step{
		{keys: "/key GROQ_API_KEY\r"},
		{wait: "key GROQ_API_KEY: "},
		{keys: "secret\r"},
		{wait: "saved GROQ_API_KEY"},
		{keys: "/quit \r"},
	})
	h.waitExit()
	if got := strings.Join(a.keys, ","); got != "GROQ_API_KEY=secret" {
		t.Fatalf("saved %q", got)
	}
}

func TestSessionPromptKeptWhenNoProviders(t *testing.T) {
	a := newFakeAgent()
	a.model = ""
	a.providers = nil
	h := startSessionWith(t, a, Config{})
	h.play([]step{
		{keys: "hello\r"},
		{wait: "pick a model first"},
		{wait: pickerEmpty},
		{wait: "› hello"},
		{keys: "\x15/quit \r"},
	})
	h.waitExit()
}

func TestSessionJevMissingKeyOffersKeyCommand(t *testing.T) {
	a := newFakeAgent()
	a.jevErr = KeyMissingError{Hint: "jev needs a key: hp key set TYPESAFE_API_KEY"}
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "/jev on\r"}, {wait: keyAddHint}, {keys: "\x04"}})
	h.waitExit()
}
