package tool

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

const MaxListed = 500

var listTool = Tool{
	Name:        "list",
	Description: "List the files under a directory, one relative path per line, sorted. Skips .git. Use it to see what exists before reading.",
	Schema:      listSchema,
	Run:         runList,
}

var listSchema = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Directory to list; defaults to the working directory."}},"additionalProperties":false}`)

type listArgs struct {
	Path string `json:"path"`
}

func BrowserBuiltins() []Tool {
	return []Tool{readTool, writeTool, editTool, grepTool, listTool}
}

func runList(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	args, err := decodeArgs[listArgs](raw)
	if err != nil {
		return errorf("%v", err), nil
	}
	root := env.Dir
	if args.Path != "" {
		root = Resolve(env.Dir, args.Path)
	}
	var paths []string
	err = Walk(env, root, func(p string, _ host.Entry) { paths = append(paths, relativeTo(root, p)) })
	if errors.Is(err, fs.ErrNotExist) {
		return errorf("path not found: %s", args.Path), nil
	}
	if err != nil {
		return Result{}, err
	}
	return formatList(paths), nil
}

func relativeTo(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		return rel
	}
	return p
}

func formatList(paths []string) Result {
	if len(paths) == 0 {
		return Result{Text: "(empty)"}
	}
	capped := len(paths) > MaxListed
	if capped {
		paths = paths[:MaxListed]
	}
	text := strings.Join(paths, "\n")
	if capped {
		text += "\n(more files not shown; list a subdirectory)"
	}
	return Result{Text: text}
}
