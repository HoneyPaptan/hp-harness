package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestWebToolsInRegistry(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, nil, nil)
	names := toolsetNames(openToolset(env, "/proj", nil, nil, settings{}, nil))
	if !hasName(names, "web_fetch") {
		t.Fatalf("default: %v", names)
	}
	if hasName(names, "web_search") {
		t.Fatalf("default search: %v", names)
	}
	envKey, _, _ := testEnv(t, h, nil, map[string]string{"EXA_API_KEY": "exa-key"})
	names = toolsetNames(openToolset(envKey, "/proj", nil, nil, settings{}, nil))
	if !hasName(names, "web_fetch") || !hasName(names, "web_search") {
		t.Fatalf("with key: %v", names)
	}
	off := false
	envOff, _, _ := testEnv(t, h, nil, nil)
	names = toolsetNames(openToolset(envOff, "/proj", nil, nil, settings{Web: &off}, nil))
	if hasName(names, "web_fetch") || hasName(names, "web_search") {
		t.Fatalf("web off: %v", names)
	}
}

func toolsetNames(ts *toolset) []string {
	var out []string
	for _, d := range ts.reg.Defs() {
		out = append(out, d.Name)
	}
	return out
}

func TestRunWebFetchAllowSkipsReview(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("secret page"))
	}))
	t.Cleanup(srv.Close)
	h := host.NewMemory()
	pageURL := srv.URL + "/page"
	call := provider.ToolCall{ID: "1", Name: "web_fetch", Args: json.RawMessage(`{"url":"` + pageURL + `"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("done")}}
	home := t.TempDir()
	env, _, stderr := testEnv(t, h, fp, map[string]string{"HOME": home})
	writeSettingsFile(t, home, `{"permission":{"web_fetch":{"`+srv.URL+`/*":"allow"}}}`)
	env.Args = []string{"run", "-m", "xai/grok", "fetch"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	for _, req := range fp.Requests {
		if req.System == reviewSystemPrompt {
			t.Fatal("reviewer ran despite allow rule")
		}
	}
}

func TestRunWebFetchPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("never seen"))
	}))
	t.Cleanup(srv.Close)
	h := host.NewMemory()
	pageURL := srv.URL
	call := provider.ToolCall{ID: "1", Name: "web_fetch", Args: json.RawMessage(`{"url":"` + pageURL + `"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("done")}}
	env, _, stderr := testEnv(t, h, fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "--full-access", "fetch"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if len(fp.Requests) < 2 {
		t.Fatal("missing follow-up request")
	}
	last := fp.Requests[len(fp.Requests)-1]
	msg := last.Messages[len(last.Messages)-1]
	if len(msg.ToolResults) == 0 || !strings.Contains(msg.ToolResults[0].Text, "refused: private address") {
		t.Fatalf("result %q", msg.ToolResults[0].Text)
	}
}

func TestRunWebFetchAllowPrivate(t *testing.T) {
	const body = "local docs content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	h := host.NewMemory()
	pageURL := srv.URL
	call := provider.ToolCall{ID: "1", Name: "web_fetch", Args: json.RawMessage(`{"url":"` + pageURL + `"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("done")}}
	env, _, stderr := testEnv(t, h, fp, map[string]string{"HP_WEB_ALLOW_PRIVATE": "1"})
	env.Args = []string{"run", "-m", "xai/grok", "--full-access", "fetch"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if len(fp.Requests) < 2 {
		t.Fatal("missing follow-up request")
	}
	last := fp.Requests[len(fp.Requests)-1]
	msg := last.Messages[len(last.Messages)-1]
	if len(msg.ToolResults) == 0 || !strings.Contains(msg.ToolResults[0].Text, body) {
		t.Fatalf("result %q", msg.ToolResults[0].Text)
	}
}

func TestToolMainArgWeb(t *testing.T) {
	if got := toolMainArg(json.RawMessage(`{"url":"https://pkg.go.dev/std"}`)); got != "https://pkg.go.dev/std" {
		t.Fatalf("url %q", got)
	}
	if got := toolMainArg(json.RawMessage(`{"query":"go modules"}`)); got != "go modules" {
		t.Fatalf("query %q", got)
	}
}

func TestRunWebFetchToolLine(t *testing.T) {
	const body = "x"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	h := host.NewMemory()
	call := provider.ToolCall{ID: "1", Name: "web_fetch", Args: json.RawMessage(`{"url":"` + srv.URL + `"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("ok")}}
	env, _, stderr := testEnv(t, h, fp, map[string]string{"HP_WEB_ALLOW_PRIVATE": "1"})
	env.Args = []string{"run", "-m", "xai/grok", "--full-access", "go"}
	if Main(context.Background(), env) != 0 {
		t.Fatal(stderr.String())
	}
	if !strings.Contains(stderr.String(), "> web_fetch "+srv.URL) {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestPrinterWebSearchToolLine(t *testing.T) {
	h := host.NewMemory()
	env, _, stderr := testEnv(t, h, nil, nil)
	p := newPrinter(env, "id", nil)
	p.onToolStart(provider.ToolCall{Name: "web_search", Args: json.RawMessage(`{"query":"go modules"}`)})
	if !strings.Contains(stderr.String(), "> web_search go modules") {
		t.Fatalf("stderr %q", stderr.String())
	}
}
