package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var grepTool = Tool{
	Name:        "grep",
	Description: "Search file contents with a Go regular expression under a directory or in one file. Skips .git and binary files. Optional glob filters by file name, like *.go.",
	Schema:      grepSchema,
	Run:         runGrep,
}

var grepSchema = json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Go regular expression to search for in each line."},"path":{"type":"string","description":"Directory or file to search; defaults to the working directory."},"glob":{"type":"string","description":"Optional file name glob, for example *.go."}},"required":["pattern"],"additionalProperties":false}`)

type grepArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Glob    string `json:"glob"`
}

type grepMatch struct {
	path string
	line int
	text string
}

func runGrep(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	args, err := decodeArgs[grepArgs](raw)
	if err != nil {
		return errorf("%v", err), nil
	}
	return grepFiles(ctx, env, args)
}

func grepFiles(ctx context.Context, env Env, args grepArgs) (Result, error) {
	re, res, ok := compileGrep(args)
	if !ok {
		return res, nil
	}
	paths, err := listGrepPaths(env, grepRoot(env, args))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errorf("path not found: %s", args.Path), nil
		}
		return Result{}, err
	}
	matches, err := collectGrepMatches(ctx, env, re, filterGlob(paths, args.Glob))
	if err != nil {
		return Result{}, err
	}
	return formatGrep(env.Dir, matches), nil
}

func grepRoot(env Env, args grepArgs) string {
	if args.Path == "" {
		return env.Dir
	}
	return Resolve(env.Dir, args.Path)
}

func collectGrepMatches(ctx context.Context, env Env, re *regexp.Regexp, paths []string) ([]grepMatch, error) {
	var matches []grepMatch
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m, err := matchFile(env, re, p)
		if err != nil {
			return nil, err
		}
		matches = append(matches, m...)
	}
	return matches, nil
}

func compileGrep(args grepArgs) (*regexp.Regexp, Result, bool) {
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return nil, errorf("invalid regexp: %v", err), false
	}
	if args.Glob != "" {
		if _, err := filepath.Match(args.Glob, "x"); err != nil {
			return nil, errorf("invalid glob: %v", err), false
		}
	}
	return re, Result{}, true
}

func filterGlob(paths []string, glob string) []string {
	if glob == "" {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		ok, _ := filepath.Match(glob, filepath.Base(p))
		if ok {
			out = append(out, p)
		}
	}
	return out
}

func listGrepPaths(env Env, root string) ([]string, error) {
	ent, err := env.Host.Stat(root)
	if err != nil {
		return nil, err
	}
	if !ent.IsDir {
		return []string{root}, nil
	}
	var out []string
	if err := walkDir(env, root, func(p string) { out = append(out, p) }); err != nil {
		return nil, err
	}
	return out, nil
}

func walkDir(env Env, dir string, file func(string)) error {
	ents, err := env.Host.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		p := filepath.Join(dir, e.Name)
		if e.IsDir {
			if e.Name == ".git" {
				continue
			}
			if err := walkDir(env, p, file); err != nil {
				return err
			}
			continue
		}
		file(p)
	}
	return nil
}

func matchFile(env Env, re *regexp.Regexp, path string) ([]grepMatch, error) {
	data, ok, err := readGrepFile(env, path)
	if err != nil || !ok {
		return nil, err
	}
	return scanLines(path, string(data), re), nil
}

func readGrepFile(env Env, path string) ([]byte, bool, error) {
	ent, err := env.Host.Stat(path)
	if err != nil {
		return nil, false, err
	}
	if ent.Size > 1<<20 {
		return nil, false, nil
	}
	data, err := env.Host.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	if isBinary(data) {
		return nil, false, nil
	}
	return data, true, nil
}

func scanLines(path, text string, re *regexp.Regexp) []grepMatch {
	lines := strings.Split(text, "\n")
	var out []grepMatch
	for i, line := range lines {
		if re.FindStringIndex(line) == nil {
			continue
		}
		out = append(out, grepMatch{path: path, line: i + 1, text: line})
	}
	return out
}

func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return strings.IndexByte(string(data[:n]), 0) >= 0
}

func formatGrep(workDir string, matches []grepMatch) Result {
	if len(matches) == 0 {
		return Result{Text: "no matches"}
	}
	sortMatches(matches)
	capped := len(matches) > MaxMatches
	if capped {
		matches = matches[:MaxMatches]
	}
	return Result{Text: writeMatches(workDir, matches, capped)}
}

func sortMatches(matches []grepMatch) {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].path != matches[j].path {
			return matches[i].path < matches[j].path
		}
		return matches[i].line < matches[j].line
	})
}

func writeMatches(workDir string, matches []grepMatch, capped bool) string {
	var b strings.Builder
	for i, m := range matches {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(displayPath(workDir, m.path))
		b.WriteByte(':')
		b.WriteString(fmt.Sprintf("%d: %s", m.line, m.text))
	}
	if capped {
		b.WriteString("\n[capped at 200 matches]")
	}
	return b.String()
}

func displayPath(workDir, p string) string {
	rel, err := filepath.Rel(workDir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}
