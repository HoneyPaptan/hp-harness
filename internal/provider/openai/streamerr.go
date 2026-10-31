package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

var permanentKinds = map[string]int{
	"invalid_request_error":   400,
	"context_length_exceeded": 400,
	"invalid_api_key":         401,
	"authentication_error":    401,
	"insufficient_quota":      402,
	"permission_error":        403,
	"model_not_found":         404,
}

type wireStreamError struct {
	Message string
	Type    string
	Code    string
}

type wireErrorBody struct {
	Message string          `json:"message"`
	Type    string          `json:"type"`
	Code    json.RawMessage `json:"code"`
}

func (e *wireStreamError) UnmarshalJSON(b []byte) error {
	if json.Unmarshal(b, &e.Message) == nil {
		return nil
	}
	var body wireErrorBody
	if err := json.Unmarshal(b, &body); err != nil {
		return err
	}
	e.Message, e.Type = body.Message, body.Type
	e.Code = strings.Trim(string(body.Code), `"`)
	return nil
}

func (e *wireStreamError) asError() error {
	if status, ok := e.permanentStatus(); ok {
		return &provider.HTTPError{Status: status, Body: e.Message}
	}
	overloaded := e.hasKind("overloaded", "503", "529")
	if overloaded || e.hasKind("rate_limit_exceeded", "429") {
		return &provider.RateLimitError{Overloaded: overloaded, Body: e.Message}
	}
	return &provider.TransientError{Err: fmt.Errorf("stream error: %s", e.Message)}
}

func (e *wireStreamError) permanentStatus() (int, bool) {
	for _, kind := range []string{e.Type, e.Code} {
		if status, ok := permanentKinds[kind]; ok {
			return status, true
		}
	}
	status, err := strconv.Atoi(e.Code)
	return status, err == nil && permanentClientStatus(status)
}

func permanentClientStatus(status int) bool {
	if status < 400 || status > 499 {
		return false
	}
	return status != 408 && status != 409 && status != 429
}

func (e *wireStreamError) hasKind(kinds ...string) bool {
	for _, k := range kinds {
		if e.Type == k || e.Code == k {
			return true
		}
	}
	return false
}

func readFailure(ctx context.Context, err error) error {
	if errors.Is(err, bufio.ErrTooLong) {
		return err
	}
	return streamFailure(ctx, err)
}

func decodeFailure(ctx context.Context, data string, err error) error {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) && syntax.Offset >= int64(len(data)) {
		return streamFailure(ctx, err)
	}
	return err
}

func streamFailure(ctx context.Context, err error) error {
	if cause := ctx.Err(); cause != nil {
		return cause
	}
	return &provider.TransientError{Err: err}
}
