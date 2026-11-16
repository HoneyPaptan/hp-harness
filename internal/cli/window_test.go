package cli

import (
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestContextWindowSettingsSpec(t *testing.T) {
	st := settings{ContextWindows: map[string]int{"groq/fast": 4096}}
	win := contextWindow(st, "groq/fast", "fast")
	if got := win("fast"); got != 4096 {
		t.Fatalf("got %d", got)
	}
}

func TestContextWindowSettingsBareModel(t *testing.T) {
	st := settings{ContextWindows: map[string]int{"openai/gpt-oss-120b": 9999}}
	win := contextWindow(st, "groq/openai/gpt-oss-120b", "openai/gpt-oss-120b")
	if got := win("openai/gpt-oss-120b"); got != 9999 {
		t.Fatalf("got %d", got)
	}
}

func TestContextWindowFallbackIgnoresRunSpec(t *testing.T) {
	st := settings{ContextWindows: map[string]int{"xai/grok": 5000}}
	win := contextWindow(st, "xai/grok", "grok")
	if got := win("other-model"); got != defaultContextWindow {
		t.Fatalf("got %d", got)
	}
	if got := win("grok"); got != 5000 {
		t.Fatalf("primary %d", got)
	}
}

func TestContextWindowPrefixTable(t *testing.T) {
	st := settings{}
	win := contextWindow(st, "xai/grok", "grok")
	cases := map[string]int{
		"gpt-oss-120b":    131072,
		"claude-sonnet-4": 200000,
		"llama-3.3-70b":   131072,
		"kimi-k2":         131072,
	}
	for model, want := range cases {
		if got := win(model); got != want {
			t.Fatalf("%s: got %d want %d", model, got, want)
		}
	}
}

func TestContextWindowDefault(t *testing.T) {
	win := contextWindow(settings{}, "xai/grok", "grok")
	if got := win("unknown-model"); got != defaultContextWindow {
		t.Fatalf("got %d", got)
	}
}

func TestRunDepsCompactionSettings(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, &fake.Provider{}, nil)
	pr := newPrinter(env, "id", nil)
	st := settings{
		AutoCompactPercent: ptrInt(25),
		ContextWindows:     map[string]int{"xai/grok": 5000},
	}
	ep := Endpoint{Preset: "xai", Model: "grok"}
	d := runDeps(env, "", ep, nil, tool.NewRegistry(), pr.observer(), nil, emptyRunPerm("/w"), st)
	if d.CompactPercent != 25 {
		t.Fatalf("percent %d", d.CompactPercent)
	}
	if d.ContextWindow == nil || d.ContextWindow("grok") != 5000 {
		t.Fatalf("window %d", d.ContextWindow("grok"))
	}
}

func ptrInt(n int) *int { return &n }
