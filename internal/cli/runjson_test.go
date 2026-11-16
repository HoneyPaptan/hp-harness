package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func jsonLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("stdout line is not JSON: %q", line)
		}
		events = append(events, ev)
	}
	return events
}

func eventTypes(events []map[string]any) []string {
	var types []string
	for _, ev := range events {
		types = append(types, ev["type"].(string))
	}
	return types
}

func TestRunJSONStreamsEvents(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "read", Args: json.RawMessage(`{"path":"nope.txt"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("done")}}
	env, stdout, _ := testEnv(t, host.NewMemory(), fp, nil)
	env.Args = []string{"run", "--json", "-m", "xai/grok", "look"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	events := jsonLines(t, stdout.String())
	if got, want := eventTypes(events), []string{"run", "tool_call", "tool_result", "text", "end"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("types %v want %v", got, want)
	}
	if events[0]["model"] != "xai/grok" || events[0]["run_id"] == "" {
		t.Fatalf("run event %v", events[0])
	}
	if events[1]["id"] != "c1" || events[1]["name"] != "read" || events[2]["id"] != "c1" || events[2]["is_error"] != true {
		t.Fatalf("tool events %v %v", events[1], events[2])
	}
	end := events[4]
	if end["reason"] != "final" || end["text"] != "done" || end["exit_code"] != float64(0) || end["run_id"] != events[0]["run_id"] {
		t.Fatalf("end %v", end)
	}
	if _, ok := end["usage"].(map[string]any)["input_tokens"]; !ok {
		t.Fatalf("usage %v", end["usage"])
	}
}

func TestRunJSONReportsFailure(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{{Err: errors.New("boom")}}}
	env, stdout, stderr := testEnv(t, host.NewMemory(), fp, nil)
	env.Args = []string{"run", "--json", "-m", "xai/grok", "go"}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d", code)
	}
	events := jsonLines(t, stdout.String())
	end := events[len(events)-1]
	if end["type"] != "end" || end["reason"] != "error" || end["error"] != "boom" || end["exit_code"] != float64(1) {
		t.Fatalf("end %v", end)
	}
	if !strings.Contains(stderr.String(), "resume with: hp resume") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestRunJSONStepLimitExitsThree(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "read", Args: json.RawMessage(`{"path":"nope.txt"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call)}}
	env, stdout, _ := testEnv(t, host.NewMemory(), fp, nil)
	env.Args = []string{"run", "--json", "-steps", "1", "-m", "xai/grok", "go"}
	if code := Main(context.Background(), env); code != 3 {
		t.Fatalf("code %d", code)
	}
	events := jsonLines(t, stdout.String())
	if end := events[len(events)-1]; end["reason"] != "step_limit" || end["exit_code"] != float64(3) {
		t.Fatalf("end %v", end)
	}
}

func TestRunJSONRefusesDetach(t *testing.T) {
	env, _, stderr := testEnv(t, host.NewMemory(), &fake.Provider{}, nil)
	env.Args = []string{"run", "--json", "-d", "-m", "xai/grok", "go"}
	if code := Main(context.Background(), env); code != 2 || !strings.Contains(stderr.String(), "--json cannot be combined with -d") {
		t.Fatalf("code %d %q", code, stderr.String())
	}
}

type ttyFlag bool

func (t ttyFlag) check() bool { return bool(t) }

func TestRunReadsThePromptFromStdin(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"piped with no argument", []string{"run", "-m", "xai/grok"}, "fix the test\n", "fix the test"},
		{"dash reads stdin", []string{"run", "-m", "xai/grok", "-"}, "  from a file\n\n", "from a file"},
		{"an argument wins over stdin", []string{"run", "-m", "xai/grok", "from", "argv"}, "ignored", "from argv"},
		{"a lone dash after flags", []string{"run", "-m", "xai/grok", "--", "-"}, "dash\n", "dash"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
			env, _, stderr := testEnv(t, host.NewMemory(), fp, nil)
			env.Args = tc.args
			env.Stdin = strings.NewReader(tc.stdin)
			env.StdinIsTerminal = ttyFlag(false).check
			if code := Main(context.Background(), env); code != 0 {
				t.Fatalf("code %d %s", code, stderr.String())
			}
			if got := fp.Requests[0].Messages[0].Text; got != tc.want {
				t.Fatalf("prompt %q want %q", got, tc.want)
			}
		})
	}
}

func TestRunWithoutAPromptSaysSo(t *testing.T) {
	tests := []struct {
		name  string
		stdin *bytes.Buffer
		tty   bool
		want  string
	}{
		{"terminal stdin", bytes.NewBufferString("never read"), true, "prompt required"},
		{"empty pipe", bytes.NewBufferString("  \n"), false, "prompt required"},
		{"over the limit", bytes.NewBufferString(strings.Repeat("a", stdinPromptLimit+1)), false, "over 8 MiB"},
		{"no stdin at all", nil, false, "prompt required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, _, stderr := testEnv(t, host.NewMemory(), &fake.Provider{}, nil)
			env.Args = []string{"run", "-m", "xai/grok"}
			if tc.stdin != nil {
				env.Stdin = tc.stdin
			}
			env.StdinIsTerminal = ttyFlag(tc.tty).check
			if code := Main(context.Background(), env); code != 2 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("code %d %q", code, stderr.String())
			}
		})
	}
}
