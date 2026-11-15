package openai

import (
	"encoding/json"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	unknownToolMarker = "which was not in request.tools"
	unknownToolPrefix = "attempted to call tool '"
)

func unknownTool(message string) *provider.UnknownToolError {
	if !strings.Contains(message, unknownToolMarker) {
		return nil
	}
	_, rest, _ := strings.Cut(message, unknownToolPrefix)
	name, _, _ := strings.Cut(rest, "'")
	return &provider.UnknownToolError{Name: name, Body: message}
}

func unknownToolFromBody(body string) *provider.UnknownToolError {
	var wrap struct {
		Error wireStreamError `json:"error"`
	}
	if json.Unmarshal([]byte(body), &wrap) != nil {
		return nil
	}
	return unknownTool(wrap.Error.Message)
}
