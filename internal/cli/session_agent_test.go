package cli

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
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
