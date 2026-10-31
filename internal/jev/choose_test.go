package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

var _ core.ToolChooser = (*Chooser)(nil)

func toolDefs(n int) []provider.ToolDef {
	named := []string{"alpha", "edit", "beta", "read", "gamma", "bash"}
	defs := make([]provider.ToolDef, n)
	for i := range defs {
		name := fmt.Sprintf("mcp.tool-%d", i)
		if i < len(named) {
			name = named[i]
		}
		defs[i] = provider.ToolDef{Name: name, Description: "does " + name}
	}
	return defs
}

func defNames(defs []provider.ToolDef) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func scoreTool(name string, s float64) func(string) float64 {
	return func(text string) float64 {
		if strings.Contains(text, "`"+name+"`") {
			return s
		}
		return 0.01
	}
}

func chooseMsgs() []provider.Message {
	return []provider.Message{{Role: provider.RoleUser, Text: "fix the bug"}}
}

func TestChooseNarrows(t *testing.T) {
	tests := []struct {
		name     string
		tools    int
		score    func(string) float64
		want     []string
		requests int
	}{
		{"prunes twelve", 12, scoreTool("edit", 0.9), []string{"edit", "read", "bash"}, 1},
		{"eight sends nothing", 8, scoreTool("edit", 0.9), defNames(toolDefs(8)), 0},
		{"nine asks", 9, scoreTool("edit", 0.9), []string{"edit", "read", "bash"}, 1},
		{"exactly the threshold is kept", 12, scoreTool("edit", 0.2), []string{"edit", "read", "bash"}, 1},
		{"just under the threshold is dropped", 12, scoreTool("edit", 0.1999), []string{"read", "bash"}, 1},
		{"unusual names", 12, scoreTool("mcp.tool-7", 0.5), []string{"read", "bash", "mcp.tool-7"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, f := searchServer(t, tt.score)
			got := NewChooser(c).Choose(context.Background(), chooseMsgs(), toolDefs(tt.tools))
			if !reflect.DeepEqual(defNames(got), tt.want) {
				t.Fatalf("tools %v, want %v", defNames(got), tt.want)
			}
			if f.requests() != tt.requests {
				t.Fatalf("requests %d, want %d", f.requests(), tt.requests)
			}
		})
	}
}

func TestChooseQuestions(t *testing.T) {
	c, f := searchServer(t, scoreTool("edit", 0.9))
	NewChooser(c).Choose(context.Background(), chooseMsgs(), toolDefs(12))
	var req wireRequest
	if err := json.Unmarshal([]byte(f.sent()), &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Questions) != 12 {
		t.Fatalf("questions %d", len(req.Questions))
	}
	got := req.Questions[questionName(7)]
	want := "Will the agent's next step call the tool `mcp.tool-7` (does mcp.tool-7)?"
	if got.Type != "noul" || got.Instructions != want {
		t.Fatalf("question %+v", got)
	}
}

func hangUntilDone(release <-chan struct{}, onStart func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		onStart()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}
}

func fakeServer(t *testing.T, handler func(release <-chan struct{}) http.HandlerFunc) *Client {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(handler(release))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return &Client{URL: srv.URL, Key: "k", Model: "m"}
}

func respond(status int, body string) func(<-chan struct{}) http.HandlerFunc {
	return func(<-chan struct{}) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			fmt.Fprint(w, body)
		}
	}
}

func TestChooseKeepsAllOnFailure(t *testing.T) {
	tests := []struct {
		name    string
		handler func(<-chan struct{}) http.HandlerFunc
	}{
		{"bad request", respond(400, "nope")},
		{"bad body", respond(200, "not json")},
		{"no answers", respond(200, `{"answers":{}}`)},
		{"slow server", func(release <-chan struct{}) http.HandlerFunc {
			return hangUntilDone(release, func() {})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewChooser(fakeServer(t, tt.handler))
			ch.timeout = 50 * time.Millisecond
			defs := toolDefs(12)
			got := ch.Choose(context.Background(), chooseMsgs(), defs)
			if !reflect.DeepEqual(got, defs) {
				t.Fatalf("tools %v", defNames(got))
			}
		})
	}
}

func TestChooseStopsOnCallerCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := fakeServer(t, func(release <-chan struct{}) http.HandlerFunc {
		return hangUntilDone(release, cancel)
	})
	defs := toolDefs(12)
	got := NewChooser(c).Choose(ctx, chooseMsgs(), defs)
	if !reflect.DeepEqual(got, defs) {
		t.Fatalf("tools %v", defNames(got))
	}
}

func TestChooseStatePieces(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Text: "first"},
		{Role: provider.RoleAssistant, Text: "older"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "grep"}}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{Text: "old result"}}},
		{Role: provider.RoleAssistant, Text: "latest"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "2", Name: "read"}, {ID: "3", Name: "bash"}}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{Text: "one"}, {Text: "two"}}},
	}
	want := chooseState{FirstUser: "first", LastAssistant: "latest", LastTools: "read, bash"}
	if got := buildChooseState(msgs); got != want {
		t.Fatalf("state %+v", got)
	}
}

func TestChooseNeverSendsToolOutput(t *testing.T) {
	c, f := searchServer(t, scoreTool("edit", 0.9))
	msgs := []provider.Message{
		{Role: provider.RoleUser, Text: "fix the deploy"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read"}}},
		{Role: provider.RoleUser, ToolResults: []provider.ToolResult{{Text: "API_TOKEN=hunter2"}}},
	}
	NewChooser(c).Choose(context.Background(), msgs, toolDefs(12))
	if sent := f.sent(); sent == "" || strings.Contains(sent, "hunter2") {
		t.Fatalf("sent %q", sent)
	}
}

func TestChooseStateCaps(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"ascii", strings.Repeat("x", 3<<10)},
		{"cut inside a rune", "x" + strings.Repeat("é", 2<<10)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, f := searchServer(t, scoreTool("edit", 0.9))
			msgs := []provider.Message{
				{Role: provider.RoleUser, Text: tt.text},
				{Role: provider.RoleAssistant, Text: tt.text},
				{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{Name: tt.text}}},
			}
			NewChooser(c).Choose(context.Background(), msgs, toolDefs(12))
			var req struct {
				State chooseState `json:"state"`
			}
			if err := json.Unmarshal([]byte(f.sent()), &req); err != nil {
				t.Fatal(err)
			}
			pieces := []string{req.State.FirstUser, req.State.LastAssistant, req.State.LastTools}
			for _, piece := range pieces {
				if len(piece) > maxChooseState || len(piece) < maxChooseState-1 {
					t.Fatalf("piece is %d bytes", len(piece))
				}
				if !utf8.ValidString(piece) || strings.ContainsRune(piece, utf8.RuneError) {
					t.Fatalf("piece is not clean text")
				}
			}
		})
	}
}
