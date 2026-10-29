package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

var readTool = Tool{
	Name:        "read",
	Description: "Read a text file. Returns lines prefixed with 1-based line numbers. Use offset and limit to page through long files.",
	Schema:      readSchema,
	Run:         runRead,
}

var readSchema = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path, relative to the working directory or absolute."},"offset":{"type":"integer","description":"1-based line number to start reading from."},"limit":{"type":"integer","description":"Maximum number of lines to return."}},"required":["path"],"additionalProperties":false}`)

type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func runRead(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	args, err := decodeArgs[readArgs](raw)
	if err != nil {
		return errorf("%v", err), nil
	}
	if args.Offset == 0 {
		args.Offset = 1
	}
	if args.Limit == 0 {
		args.Limit = DefaultReadLimit
	}
	return readFile(env, args)
}

func readFile(env Env, args readArgs) (Result, error) {
	p := Resolve(env.Dir, args.Path)
	ent, err := env.Host.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errorf("file not found: %s", args.Path), nil
		}
		return Result{}, err
	}
	if ent.IsDir {
		return errorf("path is a directory: %s", args.Path), nil
	}
	data, err := env.Host.ReadFile(p)
	if err != nil {
		return Result{}, err
	}
	return formatRead(args.Path, data, args.Offset, args.Limit), nil
}

func formatRead(path string, data []byte, offset, limit int) Result {
	lines := splitLines(data)
	start := offset - 1
	if start >= len(lines) {
		return errorf("offset past end of file: %s", path)
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%6d\t%s", i+1, lines[i])
	}
	return Result{Text: truncateHead(b.String(), MaxOutput)}
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	s := strings.TrimSuffix(string(data), "\n")
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}
