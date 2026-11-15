package openai

import (
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestEffortWireShape(t *testing.T) {
	cases := []struct {
		name   string
		client Client
		effort string
		want   string
		absent []string
	}{
		{name: "openrouter nests it", client: Client{NestedReasoning: true}, effort: "high", want: `"reasoning":{"effort":"high"}`, absent: []string{"reasoning_effort"}},
		{name: "openai style is top level", client: Client{}, effort: "low", want: `"reasoning_effort":"low"`, absent: []string{`"reasoning":`}},
		{name: "empty sends nothing", client: Client{NestedReasoning: true}, absent: []string{"reasoning"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := tc.client.encodeStreamRequest(provider.Request{Model: "m", Effort: tc.effort})
			if err != nil {
				t.Fatal(err)
			}
			body := string(b)
			if tc.want != "" && !strings.Contains(body, tc.want) {
				t.Fatalf("missing %s in %s", tc.want, body)
			}
			for _, a := range tc.absent {
				if strings.Contains(body, a) {
					t.Fatalf("unexpected %s in %s", a, body)
				}
			}
		})
	}
}

func TestHeadersSentOnStreamRequest(t *testing.T) {
	c := Client{BaseURL: "https://x", APIKey: "k", Headers: OpenRouterApp}
	req, err := c.newStreamHTTPRequest(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-OpenRouter-Title") != "hp" || req.Header.Get("HTTP-Referer") == "" || req.Header.Get("Authorization") != "Bearer k" {
		t.Fatalf("headers %v", req.Header)
	}
}
