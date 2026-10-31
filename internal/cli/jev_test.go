package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

const jevKeyError = "error: TYPESAFE_API_KEY or OPENROUTER_API_KEY must be set\n"

type jevServer struct {
	*httptest.Server
	hits atomic.Int64
}

func newJevServer(t *testing.T) *jevServer {
	t.Helper()
	s := &jevServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(s.answer))
	t.Cleanup(s.Close)
	return s
}

func (s *jevServer) answer(w http.ResponseWriter, r *http.Request) {
	s.hits.Add(1)
	var req struct {
		Questions map[string]json.RawMessage `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	answers := make([]string, 0, len(req.Questions))
	for name := range req.Questions {
		answers = append(answers, fmt.Sprintf(`%q:{"type":"noul","noul":0.9}`, name))
	}
	fmt.Fprintf(w, `{"answers":{%s},"usage":{"input_tokens":11,"output_tokens":3}}`, strings.Join(answers, ","))
}

func (s *jevServer) wire(env *Env) {
	env.NewJev = func(getenv func(string) string) (*jev.Client, error) {
		c, err := jev.FromEnv(getenv)
		if err != nil {
			return nil, err
		}
		c.URL = s.URL
		return &c, nil
	}
}

func hasTool(req provider.Request, name string) bool {
	for _, d := range req.Tools {
		if d.Name == name {
			return true
		}
	}
	return false
}

func onlyRunStart(t *testing.T, h *host.Memory, getenv func(string) string) map[string]any {
	t.Helper()
	ents, err := h.ReadDir(runsDir(getenv))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name, ".jsonl") {
			continue
		}
		return runStartFields(t, h, filepath.Join(runsDir(getenv), e.Name))
	}
	t.Fatal("no journal")
	return nil
}

func runStartFields(t *testing.T, h *host.Memory, jpath string) map[string]any {
	t.Helper()
	recs, err := journal.Read(h, jpath)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(recs[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func seedJevRun(t *testing.T, h *host.Memory, getenv func(string) string, id string, on bool) {
	t.Helper()
	start := journal.RunStart{
		RunID: id, Model: "m", System: "s", Dir: "/w", StepLimit: 50,
		Preset: "xai", BaseURL: "https://api.x.ai/v1", KeyEnv: "XAI_API_KEY", Jev: on,
	}
	w, err := journal.Create(h, filepath.Join(runsDir(getenv), id+".jsonl"), start)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("go"); err != nil {
		t.Fatal(err)
	}
}

func TestJevOff(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		args  []string
	}{
		{name: "default"},
		{name: "other env value", extra: map[string]string{"HP_JEV": "0"}},
		{name: "flag beats env", extra: map[string]string{"HP_JEV": "1"}, args: []string{"--jev=false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
			extra := map[string]string{"TYPESAFE_API_KEY": "k"}
			for k, v := range tt.extra {
				extra[k] = v
			}
			env, _, stderr := testEnv(t, h, fp, extra)
			env.NewJev = func(func(string) string) (*jev.Client, error) {
				t.Error("jev client built with jev off")
				return nil, jev.ErrNoKey
			}
			env.Args = append(append([]string{"run"}, tt.args...), "-m", "xai/grok", "go")
			if code := Main(context.Background(), env); code != 0 {
				t.Fatalf("code %d %s", code, stderr.String())
			}
			if hasTool(fp.Requests[0], "search") {
				t.Fatal("search registered")
			}
			if strings.Contains(strings.ToLower(stderr.String()), "jev") {
				t.Fatalf("stderr %q", stderr.String())
			}
			if _, ok := onlyRunStart(t, h, env.Getenv)["jev"]; ok {
				t.Fatal("run_start has jev key")
			}
		})
	}
}

func TestJevOn(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		args  []string
	}{
		{name: "flag", args: []string{"--jev"}},
		{name: "env", extra: map[string]string{"HP_JEV": "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			srv := newJevServer(t)
			fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
			extra := map[string]string{"OPENROUTER_API_KEY": "k"}
			for k, v := range tt.extra {
				extra[k] = v
			}
			env, _, stderr := testEnv(t, h, fp, extra)
			srv.wire(&env)
			env.Args = append(append([]string{"run"}, tt.args...), "-m", "xai/grok", "go")
			if code := Main(context.Background(), env); code != 0 {
				t.Fatalf("code %d %s", code, stderr.String())
			}
			if !hasTool(fp.Requests[0], "search") {
				t.Fatal("search missing")
			}
			if onlyRunStart(t, h, env.Getenv)["jev"] != true {
				t.Fatal("run_start lacks jev")
			}
		})
	}
}

func TestRunDepsChooser(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, &fake.Provider{}, nil)
	pr := newPrinter(env, "id", nil)
	if d := runDeps(env, Endpoint{}, nil, nil, pr.observer(), nil); d.ToolChooser != nil {
		t.Fatal("chooser set with jev off")
	}
	if d := runDeps(env, Endpoint{}, nil, nil, pr.observer(), &jev.Client{}); d.ToolChooser == nil {
		t.Fatal("chooser missing with jev on")
	}
}

func TestJevMissingKeysRefused(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		extra map[string]string
	}{
		{name: "flag", args: []string{"run", "--jev", "-m", "xai/grok", "go"}},
		{name: "env", args: []string{"run", "-m", "xai/grok", "go"}, extra: map[string]string{"HP_JEV": "1"}},
		{name: "detach", args: []string{"run", "-d", "--jev", "-m", "xai/grok", "go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			srv := newJevServer(t)
			fp := &fake.Provider{}
			env, _, stderr := testEnv(t, h, fp, tt.extra)
			srv.wire(&env)
			env.Spawn = func([]string, string) (int, error) {
				t.Error("child spawned")
				return 0, nil
			}
			env.Args = tt.args
			if code := Main(context.Background(), env); code != 2 {
				t.Fatalf("code %d", code)
			}
			if stderr.String() != jevKeyError {
				t.Fatalf("stderr %q", stderr.String())
			}
			if len(fp.Requests) != 0 || srv.hits.Load() != 0 {
				t.Fatalf("requests %d jev %d", len(fp.Requests), srv.hits.Load())
			}
			if ents, _ := h.ReadDir(runsDir(env.Getenv)); len(ents) != 0 {
				t.Fatalf("runs %d", len(ents))
			}
		})
	}
}

func TestResumeJevFromJournal(t *testing.T) {
	tests := []struct {
		name    string
		journal bool
		extra   map[string]string
		want    bool
	}{
		{name: "on without flag", journal: true, extra: map[string]string{"TYPESAFE_API_KEY": "k"}, want: true},
		{name: "on with env zero", journal: true, extra: map[string]string{"TYPESAFE_API_KEY": "k", "HP_JEV": "0"}, want: true},
		{name: "off with env one", extra: map[string]string{"TYPESAFE_API_KEY": "k", "HP_JEV": "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			srv := newJevServer(t)
			fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
			env, _, stderr := testEnv(t, h, fp, tt.extra)
			srv.wire(&env)
			seedJevRun(t, h, env.Getenv, "r1", tt.journal)
			env.Args = []string{"resume", "r1"}
			if code := Main(context.Background(), env); code != 0 {
				t.Fatalf("code %d %s", code, stderr.String())
			}
			if got := hasTool(fp.Requests[0], "search"); got != tt.want {
				t.Fatalf("search %v want %v", got, tt.want)
			}
			if got := strings.Contains(stderr.String(), "jev requests"); got != tt.want {
				t.Fatalf("summary %q", stderr.String())
			}
		})
	}
}

func TestResumeJevMissingKeys(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "foreground", args: []string{"resume", "r1"}},
		{name: "detach", args: []string{"resume", "-d", "r1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			srv := newJevServer(t)
			fp := &fake.Provider{}
			env, _, stderr := testEnv(t, h, fp, nil)
			srv.wire(&env)
			env.Spawn = func([]string, string) (int, error) {
				t.Error("child spawned")
				return 0, nil
			}
			seedJevRun(t, h, env.Getenv, "r1", true)
			env.Args = tt.args
			if code := Main(context.Background(), env); code != 2 {
				t.Fatalf("code %d", code)
			}
			if stderr.String() != jevKeyError {
				t.Fatalf("stderr %q", stderr.String())
			}
			if len(fp.Requests) != 0 || srv.hits.Load() != 0 {
				t.Fatalf("requests %d jev %d", len(fp.Requests), srv.hits.Load())
			}
		})
	}
}

func TestDetachedRunKeepsJev(t *testing.T) {
	h := host.NewMemory()
	srv := newJevServer(t)
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	env, stdout, stderr := testEnv(t, h, fp, map[string]string{"TYPESAFE_API_KEY": "k"})
	srv.wire(&env)
	var child []string
	env.Spawn = func(args []string, _ string) (int, error) {
		child = append([]string(nil), args...)
		return 42, nil
	}
	env.Args = []string{"run", "-d", "--jev", "-m", "xai/grok", "go"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	id := strings.TrimSpace(stdout.String())
	if len(child) != 2 || child[0] != "resume" || child[1] != id {
		t.Fatalf("child %v", child)
	}
	if runStartFields(t, h, filepath.Join(runsDir(env.Getenv), id+".jsonl"))["jev"] != true {
		t.Fatal("run_start lacks jev")
	}
	if err := h.Remove(runPidPath(env.Getenv, id)); err != nil {
		t.Fatal(err)
	}
	env.Args = child
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if !hasTool(fp.Requests[0], "search") {
		t.Fatal("search missing in child")
	}
}

func TestJevSummary(t *testing.T) {
	h := host.NewMemory()
	if err := h.WriteFile("/proj/a.go", []byte("package a\n")); err != nil {
		t.Fatal(err)
	}
	srv := newJevServer(t)
	call := provider.ToolCall{ID: "c1", Name: "search", Args: json.RawMessage(`{"query":"package"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("ok")}}
	env, _, stderr := testEnv(t, h, fp, map[string]string{"TYPESAFE_API_KEY": "k"})
	srv.wire(&env)
	env.Args = []string{"run", "--jev", "-m", "xai/grok", "go"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if srv.hits.Load() != 2 {
		t.Fatalf("jev requests %d", srv.hits.Load())
	}
	want := "  jev requests 2 tokens 22/6\n"
	if !strings.HasSuffix(stderr.String(), want) || strings.Count(stderr.String(), "jev requests") != 1 {
		t.Fatalf("stderr %q", stderr.String())
	}
}
