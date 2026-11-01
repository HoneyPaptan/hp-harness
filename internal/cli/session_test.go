package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

const sessionWait = 5 * time.Second

type fakeTerm struct {
	in       *io.PipeReader
	keys     *io.PipeWriter
	mu       sync.Mutex
	out      bytes.Buffer
	raw      int
	restored int
	failRaw  bool
}

func newFakeTerm(t *testing.T) *fakeTerm {
	t.Helper()
	r, w := io.Pipe()
	t.Cleanup(func() { w.Close() })
	return &fakeTerm{in: r, keys: w}
}

func (f *fakeTerm) Read(p []byte) (int, error) { return f.in.Read(p) }

func (f *fakeTerm) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.Write(p)
}

func (f *fakeTerm) Interactive() bool { return true }

func (f *fakeTerm) MakeRaw() (func() error, error) {
	if f.failRaw {
		return nil, errors.New("no raw")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.raw++
	return func() error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.restored++
		return nil
	}, nil
}

func (f *fakeTerm) Width() int { return 80 }

func (f *fakeTerm) Signals(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(ctx)
}

func (f *fakeTerm) text() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.String()
}

func (f *fakeTerm) send(t *testing.T, keys string) {
	t.Helper()
	if _, err := io.WriteString(f.keys, keys); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeTerm) await(t *testing.T, text string, count int) {
	t.Helper()
	deadline := time.Now().Add(sessionWait)
	for strings.Count(f.text(), text) < count {
		if time.Now().After(deadline) {
			t.Fatalf("never saw %d of %q in %q", count, text, f.text())
		}
		time.Sleep(time.Millisecond)
	}
}

func startHP(t *testing.T, env Env) <-chan int {
	t.Helper()
	code := make(chan int, 1)
	go func() { code <- Main(context.Background(), env) }()
	return code
}

func awaitCode(t *testing.T, code <-chan int) int {
	t.Helper()
	select {
	case c := <-code:
		return c
	case <-time.After(sessionWait):
		t.Fatal("session did not exit")
		return 0
	}
}

func userTexts(msgs []provider.Message) []string {
	var out []string
	for _, m := range msgs {
		if m.Text != "" {
			out = append(out, m.Text)
		}
	}
	return out
}

func TestBareHPWithoutTerminal(t *testing.T) {
	cases := []struct {
		name string
		term Terminal
		args []string
	}{
		{name: "no terminal"},
		{name: "not interactive", term: pipedTerm{}},
		{name: "flags without terminal", args: []string{"-m", "xai/grok"}},
		{name: "unknown flag", args: []string{"--nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
			env.Term = tc.term
			env.Args = tc.args
			if code := Main(context.Background(), env); code != 2 {
				t.Fatalf("code %d", code)
			}
			if !strings.HasPrefix(stderr.String(), "usage: hp <run|") {
				t.Fatalf("stderr %q", stderr.String())
			}
		})
	}
}

type pipedTerm struct{ Terminal }

func (pipedTerm) Interactive() bool { return false }

func TestSessionUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "no model", want: "use -m preset/model or set HP_MODEL"},
		{name: "prompt", args: []string{"-m", "xai/grok", "hi"}, want: "use hp run"},
		{name: "detach", args: []string{"-m", "xai/grok", "-d"}, want: "use hp run -d"},
		{name: "no run to continue", args: []string{"-c"}, want: "no run to continue in /proj"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
			term := newFakeTerm(t)
			env.Term = term
			env.Args = tc.args
			if code := Main(context.Background(), env); code != 2 {
				t.Fatalf("code %d", code)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr %q", stderr.String())
			}
			if term.raw != 0 {
				t.Fatal("entered raw mode")
			}
		})
	}
}

func TestSessionRestoresTerminal(t *testing.T) {
	fp := &fake.Provider{}
	env, _, _ := testEnv(t, host.NewMemory(), fp, nil)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.await(t, "› ", 1)
	term.send(t, "\x04")
	if c := awaitCode(t, code); c != 0 {
		t.Fatalf("code %d", c)
	}
	if term.restored != 1 || !strings.HasSuffix(term.text(), "\x1b[?2004l") {
		t.Fatalf("restored %d output %q", term.restored, term.text())
	}
	if len(fp.Requests) != 0 {
		t.Fatalf("requests %d", len(fp.Requests))
	}
}

func TestSessionSecondTurnCarriesHistory(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("one"), fake.TextTurn("two")}}
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, fp, nil)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.send(t, "first\r")
	term.await(t, "xai/grok  steps 0", 1)
	if ids, _ := sortedRunIDs(env, runsDir(env.Getenv)); len(ids) != 1 || !runAlive(withAlive(env), ids[0]) {
		t.Fatalf("run not live: %v", ids)
	}
	term.send(t, "second\r")
	term.await(t, "xai/grok  steps 0", 2)
	term.send(t, "\x04")
	awaitCode(t, code)
	want := []string{"first", "one", "second"}
	if got := userTexts(fp.Requests[1].Messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("history %q", got)
	}
	ids, _ := sortedRunIDs(env, runsDir(env.Getenv))
	if len(ids) != 1 {
		t.Fatalf("runs %v", ids)
	}
	if _, ok := readRunPid(env, ids[0]); ok {
		t.Fatal("pid left behind")
	}
}

func withAlive(env Env) Env {
	env.Alive = func(int) bool { return true }
	return env
}

func TestSessionInterruptThenContinues(t *testing.T) {
	block := &blockOnce{fp: &fake.Provider{Turns: []fake.Turn{fake.TextTurn("after")}}, entered: make(chan struct{})}
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, nil, nil)
	env.NewProvider = func(Endpoint) provider.Provider { return block }
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.send(t, "first\r")
	<-block.entered
	term.send(t, "\x03")
	term.await(t, "interrupted", 1)
	term.await(t, "\r\n\r\n", 1)
	term.send(t, "again\r")
	term.await(t, "steps 0", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	want := []string{"first", "again"}
	if got := userTexts(block.fp.Requests[0].Messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("history %q", got)
	}
	ids, _ := sortedRunIDs(env, runsDir(env.Getenv))
	st, err := replayRun(env, filepath.Join(runsDir(env.Getenv), ids[0]+".jsonl"))
	if err != nil || !st.Finished {
		t.Fatalf("state %+v err %v", st, err)
	}
}

type blockOnce struct {
	once    sync.Once
	fp      *fake.Provider
	entered chan struct{}
}

func (b *blockOnce) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	first := false
	b.once.Do(func() { first = true })
	if !first {
		return b.fp.Stream(ctx, req)
	}
	return func(yield func(provider.Event, error) bool) {
		close(b.entered)
		<-ctx.Done()
		yield(provider.Event{}, ctx.Err())
	}
}

func seedDirRun(t *testing.T, h *host.Memory, getenv func(string) string, id, dir string, done bool) {
	t.Helper()
	start := journal.RunStart{
		RunID: id, Model: "grok", System: "s", Dir: dir, StepLimit: 50,
		Preset: "xai", BaseURL: "https://api.x.ai/v1", KeyEnv: "XAI_API_KEY",
	}
	w, err := journal.Create(h, filepath.Join(runsDir(getenv), id+".jsonl"), start)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("old " + id); err != nil {
		t.Fatal(err)
	}
	if !done {
		return
	}
	if err := w.Assistant(provider.Message{Role: provider.RoleAssistant, Text: "answer"}); err != nil {
		t.Fatal(err)
	}
	if err := w.End("final"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionContinuePicksMatchingDir(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("more")}}
	env, _, _ := testEnv(t, h, fp, nil)
	seedDirRun(t, h, env.Getenv, "20260101-000000-aaaa", "/proj", true)
	seedDirRun(t, h, env.Getenv, "20260102-000000-bbbb", "/proj", true)
	seedDirRun(t, h, env.Getenv, "20260103-000000-cccc", "/elsewhere", true)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-c"}
	code := startHP(t, env)
	term.await(t, "continuing 20260102-000000-bbbb", 1)
	term.send(t, "next\r")
	term.await(t, "xai/grok  steps", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	want := []string{"old 20260102-000000-bbbb", "answer", "next"}
	if got := userTexts(fp.Requests[0].Messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("history %q", got)
	}
}

func TestSessionContinueResumesUnfinishedFirst(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("settled")}}
	env, _, _ := testEnv(t, h, fp, nil)
	seedDirRun(t, h, env.Getenv, "20260101-000000-aaaa", "/proj", false)
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-c"}
	code := startHP(t, env)
	term.await(t, "settled", 1)
	term.await(t, "steps", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	if got := userTexts(fp.Requests[0].Messages); !reflect.DeepEqual(got, []string{"old 20260101-000000-aaaa"}) {
		t.Fatalf("history %q", got)
	}
}

func TestSessionRefusesLiveRun(t *testing.T) {
	h := host.NewMemory()
	env, _, stderr := testEnv(t, h, nil, nil)
	id := "20260101-000000-aaaa"
	seedDirRun(t, h, env.Getenv, id, "/proj", true)
	if err := writeRunPidFor(env, id, 99); err != nil {
		t.Fatal(err)
	}
	env.Alive = func(int) bool { return true }
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-c"}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "is running") || term.raw != 0 {
		t.Fatalf("stderr %q raw %d", stderr.String(), term.raw)
	}
}

func TestSessionModelSwitchSurvivesResume(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.TextTurn("one"), fake.TextTurn("two"), fake.TextTurn("three"),
	}}
	env, _, _ := testEnv(t, h, fp, map[string]string{"GROQ_API_KEY": "g"})
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.send(t, "first\r")
	term.await(t, "xai/grok  steps", 1)
	term.send(t, "/model groq/fast\r")
	term.await(t, "model groq/fast", 1)
	term.send(t, "second\r")
	term.await(t, "groq/fast  steps", 1)
	term.send(t, "\x04")
	awaitCode(t, code)
	if fp.Requests[1].Model != "fast" {
		t.Fatalf("model %q", fp.Requests[1].Model)
	}
	ids, _ := sortedRunIDs(env, runsDir(env.Getenv))
	jpath := filepath.Join(runsDir(env.Getenv), ids[0]+".jsonl")
	w, _, err := journal.Open(h, jpath)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("open again"); err != nil {
		t.Fatal(err)
	}
	env.Args = []string{"resume", ids[0]}
	if c := Main(context.Background(), env); c != 0 {
		t.Fatalf("code %d", c)
	}
	if fp.Requests[2].Model != "fast" {
		t.Fatalf("resume model %q", fp.Requests[2].Model)
	}
}

func TestSessionNewStartsAFreshRun(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("one"), fake.TextTurn("two")}}
	env, _, _ := testEnv(t, h, fp, nil)
	env.Rand = bytes.NewReader([]byte{1, 2, 3, 4})
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.send(t, "first\r")
	term.await(t, "steps", 1)
	term.send(t, "/new\r")
	term.await(t, "new run", 1)
	term.send(t, "second\r")
	term.await(t, "steps", 2)
	term.send(t, "/runs\r")
	term.await(t, "final", 2)
	term.send(t, "/quit\r")
	awaitCode(t, code)
	if got := userTexts(fp.Requests[1].Messages); !reflect.DeepEqual(got, []string{"second"}) {
		t.Fatalf("history %q", got)
	}
	if ids, _ := sortedRunIDs(env, runsDir(env.Getenv)); len(ids) != 2 {
		t.Fatalf("runs %v", ids)
	}
}

func TestColorOn(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "plain", env: map[string]string{"TERM": "xterm"}, want: true},
		{name: "no color", env: map[string]string{"TERM": "xterm", "NO_COLOR": "1"}},
		{name: "dumb", env: map[string]string{"TERM": "dumb"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := colorOn(getenvMap(tc.env)); got != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestEndpointFor(t *testing.T) {
	getenv := getenvMap(map[string]string{"GROQ_API_KEY": "g", "K": "k"})
	custom := Endpoint{Preset: "custom", Model: "a", BaseURL: "http://local", KeyEnv: "K"}
	cases := []struct {
		name     string
		cur      Endpoint
		spec     string
		wantSpec string
		wantBase string
		wantErr  bool
	}{
		{name: "custom keeps the endpoint", cur: custom, spec: "b", wantSpec: "custom/b", wantBase: "http://local"},
		{name: "custom prefix", cur: custom, spec: "custom/b", wantSpec: "custom/b", wantBase: "http://local"},
		{name: "custom prefix over a preset name", cur: custom, spec: "custom/groq/f", wantSpec: "custom/groq/f", wantBase: "http://local"},
		{name: "known preset uses its own endpoint", cur: custom, spec: "groq/f", wantSpec: "groq/f", wantBase: presets["groq"].baseURL},
		{name: "empty model", cur: custom, spec: "groq", wantErr: true},
		{name: "unknown preset", cur: Endpoint{Preset: "xai"}, spec: "nope/x", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ep, err := endpointFor(tc.cur, tc.spec, getenv)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got %+v", ep)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if endpointSpec(ep) != tc.wantSpec || ep.BaseURL != tc.wantBase {
				t.Fatalf("got %+v", ep)
			}
		})
	}
}
