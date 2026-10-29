package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

var editTool = Tool{
	Name:        "edit",
	Description: "Replace an exact string in a file. old_string must match exactly once unless replace_all is true; include surrounding lines to make it unique.",
	Schema:      editSchema,
	Run:         runEdit,
}

var editSchema = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path, relative to the working directory or absolute."},"old_string":{"type":"string","description":"Exact text to find in the file."},"new_string":{"type":"string","description":"Text to insert in place of old_string."},"replace_all":{"type":"boolean","description":"Replace every occurrence instead of requiring a single match."}},"required":["path","old_string","new_string"],"additionalProperties":false}`)

type editArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func runEdit(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	args, err := decodeArgs[editArgs](raw)
	if err != nil {
		return errorf("%v", err), nil
	}
	return editFile(env, args)
}

func editFile(env Env, args editArgs) (Result, error) {
	if res, ok := validateEdit(args); !ok {
		return res, nil
	}
	content, res, err := readTarget(env, args.Path)
	if err != nil {
		return Result{}, err
	}
	if res.IsError {
		return res, nil
	}
	return commitEdit(env, args, content)
}

func commitEdit(env Env, args editArgs, content string) (Result, error) {
	next, n, res, ok := applyEdit(content, args)
	if !ok {
		return res, nil
	}
	p := Resolve(env.Dir, args.Path)
	if err := env.Host.WriteFile(p, []byte(next)); err != nil {
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("replaced %d occurrence(s) in %s", n, args.Path)}, nil
}

func validateEdit(args editArgs) (Result, bool) {
	if args.OldString == "" {
		return errorf("old_string is empty"), false
	}
	if args.OldString == args.NewString {
		return errorf("old_string and new_string are the same"), false
	}
	return Result{}, true
}

func readTarget(env Env, path string) (string, Result, error) {
	p := Resolve(env.Dir, path)
	data, err := env.Host.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", errorf("file not found: %s", path), nil
		}
		return "", Result{}, err
	}
	return string(data), Result{}, nil
}

func applyEdit(content string, args editArgs) (string, int, Result, bool) {
	n := strings.Count(content, args.OldString)
	if n == 0 {
		return content, 0, errorf("old_string not found in %s", args.Path), false
	}
	if res, ok := checkEditCount(n, args); !ok {
		return content, 0, res, false
	}
	return replaceEdit(content, args, n)
}

func checkEditCount(n int, args editArgs) (Result, bool) {
	if n > 1 && !args.ReplaceAll {
		return errorf("%d matches for old_string in %s; add more context or set replace_all", n, args.Path), false
	}
	return Result{}, true
}

func replaceEdit(content string, args editArgs, n int) (string, int, Result, bool) {
	if args.ReplaceAll {
		return strings.ReplaceAll(content, args.OldString, args.NewString), n, Result{}, true
	}
	return strings.Replace(content, args.OldString, args.NewString, 1), 1, Result{}, true
}
