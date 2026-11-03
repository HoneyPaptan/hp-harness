package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

type listingStub struct {
	fake.Provider
	mu     sync.Mutex
	ep     Endpoint
	called bool
	ids    []string
	err    error
	block  bool
}

func (s *listingStub) ListModels(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	s.called = true
	s.mu.Unlock()
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.ids, nil
}

func (s *listingStub) wasCalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.called
}

func sessionAgentWithLister(t *testing.T, ep Endpoint, stub *listingStub, extra map[string]string) *sessionAgent {
	t.Helper()
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, &fake.Provider{}, extra)
	env.NewProvider = func(got Endpoint) provider.Provider {
		stub.ep = got
		return stub
	}
	return &sessionAgent{env: env, ep: ep, bySpec: make(map[string]Endpoint)}
}

func TestSessionAgentNoModel(t *testing.T) {
	stub := &listingStub{ids: []string{"fast"}}
	a := sessionAgentWithLister(t, Endpoint{}, stub, map[string]string{"GROQ_API_KEY": "k"})
	if a.Model() != "" {
		t.Fatalf("model %q", a.Model())
	}
	_, err := a.Send(context.Background(), "hi", func(tui.Event) {})
	if !errors.Is(err, tui.NoModelError{}) {
		t.Fatalf("err = %v", err)
	}
	if a.id != "" || a.jpath != "" {
		t.Fatalf("run started id=%q jpath=%q", a.id, a.jpath)
	}
	if err := a.SetModel("groq/fast"); err != nil {
		t.Fatal(err)
	}
	if a.Model() != "groq/fast" {
		t.Fatalf("model %q", a.Model())
	}
}

func TestSessionAgentModelsGroqWithKey(t *testing.T) {
	stub := &listingStub{ids: []string{"m1", "m2"}}
	a := sessionAgentWithLister(t, Endpoint{Preset: "xai", Model: "g"}, stub, map[string]string{"GROQ_API_KEY": "gk"})
	got, err := a.Models(context.Background(), "groq")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "m1,m2" {
		t.Fatalf("got %v", got)
	}
	if stub.ep.BaseURL != presets["groq"].baseURL || stub.ep.APIKey != "gk" || stub.ep.Model != "" {
		t.Fatalf("endpoint = %+v", stub.ep)
	}
}

func TestSessionAgentModelsGroqNoKey(t *testing.T) {
	stub := &listingStub{ids: []string{"m1"}}
	a := sessionAgentWithLister(t, Endpoint{Preset: "xai", Model: "g"}, stub, nil)
	_, err := a.Models(context.Background(), "groq")
	if err == nil {
		t.Fatal("expected error")
	}
	var ue usageError
	if !errors.As(err, &ue) || !strings.Contains(ue.msg, "GROQ_API_KEY") {
		t.Fatalf("err = %v", err)
	}
	if stub.wasCalled() {
		t.Fatal("stub called")
	}
}

func TestSessionAgentModelsUnknownPreset(t *testing.T) {
	stub := &listingStub{}
	a := sessionAgentWithLister(t, Endpoint{}, stub, map[string]string{"GROQ_API_KEY": "k"})
	_, err := a.Models(context.Background(), "nosuch")
	if err == nil {
		t.Fatal("expected error")
	}
	var ue usageError
	if !errors.As(err, &ue) || !strings.Contains(ue.msg, "unknown preset") {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionAgentModelsStubError(t *testing.T) {
	want := errors.New("list failed")
	stub := &listingStub{err: want}
	a := sessionAgentWithLister(t, Endpoint{}, stub, map[string]string{"GROQ_API_KEY": "k"})
	_, err := a.Models(context.Background(), "groq")
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionAgentModelsTimeout(t *testing.T) {
	old := modelListTimeout
	modelListTimeout = 50 * time.Millisecond
	defer func() { modelListTimeout = old }()
	stub := &listingStub{block: true}
	a := sessionAgentWithLister(t, Endpoint{}, stub, map[string]string{"GROQ_API_KEY": "k"})
	start := time.Now()
	_, err := a.Models(context.Background(), "groq")
	if err == nil {
		t.Fatal("expected error")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("took too long")
	}
}

func TestSessionAgentModelsKeepsCurrentEndpoint(t *testing.T) {
	stub := &listingStub{ids: []string{"m"}}
	cur := Endpoint{Preset: "groq", Model: "llama", BaseURL: "https://custom.example/v1", KeyEnv: "GROQ_API_KEY"}
	a := sessionAgentWithLister(t, cur, stub, map[string]string{"GROQ_API_KEY": "k", "OPENAI_API_KEY": "o"})
	before := a.ep
	beforeID, beforeJpath := a.id, a.jpath
	_, err := a.Models(context.Background(), "openai")
	if err != nil {
		t.Fatal(err)
	}
	if a.ep != before || a.id != beforeID || a.jpath != beforeJpath {
		t.Fatalf("agent state changed")
	}
	if stub.ep.BaseURL != presets["openai"].baseURL {
		t.Fatalf("listed with %+v", stub.ep)
	}
	_, err = a.Models(context.Background(), "groq")
	if err != nil {
		t.Fatal(err)
	}
	if stub.ep.BaseURL != cur.BaseURL {
		t.Fatalf("current preset base URL = %q", stub.ep.BaseURL)
	}
}

func askModeAgent(t *testing.T, fp *fake.Provider) *sessionAgent {
	t.Helper()
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, fp, map[string]string{"HP_PERMISSION_MODE": "ask"})
	ep := Endpoint{Preset: "xai", Model: "grok", BaseURL: presets["xai"].baseURL, KeyEnv: "XAI_API_KEY"}
	spec := endpointSpec(ep)
	a, err := newSessionAgent(env, runOpts{}, ep, map[string]Endpoint{spec: ep}, nil, "/proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.release() })
	return a
}

func writeCall(id string) provider.ToolCall {
	return provider.ToolCall{ID: id, Name: "write", Args: json.RawMessage(`{"path":".env","contents":"x"}`)}
}

func TestSessionAgentAskWriteApprove(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(writeCall("1")),
		fake.TextTurn("done"),
	}}
	a := askModeAgent(t, fp)
	approved := make(chan struct{}, 1)
	_, err := a.Send(context.Background(), "go", func(ev tui.Event) {
		if ev.Kind == tui.EventApprove {
			if ev.Text != "write: .env" {
				t.Errorf("action %q", ev.Text)
			}
			approved <- struct{}{}
			ev.Reply <- tui.ApproveYes
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-approved:
	default:
		t.Fatal("no approval event")
	}
}

func TestSessionAgentAskAlwaysSkipsSecond(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(writeCall("1"), writeCall("2")),
		fake.TextTurn("done"),
	}}
	a := askModeAgent(t, fp)
	var n int
	_, err := a.Send(context.Background(), "go", func(ev tui.Event) {
		if ev.Kind == tui.EventApprove {
			n++
			ev.Reply <- tui.ApproveAlways
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("approvals %d", n)
	}
}

func TestSessionAgentAskNoHeld(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(writeCall("1")),
		fake.TextTurn("done"),
	}}
	a := askModeAgent(t, fp)
	_, err := a.Send(context.Background(), "go", func(ev tui.Event) {
		if ev.Kind == tui.EventApprove {
			ev.Reply <- tui.ApproveNo
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.Requests) < 2 {
		t.Fatalf("requests %d", len(fp.Requests))
	}
	if text := toolResults(fp.Requests[1])[0].Text; text != "held: declined by the user" {
		t.Fatalf("result %q", text)
	}
}

func TestSessionAgentAskCancelWaiting(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(writeCall("1")),
		fake.TextTurn("done"),
	}}
	a := askModeAgent(t, fp)
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = a.Send(ctx, "go", func(ev tui.Event) {
			if ev.Kind == tui.EventApprove {
				close(waiting)
			}
		})
		close(done)
	}()
	<-waiting
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stuck waiting for cancel")
	}
	if time.Since(start) > 800*time.Millisecond {
		t.Fatalf("slow cancel")
	}
}

func TestSessionAgentPermissionSwitchResume(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("one"), fake.TextTurn("two")}}
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, fp, nil)
	ep := Endpoint{Preset: "xai", Model: "grok", BaseURL: presets["xai"].baseURL, KeyEnv: "XAI_API_KEY"}
	spec := endpointSpec(ep)
	bySpec := map[string]Endpoint{spec: ep}
	a, err := newSessionAgent(env, runOpts{}, ep, bySpec, nil, "/proj")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Send(context.Background(), "hi", func(tui.Event) {}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetPermissionMode("ask"); err != nil {
		t.Fatal(err)
	}
	recs, err := journal.Read(h, a.jpath)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, rec := range recs {
		if rec.Type == journal.TypePermissionSwitch {
			saw = true
		}
	}
	if !saw {
		t.Fatal("no permission_switch")
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	id, jpath := a.id, a.jpath
	a.release()
	a2, err := restoredAgent(env, st, bySpec, ep, nil, id, jpath)
	if err != nil {
		t.Fatal(err)
	}
	defer a2.release()
	if a2.PermissionMode() != "ask" {
		t.Fatalf("mode %q", a2.PermissionMode())
	}
}
