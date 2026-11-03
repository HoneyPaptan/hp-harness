package openai

import (
	"encoding/json"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func contextOverflow(code, typ, message string) bool {
	if code == "context_length_exceeded" || typ == "context_length_exceeded" {
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(lower, "context length") ||
		strings.Contains(lower, "context window") ||
		strings.Contains(lower, "reduce the length of the messages")
}

func contextOverflowFromBody(body string) bool {
	var wrap struct {
		Error struct {
			Message string          `json:"message"`
			Type    string          `json:"type"`
			Code    json.RawMessage `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &wrap) != nil {
		return contextOverflow("", "", body)
	}
	code := strings.Trim(string(wrap.Error.Code), `"`)
	return contextOverflow(code, wrap.Error.Type, wrap.Error.Message)
}

func httpError(status int, body string) *provider.HTTPError {
	return &provider.HTTPError{Status: status, Body: body, Overflow: contextOverflowFromBody(body)}
}
