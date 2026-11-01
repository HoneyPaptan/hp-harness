package anthropic

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	defaultMaxTokens = 16000
	emptySchema      = `{"type":"object"}`
	emptyInput       = "{}"
)

var (
	ephemeral  = &wireCache{Type: "ephemeral"}
	idForbids  = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	objectHead = []byte("{")
)

func buildRequest(req provider.Request, fallbackMax int) wireRequest {
	return wireRequest{
		Model: req.Model, MaxTokens: maxTokens(req.MaxTokens, fallbackMax),
		System: systemBlocks(req.System), Messages: mapMessages(req.Messages),
		Tools: mapTools(req.Tools), Stream: true,
	}
}

func maxTokens(requested, fallback int) int {
	if requested > 0 {
		return requested
	}
	if fallback > 0 {
		return fallback
	}
	return defaultMaxTokens
}

func systemBlocks(text string) []wireBlock {
	if text == "" {
		return nil
	}
	return []wireBlock{{Type: "text", Text: text, CacheControl: ephemeral}}
}

func mapMessages(msgs []provider.Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		if blocks := messageBlocks(m); len(blocks) > 0 {
			out = appendTurn(out, string(m.Role), blocks)
		}
	}
	markLastUser(out)
	return out
}

func messageBlocks(m provider.Message) []wireBlock {
	if m.Role == provider.RoleUser {
		return userBlocks(m)
	}
	if m.Role == provider.RoleAssistant {
		return assistantBlocks(m)
	}
	return nil
}

func userBlocks(m provider.Message) []wireBlock {
	out := make([]wireBlock, 0, len(m.ToolResults)+1)
	for _, tr := range m.ToolResults {
		out = append(out, wireBlock{
			Type: "tool_result", ToolUseID: wireID(tr.CallID),
			Content: tr.Text, IsError: tr.IsError,
		})
	}
	return appendText(out, m.Text)
}

func assistantBlocks(m provider.Message) []wireBlock {
	out := appendText(make([]wireBlock, 0, len(m.ToolCalls)+1), m.Text)
	for _, c := range m.ToolCalls {
		out = append(out, wireBlock{
			Type: "tool_use", ID: wireID(c.ID), Name: c.Name, Input: toolInput(c.Args),
		})
	}
	return out
}

func appendText(blocks []wireBlock, text string) []wireBlock {
	if text == "" {
		return blocks
	}
	return append(blocks, wireBlock{Type: "text", Text: text})
}

func wireID(id string) string {
	return idForbids.ReplaceAllString(id, "_")
}

func toolInput(args json.RawMessage) json.RawMessage {
	if json.Valid(args) && bytes.HasPrefix(bytes.TrimSpace(args), objectHead) {
		return args
	}
	return json.RawMessage(emptyInput)
}

func appendTurn(out []wireMessage, role string, blocks []wireBlock) []wireMessage {
	if n := len(out); n > 0 && out[n-1].Role == role {
		out[n-1].Content = resultsFirst(append(out[n-1].Content, blocks...))
		return out
	}
	return append(out, wireMessage{Role: role, Content: blocks})
}

func resultsFirst(blocks []wireBlock) []wireBlock {
	slices.SortStableFunc(blocks, func(a, b wireBlock) int {
		return resultRank(a) - resultRank(b)
	})
	return blocks
}

func resultRank(b wireBlock) int {
	if b.Type == "tool_result" {
		return 0
	}
	return 1
}

func markLastUser(msgs []wireMessage) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == string(provider.RoleUser) {
			blocks := msgs[i].Content
			blocks[len(blocks)-1].CacheControl = ephemeral
			return
		}
	}
}

func mapTools(tools []provider.ToolDef) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, len(tools))
	for i, t := range tools {
		out[i] = wireTool{Name: t.Name, Description: t.Description, InputSchema: toolSchema(t.Schema)}
	}
	out[len(out)-1].CacheControl = ephemeral
	return out
}

func toolSchema(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 {
		return json.RawMessage(emptySchema)
	}
	return schema
}
