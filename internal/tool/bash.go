package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

var bashTool = Tool{
	Name:        "bash",
	Description: "Run a command with bash -c in the working directory. Each call starts a fresh shell. Returns combined stdout and stderr and the exit code. Default timeout 120000 ms, max 600000.",
	Schema:      bashSchema,
	Run:         runBash,
}

var bashSchema = json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command passed to bash -c."},"timeout_ms":{"type":"integer","description":"Timeout in milliseconds; 0 uses 120000, capped at 600000."}},"required":["command"],"additionalProperties":false}`)

type bashArgs struct {
	Command   string `json:"command"`
	TimeoutMS int    `json:"timeout_ms"`
}

func runBash(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	args, err := decodeArgs[bashArgs](raw)
	if err != nil {
		return errorf("%v", err), nil
	}
	if args.Command == "" {
		return errorf("command is empty"), nil
	}
	timeout := bashTimeout(args.TimeoutMS)
	req := host.ExecRequest{Command: args.Command, Dir: env.Dir, Timeout: timeout}
	res, err := env.Host.Exec(ctx, req)
	if err != nil {
		return Result{}, err
	}
	return formatBash(res, timeout), nil
}

func bashTimeout(ms int) time.Duration {
	if ms <= 0 {
		return DefaultTimeout
	}
	d := time.Duration(ms) * time.Millisecond
	if d > MaxTimeout {
		return MaxTimeout
	}
	return d
}

func formatBash(res host.ExecResult, timeout time.Duration) Result {
	if res.TimedOut {
		return bashTimedOut(res, timeout)
	}
	return bashFinished(res)
}

func bashTimedOut(res host.ExecResult, timeout time.Duration) Result {
	text := fmt.Sprintf("command timed out after %s", timeout)
	if out := string(res.Output); out != "" {
		text += "\n" + truncateMiddle(out, MaxOutput)
	}
	return Result{Text: text, IsError: true}
}

func bashFinished(res host.ExecResult) Result {
	text := truncateMiddle(string(res.Output), MaxOutput)
	if text != "" {
		text += "\n"
	}
	text += fmt.Sprintf("exit code: %d", res.ExitCode)
	return Result{Text: text}
}
