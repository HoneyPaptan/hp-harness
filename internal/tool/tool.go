package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

const (
	MaxOutput        = 30_000
	MaxMatches       = 200
	DefaultTimeout   = 120 * time.Second
	MaxTimeout       = 10 * time.Minute
	DefaultReadLimit = 2000
)

type Env struct {
	Host host.Host
	Dir  string
}

type Result struct {
	Text    string
	IsError bool
}

type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Deferred    bool
	Run         func(ctx context.Context, env Env, args json.RawMessage) (Result, error)
}

func Builtins() []Tool {
	return []Tool{readTool, writeTool, editTool, bashTool, grepTool}
}

func errorf(format string, a ...any) Result {
	return Result{Text: fmt.Sprintf(format, a...), IsError: true}
}
