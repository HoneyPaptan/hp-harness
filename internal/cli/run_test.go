package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"iter"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func testEnv(t *testing.T, h *host.Memory, fp *fake.Provider, extra map[string]string) (Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	m := map[string]string{"HOME": "/home/t", "XDG_STATE_HOME": "/state", "XAI_API_KEY": "key"}
	for k, v := range extra {
		m[k] = v
	}
	h.Clock = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return Env{
		Args: nil, Getenv: getenvMap(m), Cwd: "/proj",
		Stdout: stdout, Stderr: stderr, Host: h,
		Rand: bytes.NewReader([]byte{1, 2}), Version: "dev",
		NewProvider: func(ep Endpoint) provider.Provider { return fp },
	}, stdout, stderr
}

func TestRunFinalAnswer(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("hello world")}}
	env, stdout, stderr := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "say hi"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	if stdout.String() != "hello world\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "run ") || !strings.Contains(stderr.String(), "final") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestRunToolLine(t *testing.T) {
	h := host.NewMemory()
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"a.go"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("ok")}}
	env, _, stderr := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "go"}
	if Main(context.Background(), env) != 0 {
		t.Fatal(stderr.String())
	}
	if !strings.Contains(stderr.String(), "> read a.go") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestRunStepLimit(t *testing.T) {
	h := host.NewMemory()
	call := provider.ToolCall{ID: "1", Name: "read", Args: json.RawMessage(`{"path":"x"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("nope")}}
	env, _, stderr := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "--steps", "1", "work"}
	if code := Main(context.Background(), env); code != 3 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "step_limit") {
		t.Fatal(stderr.String())
	}
}

func TestRunProviderError(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{{Err: errors.New("boom")}}}
	env, _, stderr := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "x"}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "resume with:") {
		t.Fatal(stderr.String())
	}
}

func TestRunInterrupt(t *testing.T) {
	h := host.NewMemory()
	block := blockProvider{}
	env, _, stderr := testEnv(t, h, nil, nil)
	env.NewProvider = func(Endpoint) provider.Provider { return block }
	env.Args = []string{"run", "-m", "xai/grok", "x"}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	if code := Main(ctx, env); code != 130 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "interrupted, resume with:") {
		t.Fatal(stderr.String())
	}
}

type blockProvider struct{}

func (blockProvider) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		if !yield(provider.Event{Kind: provider.EventText, Text: "p"}, nil) {
			return
		}
		<-ctx.Done()
		yield(provider.Event{}, ctx.Err())
	}
}

func TestRunMissingKeyNoJournal(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("x")}}
	env, _, stderr := testEnv(t, h, fp, map[string]string{"XAI_API_KEY": ""})
	env.Args = []string{"run", "-m", "xai/grok", "hi"}
	if code := Main(context.Background(), env); code != 2 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "XAI_API_KEY") {
		t.Fatal(stderr.String())
	}
	dir := runsDir(env.Getenv)
	ents, _ := h.ReadDir(dir)
	if len(ents) != 0 {
		t.Fatalf("runs %d", len(ents))
	}
}

func TestRunNoModel(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{}
	env, _, stderr := testEnv(t, h, fp, map[string]string{"HP_MODEL": ""})
	env.Args = []string{"run", "hi"}
	if Main(context.Background(), env) != 2 {
		t.Fatal(stderr.String())
	}
	if !strings.Contains(stderr.String(), "preset/model") {
		t.Fatal(stderr.String())
	}
}

func TestRunHPMModel(t *testing.T) {
	h := host.NewMemory()
	var got Endpoint
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	env, _, _ := testEnv(t, h, fp, map[string]string{"HP_MODEL": "xai/grok"})
	env.NewProvider = func(ep Endpoint) provider.Provider {
		got = ep
		return fp
	}
	env.Args = []string{"run", "hi"}
	if Main(context.Background(), env) != 0 {
		t.Fatal()
	}
	if got.Model != "grok" || got.Preset != "xai" {
		t.Fatalf("%+v", got)
	}
}

func TestRunSkillTool(t *testing.T) {
	h := host.NewMemory()
	dir := "/proj"
	skillRoot := path.Join(dir, ".hp", "skills", "demo")
	body := "---\nname: demo\ndescription: does things\n---\nbody\n"
	h.WriteFile(path.Join(skillRoot, "SKILL.md"), []byte(body))
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	env, _, _ := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "use skill"}
	if Main(context.Background(), env) != 0 {
		t.Fatal()
	}
	if len(fp.Requests) == 0 {
		t.Fatal("no requests")
	}
	names := toolNames(fp.Requests[0].Tools)
	if !hasName(names, "skill") {
		t.Fatalf("tools %v", names)
	}
}

func toolNames(defs []provider.ToolDef) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}

func hasName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

func TestRunNoSkillTool(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("done")}}
	env, _, _ := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "x"}
	Main(context.Background(), env)
	for _, n := range toolNames(fp.Requests[0].Tools) {
		if n == "skill" {
			t.Fatal("unexpected skill tool")
		}
	}
}

var _ provider.Provider = blockProvider{}

func TestRunInterruptEndsStdoutLine(t *testing.T) {
	h := host.NewMemory()
	turn := fake.Turn{Events: []provider.Event{{Kind: provider.EventText, Text: "partial"}}, Err: context.Canceled}
	fp := &fake.Provider{Turns: []fake.Turn{turn}}
	env, stdout, _ := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "x"}
	if code := Main(context.Background(), env); code != 130 {
		t.Fatalf("code %d", code)
	}
	if stdout.String() != "partial\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestRunToolFirstHasNoBlankLine(t *testing.T) {
	h := host.NewMemory()
	h.WriteFile("/proj/a.txt", []byte("x"))
	call := provider.ToolCall{ID: "c1", Name: "read", Args: json.RawMessage(`{"path":"a.txt"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("done")}}
	env, stdout, _ := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "x"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	if stdout.String() != "done\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
}
