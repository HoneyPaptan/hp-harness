package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

var writeTool = Tool{
	Name:        "write",
	Description: "Create or overwrite a file with the given content. Parent directories are created.",
	Schema:      writeSchema,
	Run:         runWrite,
}

var writeSchema = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path, relative to the working directory or absolute."},"content":{"type":"string","description":"Full file contents to write."}},"required":["path","content"],"additionalProperties":false}`)

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func runWrite(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	args, err := decodeArgs[writeArgs](raw)
	if err != nil {
		return errorf("%v", err), nil
	}
	return writeFile(env, args)
}

func writeFile(env Env, args writeArgs) (Result, error) {
	p := Resolve(env.Dir, args.Path)
	if res, err := guardWrite(env, p, args.Path); res.IsError || err != nil {
		return res, err
	}
	data := []byte(args.Content)
	if err := env.Host.WriteFile(p, data); err != nil {
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("wrote %d bytes to %s", len(data), args.Path)}, nil
}

func guardWrite(env Env, p, argPath string) (Result, error) {
	ent, err := env.Host.Stat(p)
	if err == nil && ent.IsDir {
		return errorf("path is a directory: %s", argPath), nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Result{}, err
	}
	return Result{}, nil
}
