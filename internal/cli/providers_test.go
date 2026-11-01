package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPresetEndpoints(t *testing.T) {
	cases := []struct {
		preset, url, key string
	}{
		{"opencode", "https://opencode.ai/inference/openai/v1", "OPENCODE_API_KEY"},
		{"openai", "https://api.openai.com/v1", "OPENAI_API_KEY"},
		{"anthropic", "https://api.anthropic.com/v1", "ANTHROPIC_API_KEY"},
		{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "GEMINI_API_KEY"},
		{"xai", "https://api.x.ai/v1", "XAI_API_KEY"},
		{"deepseek", "https://api.deepseek.com", "DEEPSEEK_API_KEY"},
		{"mistral", "https://api.mistral.ai/v1", "MISTRAL_API_KEY"},
		{"cerebras", "https://api.cerebras.ai/v1", "CEREBRAS_API_KEY"},
		{"together", "https://api.together.ai/v1", "TOGETHER_API_KEY"},
		{"fireworks", "https://api.fireworks.ai/inference/v1", "FIREWORKS_API_KEY"},
		{"openrouter", "https://openrouter.ai/api/v1", "OPENROUTER_API_KEY"},
		{"groq", "https://api.groq.com/openai/v1", "GROQ_API_KEY"},
		{"ollama", "http://127.0.0.1:11434/v1", ""},
	}
	if len(cases) != len(presets) {
		t.Fatalf("%d cases for %d presets", len(cases), len(presets))
	}
	for _, tc := range cases {
		t.Run(tc.preset, func(t *testing.T) {
			getenv := getenvMap(map[string]string{tc.key: "v"})
			ep, err := resolveEndpoint(tc.preset+"/some/model", "", "", getenv)
			if err != nil {
				t.Fatal(err)
			}
			if ep.BaseURL != tc.url || ep.KeyEnv != tc.key || ep.Model != "some/model" {
				t.Fatalf("endpoint = %+v", ep)
			}
		})
	}
}

func providerRows(t *testing.T, vars map[string]string, args ...string) (string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := Env{Stdout: &stdout, Stderr: &stderr, Getenv: getenvMap(vars)}
	code := cmdProviders(env, args)
	return stdout.String(), code
}

func TestProvidersShowsKeyStateAndNeverAValue(t *testing.T) {
	out, code := providerRows(t, map[string]string{"ANTHROPIC_API_KEY": "sk-secret-value"})
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if strings.Contains(out, "sk-secret-value") {
		t.Fatalf("key leaked:\n%s", out)
	}
	cases := []struct {
		preset, want string
	}{
		{"anthropic", "ANTHROPIC_API_KEY set"},
		{"openai", "OPENAI_API_KEY not set"},
		{"ollama", "no key needed"},
	}
	for _, tc := range cases {
		row := rowFor(out, tc.preset)
		if !strings.HasSuffix(row, tc.want) || !strings.Contains(row, presets[tc.preset].baseURL) {
			t.Fatalf("row for %s = %q", tc.preset, row)
		}
	}
}

func rowFor(out, preset string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, preset+" ") {
			return line
		}
	}
	return ""
}

func TestProvidersRejectsArguments(t *testing.T) {
	if _, code := providerRows(t, nil, "extra"); code != 2 {
		t.Fatalf("code = %d", code)
	}
}

func TestConfiguredPresetsListsTheNewProviders(t *testing.T) {
	got := configuredPresets(getenvMap(map[string]string{"GEMINI_API_KEY": "k"}))
	if strings.Join(got, ",") != "gemini,ollama" {
		t.Fatalf("got %v", got)
	}
}
