package cli

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/permit"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestResolvePermissionPrecedence(t *testing.T) {
	st := settings{PermissionMode: string(permit.ModeAsk)}
	cases := []struct {
		name       string
		full       bool
		env        string
		journal    string
		wantMode   permit.Mode
		wantSource string
	}{
		{"flag", true, "", "", permit.ModeFull, "flag"},
		{"env", false, "full-access", "", permit.ModeFull, "env"},
		{"journal", false, "", "ask", permit.ModeAsk, "journal"},
		{"settings", false, "", "", permit.ModeAsk, "settings"},
		{"default", false, "", "", permit.ModeAuto, "default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getenv := getenvMap(map[string]string{"HP_PERMISSION_MODE": tc.env})
			stUse := st
			if tc.name == "default" {
				stUse = settings{}
			}
			mode, src, err := resolvePermissionMode(tc.full, getenv, tc.journal, stUse)
			if err != nil {
				t.Fatal(err)
			}
			if mode != tc.wantMode || src != tc.wantSource {
				t.Fatalf("mode %q src %q", mode, src)
			}
		})
	}
}

func TestPermissionsJSON(t *testing.T) {
	home := t.TempDir()
	content := `{"permission_mode":"auto","permission":{"bash":{"git push *":"deny"}}}`
	writeSettingsFile(t, home, content)
	h := host.NewMemory()
	env, stdout, stderr := testEnv(t, h, nil, map[string]string{"HOME": home})
	env.Args = []string{"permissions", "--json"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	var out struct {
		Mode     string `json:"mode"`
		Source   string `json:"source"`
		Settings string `json:"settings"`
		Rules    []struct {
			Tool, Pattern, Verdict string
		} `json:"rules"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Mode != "auto" || out.Source != "settings" {
		t.Fatalf("%+v", out)
	}
	if !strings.HasSuffix(out.Settings, "settings.json") {
		t.Fatal(out.Settings)
	}
	if len(out.Rules) != 1 || out.Rules[0].Verdict != "deny" {
		t.Fatalf("%+v", out.Rules)
	}
}

func TestPermissionsBadSettingsExit2(t *testing.T) {
	home := t.TempDir()
	writeSettingsFile(t, home, `{"permission_mode":"nope"}`)
	env, _, stderr := testEnv(t, host.NewMemory(), nil, map[string]string{"HOME": home})
	env.Args = []string{"permissions", "--json"}
	if code := Main(context.Background(), env); code != 2 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "settings.json") {
		t.Fatal(stderr.String())
	}
}

func TestRunBashDenyByRule(t *testing.T) {
	home := t.TempDir()
	writeSettingsFile(t, home, `{"permission_mode":"auto","permission":{"bash":{"git push *":"deny"}}}`)
	h := host.NewMemory()
	var runs atomic.Int32
	h.ExecFunc = func(context.Context, host.ExecRequest) (host.ExecResult, error) {
		runs.Add(1)
		return host.ExecResult{}, nil
	}
	call := provider.ToolCall{ID: "1", Name: "bash", Args: json.RawMessage(`{"command":"git push origin main"}`)}
	fp := &fake.Provider{Turns: []fake.Turn{fake.ToolTurn(call), fake.TextTurn("done")}}
	env, _, stderr := testEnv(t, h, fp, map[string]string{"HOME": home})
	env.Args = []string{"run", "-m", "xai/grok", "push"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	if runs.Load() != 0 {
		t.Fatalf("runs %d", runs.Load())
	}
	want := "held: denied by rule bash git push *"
	last := fp.Requests[len(fp.Requests)-1].Messages[len(fp.Requests[len(fp.Requests)-1].Messages)-1]
	if len(last.ToolResults) != 1 || last.ToolResults[0].Text != want {
		t.Fatalf("results %+v", last.ToolResults)
	}
}
