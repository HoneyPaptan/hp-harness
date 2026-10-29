package openai

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func marshalRequest(req provider.Request) []byte {
	b, err := json.Marshal(buildRequest(req))
	if err != nil {
		panic(err)
	}
	return b
}

func jsonEqual(a, b []byte) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

func TestBuildRequest(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	tests := []struct {
		name string
		req  provider.Request
		want string
	}{
		{
			name: "system plus user",
			req: provider.Request{
				Model:  "gpt-4",
				System: "You are helpful.",
				Messages: []provider.Message{
					{Role: provider.RoleUser, Text: "hi"},
				},
			},
			want: `{"model":"gpt-4","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"hi"}],"stream":false}`,
		},
		{
			name: "assistant tool calls null content",
			req: provider.Request{
				Model: "m",
				Messages: []provider.Message{
					{
						Role: provider.RoleAssistant,
						ToolCalls: []provider.ToolCall{
							{ID: "c1", Name: "read", Args: json.RawMessage(`{"path":"a.go"}`)},
						},
					},
				},
			},
			want: `{"model":"m","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"read","arguments":"{\"path\":\"a.go\"}"}}]}],"stream":false}`,
		},
		{
			name: "tool results and error prefix",
			req: provider.Request{
				Model: "m",
				Messages: []provider.Message{
					{
						Role: provider.RoleUser,
						ToolResults: []provider.ToolResult{
							{CallID: "c1", Text: "ok"},
							{CallID: "c2", Text: "fail", IsError: true},
						},
						Text: "continue",
					},
				},
			},
			want: `{"model":"m","messages":[{"role":"tool","content":"ok","tool_call_id":"c1"},{"role":"tool","content":"error: fail","tool_call_id":"c2"},{"role":"user","content":"continue"}],"stream":false}`,
		},
		{
			name: "tools present",
			req: provider.Request{
				Model: "m",
				Tools: []provider.ToolDef{
					{Name: "read", Description: "read file", Schema: schema},
				},
			},
			want: `{"model":"m","messages":null,"tools":[{"type":"function","function":{"name":"read","description":"read file","parameters":{"type":"object"}}}],"stream":false}`,
		},
		{
			name: "no tools omitted",
			req: provider.Request{
				Model: "m",
			},
			want: `{"model":"m","messages":null,"stream":false}`,
		},
		{
			name: "max tokens omitted",
			req: provider.Request{
				Model:     "m",
				MaxTokens: 0,
			},
			want: `{"model":"m","messages":null,"stream":false}`,
		},
		{
			name: "max tokens set",
			req: provider.Request{
				Model:     "m",
				MaxTokens: 100,
			},
			want: `{"model":"m","messages":null,"stream":false,"max_tokens":100}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := marshalRequest(tt.req)
			if !jsonEqual(got, []byte(tt.want)) {
				t.Fatalf("got %s\nwant %s", got, tt.want)
			}
		})
	}
}
