package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const (
	defaultSearchLimit = 10
	maxSearchFiles     = 5000
	maxSearchPaths     = 20
	pathThreshold      = 0.3
	windowThreshold    = 0.5
	noMatches          = "no matches"
)

var searchSchema = json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"What the code or text does, in plain words."},"path":{"type":"string","description":"Directory or file to search; defaults to the working directory."},"limit":{"type":"integer","minimum":1,"description":"Maximum results; defaults to 10."}},"required":["query"],"additionalProperties":false}`)

type searchArgs struct {
	Query string `json:"query"`
	Path  string `json:"path"`
	Limit int    `json:"limit"`
}

func SearchTool(c *Client) tool.Tool {
	return tool.Tool{
		Name:        "search",
		Description: "Find code or text by meaning and list path:start-end ranges with scores, best first. File paths and contents are sent to the Jev service; secret-looking files are withheld.",
		Schema:      searchSchema,
		Run:         c.runSearch,
	}
}

func (c *Client) runSearch(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
	args, err := decodeSearchArgs(raw)
	if err != nil {
		return errorResult("%v", err), nil
	}
	hits, err := c.search(ctx, env, args)
	if err != nil {
		return searchFailure(ctx, args, err)
	}
	return formatHits(hits, args.Limit), nil
}

func decodeSearchArgs(raw json.RawMessage) (searchArgs, error) {
	var args searchArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return args, err
	}
	if strings.TrimSpace(args.Query) == "" {
		return args, errors.New("query is required")
	}
	if args.Limit <= 0 {
		args.Limit = defaultSearchLimit
	}
	return args, nil
}

func (c *Client) search(ctx context.Context, env tool.Env, args searchArgs) ([]hit, error) {
	paths, err := searchCandidates(ctx, env, searchRoot(env, args))
	if err != nil {
		return nil, err
	}
	kept, err := c.pathPass(ctx, env, args.Query, paths)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.windowPass(ctx, env, args.Query, kept)
}

func searchRoot(env tool.Env, args searchArgs) string {
	if args.Path == "" {
		return env.Dir
	}
	return tool.Resolve(env.Dir, args.Path)
}

func searchFailure(ctx context.Context, args searchArgs, err error) (tool.Result, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return tool.Result{}, ctxErr
	}
	if errors.Is(err, fs.ErrNotExist) {
		return errorResult("path not found: %s", args.Path), nil
	}
	return errorResult("search failed: %v", err), nil
}

func errorResult(format string, a ...any) tool.Result {
	return tool.Result{Text: fmt.Sprintf(format, a...), IsError: true}
}

func (c *Client) score(ctx context.Context, query string, instructions []string) ([]float64, error) {
	if len(instructions) == 0 {
		return nil, nil
	}
	qs := make(map[string]Question, len(instructions))
	for i, text := range instructions {
		qs[questionName(i)] = Noul(text)
	}
	answers, _, err := c.Ask(ctx, map[string]string{"query": query}, qs)
	if err != nil {
		return nil, err
	}
	scores := make([]float64, len(instructions))
	for i := range scores {
		scores[i] = answers[questionName(i)].Noul
	}
	return scores, nil
}

func questionName(i int) string {
	return fmt.Sprintf("q%d", i)
}
