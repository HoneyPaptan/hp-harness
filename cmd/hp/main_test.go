package main

import (
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/cli"
	"github.com/HoneyPaptan/hp-harness/internal/provider/anthropic"
	"github.com/HoneyPaptan/hp-harness/internal/provider/openai"
)

func TestNewProviderPicksTheAdapter(t *testing.T) {
	if _, ok := newProvider(cli.Endpoint{Preset: "anthropic"}).(*anthropic.Client); !ok {
		t.Fatal("anthropic preset did not get the Messages adapter")
	}
	cases := []struct {
		preset          string
		disableThinking bool
		noStreamOptions bool
	}{
		{preset: "openai"},
		{preset: "custom"},
		{preset: "deepseek", disableThinking: true},
		{preset: "mistral", noStreamOptions: true},
	}
	for _, tc := range cases {
		t.Run(tc.preset, func(t *testing.T) {
			c, ok := newProvider(cli.Endpoint{Preset: tc.preset}).(*openai.Client)
			if !ok {
				t.Fatal("not the OpenAI adapter")
			}
			if c.DisableThinking != tc.disableThinking || c.NoStreamOptions != tc.noStreamOptions {
				t.Fatalf("client = %+v", c)
			}
		})
	}
}
