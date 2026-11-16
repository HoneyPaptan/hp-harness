package main

import (
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/cli"
	"github.com/HoneyPaptan/hp-harness/internal/provider/openai"
)

func TestNewProviderSendsTheUserAgent(t *testing.T) {
	for _, preset := range []string{"openrouter", "custom", "groq"} {
		t.Run(preset, func(t *testing.T) {
			c, ok := newProvider(cli.Endpoint{Preset: preset}).(*openai.Client)
			if !ok {
				t.Fatal("not the OpenAI adapter")
			}
			if want := "hp/" + version; c.UserAgent != want {
				t.Fatalf("UserAgent = %q, want %q", c.UserAgent, want)
			}
		})
	}
}
