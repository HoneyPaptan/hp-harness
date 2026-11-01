package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const emptyOutput = "(no output)"

var (
	defaultSchema = json.RawMessage(`{"type":"object","properties":{}}`)
	emptyArgs     = json.RawMessage(`{}`)
)

type listResult struct {
	Tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	} `json:"tools"`
	NextCursor string `json:"nextCursor"`
}

func (r listResult) tools() []Tool {
	out := make([]Tool, 0, len(r.Tools))
	for _, t := range r.Tools {
		if t.Name == "" {
			continue
		}
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = defaultSchema
		}
		out = append(out, Tool{Name: t.Name, Description: t.Description, Schema: schema})
	}
	return out
}

type callResult struct {
	Content    []contentBlock  `json:"content"`
	IsError    bool            `json:"isError"`
	Structured json.RawMessage `json:"structuredContent"`
}

type contentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Data     string          `json:"data"`
	MimeType string          `json:"mimeType"`
	URI      string          `json:"uri"`
	Resource *embeddedSource `json:"resource"`
}

type embeddedSource struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
	Blob     string `json:"blob"`
}

func callParams(name string, args json.RawMessage) (map[string]any, error) {
	args = bytes.TrimSpace(args)
	if len(args) == 0 || string(args) == "null" {
		args = emptyArgs
	}
	if !json.Valid(args) {
		return nil, errors.New("tool arguments are not valid JSON")
	}
	return map[string]any{"name": name, "arguments": args}, nil
}

func decodeCall(raw json.RawMessage) (tool.Result, error) {
	var res callResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return tool.Result{}, err
	}
	return tool.Result{Text: tool.Truncate(res.text()), IsError: res.IsError}, nil
}

func (r callResult) text() string {
	parts := make([]string, 0, len(r.Content))
	for _, b := range r.Content {
		parts = append(parts, b.summary())
	}
	if text := strings.Join(parts, "\n"); text != "" {
		return text
	}
	if len(r.Structured) > 0 && string(r.Structured) != "null" {
		return string(r.Structured)
	}
	return emptyOutput
}

func (b contentBlock) summary() string {
	switch b.Type {
	case "text":
		return b.Text
	case "image", "audio":
		return fmt.Sprintf("[%s %s, %d bytes]", b.Type, b.MimeType, encodedSize(b.Data))
	case "resource_link":
		return fmt.Sprintf("[resource link %s]", b.URI)
	case "resource":
		return b.resourceSummary()
	default:
		return fmt.Sprintf("[%s content]", b.Type)
	}
}

func (b contentBlock) resourceSummary() string {
	r := b.Resource
	if r == nil {
		return "[resource]"
	}
	if r.Text != "" {
		return r.Text
	}
	return fmt.Sprintf("[resource %s %s, %d bytes]", r.URI, r.MimeType, encodedSize(r.Blob))
}

func encodedSize(data string) int {
	return len(data) * 3 / 4
}
