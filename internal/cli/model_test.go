package cli

import (
	"errors"
	"strings"
	"testing"
)

func getenvMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveEndpoint(t *testing.T) {
	env := getenvMap(map[string]string{
		"XAI_API_KEY": "k", "OPENCODE_API_KEY": "o", "OPENROUTER_API_KEY": "r",
	})
	cases := []struct {
		name    string
		model   string
		base    string
		keyEnv  string
		getenv  func(string) string
		want    Endpoint
		wantErr string
	}{
		{
			name: "xai preset", model: "xai/grok-4.6", getenv: env,
			want: Endpoint{Preset: "xai", Model: "grok-4.6", BaseURL: "https://api.x.ai/v1", KeyEnv: "XAI_API_KEY", APIKey: "k"},
		},
		{
			name: "openrouter slash", model: "openrouter/anthropic/claude-x", getenv: env,
			want: Endpoint{Preset: "openrouter", Model: "anthropic/claude-x", BaseURL: "https://openrouter.ai/api/v1", KeyEnv: "OPENROUTER_API_KEY", APIKey: "r"},
		},
		{
			name: "ollama no key", model: "ollama/llama3", getenv: env,
			want: Endpoint{Preset: "ollama", Model: "llama3", BaseURL: "http://127.0.0.1:11434/v1"},
		},
		{
			name: "hp model fallback", model: "", getenv: getenvMap(map[string]string{"HP_MODEL": "xai/grok", "XAI_API_KEY": "k"}),
			want: Endpoint{Preset: "xai", Model: "grok", BaseURL: "https://api.x.ai/v1", KeyEnv: "XAI_API_KEY", APIKey: "k"},
		},
		{
			name: "no model", model: "", getenv: getenvMap(nil), wantErr: "model required",
		},
		{
			name: "missing key", model: "xai/grok", getenv: getenvMap(nil), wantErr: "XAI_API_KEY",
		},
		{
			name: "unknown preset", model: "foo/bar", getenv: env, wantErr: "unknown preset",
		},
		{
			name: "custom base", model: "foo/bar", base: "https://ex.example/v1", getenv: env,
			want: Endpoint{Preset: "custom", Model: "foo/bar", BaseURL: "https://ex.example/v1"},
		},
		{
			name: "override base", model: "xai/grok", base: "https://custom/v1", getenv: env,
			want: Endpoint{Preset: "xai", Model: "grok", BaseURL: "https://custom/v1", KeyEnv: "XAI_API_KEY", APIKey: "k"},
		},
		{
			name: "override key env", model: "xai/grok", keyEnv: "MY_KEY", getenv: getenvMap(map[string]string{"MY_KEY": "x"}),
			want: Endpoint{Preset: "xai", Model: "grok", BaseURL: "https://api.x.ai/v1", KeyEnv: "MY_KEY", APIKey: "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveEndpoint(tc.model, tc.base, tc.keyEnv, tc.getenv)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				var ue usageError
				if !errors.As(err, &ue) {
					t.Fatalf("want usageError, got %T", err)
				}
				if !strings.Contains(ue.msg, tc.wantErr) {
					t.Fatalf("err %q want substring %q", ue.msg, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
