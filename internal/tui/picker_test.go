package tui

import (
	"errors"
	"strings"
	"testing"
)

func modelPickerHarness(t *testing.T, a *fakeAgent) *harness {
	t.Helper()
	a.models = []string{"openai/gpt-oss-120b", "llama-3.3-70b", "qwen3-32b"}
	return startSession(t, a, false, false)
}

func openGroqModelStage(h *harness) {
	h.play([]step{
		{keys: "/model\r"},
		{wait: providerPrompt},
		{keys: "\r"},
		{wait: "model for groq: "},
	})
}

func TestModelPickerListPick(t *testing.T) {
	a := newFakeAgent()
	h := modelPickerHarness(t, a)
	openGroqModelStage(h)
	h.play([]step{
		{wait: pickerLoading},
		{keys: "gpt"},
		{wait: "gpt-oss-120b"},
		{keys: "\r"},
		{wait: "model groq/openai/gpt-oss-120b"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "groq/openai/gpt-oss-120b" {
		t.Fatalf("model %q", got)
	}
}

func TestModelPickerDownEnter(t *testing.T) {
	a := newFakeAgent()
	h := modelPickerHarness(t, a)
	openGroqModelStage(h)
	h.play([]step{
		{wait: "llama-3.3-70b"},
		{keys: "\x1b[B"},
		{keys: "\r"},
		{wait: "model groq/llama-3.3-70b"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "groq/llama-3.3-70b" {
		t.Fatalf("model %q", got)
	}
}

func TestModelPickerFilterBeforeLoad(t *testing.T) {
	wait := make(chan struct{})
	a := newFakeAgent()
	a.modelsWait = wait
	h := startSession(t, a, false, false)
	a.models = []string{"openai/gpt-oss-120b", "llama-3.3-70b", "qwen3-32b"}
	openGroqModelStage(h)
	h.play([]step{{keys: "gpt"}})
	close(wait)
	h.play([]step{
		{wait: "gpt-oss-120b"},
		{keys: "\r"},
		{wait: "model groq/openai/gpt-oss-120b"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "groq/openai/gpt-oss-120b" {
		t.Fatalf("model %q", got)
	}
}

func TestModelPickerFetchError(t *testing.T) {
	a := newFakeAgent()
	a.modelsErr = errors.New("offline")
	h := startSession(t, a, false, false)
	openGroqModelStage(h)
	h.play([]step{
		{wait: "models: offline"},
		{keys: "x\r"},
		{wait: "model groq/x"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "groq/x" {
		t.Fatalf("model %q", got)
	}
}

func TestModelPickerEmptyList(t *testing.T) {
	a := newFakeAgent()
	a.models = nil
	h := startSession(t, a, false, false)
	openGroqModelStage(h)
	h.play([]step{
		{wait: modelsNoneNote},
		{keys: "x\r"},
		{wait: "model groq/x"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "groq/x" {
		t.Fatalf("model %q", got)
	}
}

func TestModelPickerNoMatchTyped(t *testing.T) {
	a := newFakeAgent()
	h := modelPickerHarness(t, a)
	openGroqModelStage(h)
	h.play([]step{
		{wait: pickerLoading},
		{keys: "zzz\r"},
		{wait: "model groq/zzz"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "groq/zzz" {
		t.Fatalf("model %q", got)
	}
}

func TestModelPickerEscWhileLoading(t *testing.T) {
	wait := make(chan struct{})
	a := newFakeAgent()
	a.modelsWait = wait
	a.models = []string{"openai/gpt-oss-120b"}
	h := startSession(t, a, false, false)
	openGroqModelStage(h)
	h.play([]step{
		{wait: pickerLoading},
		{keys: "\x1b"},
		{wait: pickerNote},
	})
	close(wait)
	h.play([]step{{keys: "\x04"}})
	h.waitExit()
	if got := a.Model(); got != "xai/grok" {
		t.Fatalf("model %q", got)
	}
	if strings.Contains(h.out.String(), "gpt-oss-120b") {
		t.Fatalf("late list drew: %q", h.out.String())
	}
}

func TestModelSlashProviderOnly(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/model groq\r"},
		{wait: "model for groq: "},
		{keys: "\x1b"},
		{wait: pickerNote},
		{keys: "\x04"},
	})
	h.waitExit()
}

func TestModelSlashUnknownProvider(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "/model nope\r"},
		{wait: "error: bad model"},
		{keys: "\x04"},
	})
	h.waitExit()
	if got := a.Model(); got != "xai/grok" {
		t.Fatalf("model %q", got)
	}
}

func TestProviderOnlyArg(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"groq", "groq"},
		{"groq/", "groq"},
		{"groq/fast", ""},
		{"groq/a/b", ""},
	}
	for _, tc := range tests {
		if got := providerOnlyArg(tc.arg); got != tc.want {
			t.Fatalf("%q got %q want %q", tc.arg, got, tc.want)
		}
	}
}
