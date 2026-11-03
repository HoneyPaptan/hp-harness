package anthropic

import (
	"encoding/json"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func contextOverflow(typ, message string) bool {
	if typ == "request_too_large" {
		return true
	}
	return strings.Contains(strings.ToLower(message), "prompt is too long")
}

func contextOverflowFromBody(body string) bool {
	var wrap struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &wrap) != nil {
		return contextOverflow("", body)
	}
	return contextOverflow(wrap.Error.Type, wrap.Error.Message)
}

func httpError(status int, body string) *provider.HTTPError {
	return &provider.HTTPError{Status: status, Body: body, Overflow: contextOverflowFromBody(body)}
}
