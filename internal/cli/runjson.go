package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/core"
)

const stdinPromptLimit = 8 << 20

const (
	jsonEndFinal       = "final"
	jsonEndStepLimit   = "step_limit"
	jsonEndError       = "error"
	jsonEndInterrupted = "interrupted"
	jsonEndLimitWait   = "limit_wait_too_long"
)

type jsonRun struct {
	Type  string `json:"type"`
	RunID string `json:"run_id"`
	Model string `json:"model"`
	Dir   string `json:"dir"`
}

type jsonText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type jsonToolCall struct {
	Type string          `json:"type"`
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type jsonToolResult struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	IsError bool   `json:"is_error"`
	Text    string `json:"text"`
}

type jsonUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type jsonEnd struct {
	Type     string    `json:"type"`
	RunID    string    `json:"run_id"`
	Reason   string    `json:"reason"`
	ExitCode int       `json:"exit_code"`
	Text     string    `json:"text"`
	Steps    int       `json:"steps"`
	Usage    jsonUsage `json:"usage"`
	Error    string    `json:"error,omitempty"`
}

func (p *printer) emit(event any) {
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = p.env.Stdout.Write(append(line, '\n'))
}

func (p *printer) jsonStart(ep Endpoint, dir string) {
	p.emit(jsonRun{Type: "run", RunID: p.id, Model: endpointSpec(ep), Dir: dir})
}

func (p *printer) jsonEndOK(out core.Outcome, code int) {
	reason := jsonEndFinal
	if out.Reason == core.ReasonStepLimit {
		reason = jsonEndStepLimit
	}
	p.emit(jsonEnd{
		Type: "end", RunID: p.id, Reason: reason, ExitCode: code, Text: out.Text, Steps: out.Steps,
		Usage: jsonUsage{InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens},
	})
}

func (p *printer) jsonEndFailed(reason string, code int, err error) {
	p.emit(jsonEnd{Type: "end", RunID: p.id, Reason: reason, ExitCode: code, Error: err.Error()})
}

func validArgs(raw json.RawMessage) json.RawMessage {
	if json.Valid(raw) {
		return raw
	}
	quoted, _ := json.Marshal(string(raw))
	return quoted
}

func readPrompt(env Env, rest []string) (string, error) {
	if len(rest) > 1 || len(rest) == 1 && rest[0] != "-" {
		return strings.Join(rest, " "), nil
	}
	if env.Stdin == nil || env.StdinIsTerminal != nil && env.StdinIsTerminal() {
		return "", usagef("prompt required: pass it as an argument or pipe it in")
	}
	data, err := io.ReadAll(io.LimitReader(env.Stdin, stdinPromptLimit+1))
	if err != nil {
		return "", errors.New("cannot read the prompt from stdin: " + err.Error())
	}
	if len(data) > stdinPromptLimit {
		return "", usagef("prompt on stdin is over %d MiB", stdinPromptLimit>>20)
	}
	return string(bytes.TrimSpace(data)), nil
}
