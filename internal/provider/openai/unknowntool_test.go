package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const groqUnknownTool = `{"error":{"message":"Tool call validation failed: attempted to call tool 'ls' which was not in request.tools","type":"invalid_request_error","code":"tool_use_failed","failed_generation":"{\"name\": \"ls\", \"arguments\": {\"path\": \".\"}}"}}`

const groqBadSchema = `{"error":{"message":"Failed to call a function. Please adjust your prompt. See 'failed_generation' for more details.","type":"invalid_request_error","code":"tool_use_failed","failed_generation":"Tool use failed: JSON does not match the expected schema for tool calls"}}`

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestUnknownToolHTTPBody(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantName string
	}{
		{name: "groq body", status: 400, body: groqUnknownTool, wantName: "ls"},
		{name: "other tool name", status: 400, body: strings.Replace(groqUnknownTool, "'ls'", "'open_file'", 1), wantName: "open_file"},
		{name: "schema mismatch is not an unknown tool", status: 400, body: groqBadSchema},
		{name: "plain bad request", status: 400, body: `{"error":{"message":"messages is required","type":"invalid_request_error"}}`},
		{name: "not json", status: 400, body: "nope"},
		{name: "other status", status: 422, body: groqUnknownTool},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := errorFromResponse(response(tc.status, tc.body))
			var ut *provider.UnknownToolError
			if tc.wantName == "" {
				var he *provider.HTTPError
				if errors.As(err, &ut) || !errors.As(err, &he) {
					t.Fatalf("err = %#v", err)
				}
				return
			}
			if !errors.As(err, &ut) || ut.Name != tc.wantName || !strings.Contains(ut.Body, "not in request.tools") {
				t.Fatalf("err = %#v", err)
			}
		})
	}
}

func TestUnknownToolStreamFrame(t *testing.T) {
	frame := `{"error":{"message":"Tool call validation failed: attempted to call tool 'ls' which was not in request.tools","type":"invalid_request_error","code":"tool_use_failed","failed_generation":"{\"name\": \"ls\"}","status_code":400}}`
	err := readAll(context.Background(), strings.NewReader(sseBody(chunkText("par"), frame)))
	var ut *provider.UnknownToolError
	var he *provider.HTTPError
	if !errors.As(err, &ut) || ut.Name != "ls" || errors.As(err, &he) {
		t.Fatalf("err = %#v", err)
	}
	err = readAll(context.Background(), strings.NewReader(sseBody(chunkText("par"), groqBadSchema)))
	if errors.As(err, &ut) || !errors.As(err, &he) || he.Status != 400 {
		t.Fatalf("schema mismatch err = %#v", err)
	}
}
