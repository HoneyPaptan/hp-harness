package jev

import (
	"context"
	"fmt"
	"sort"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

type scoredPath struct {
	path  string
	score float64
}

func searchCandidates(ctx context.Context, env tool.Env, root string) ([]string, error) {
	named, err := walkNamed(env, root)
	if err != nil {
		return nil, err
	}
	if reachedThroughLink(env, root) {
		return nil, nil
	}
	return keepShareable(ctx, env, named)
}

func walkNamed(env tool.Env, root string) ([]string, error) {
	var out []string
	err := tool.Walk(env, root, func(p string, e host.Entry) {
		if len(out) < maxSearchFiles && !e.IsLink && !isSecretName(p) {
			out = append(out, p)
		}
	})
	return out, err
}

func keepShareable(ctx context.Context, env tool.Env, paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, ok := readShareable(env, p); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (c *Client) pathPass(ctx context.Context, env tool.Env, query string, paths []string) ([]string, error) {
	instructions := make([]string, len(paths))
	for i, p := range paths {
		instructions[i] = pathInstructions(query, tool.DisplayPath(env.Dir, p))
	}
	scores, err := c.score(ctx, query, instructions)
	if err != nil {
		return nil, err
	}
	return topPaths(paths, scores), nil
}

func pathInstructions(query, display string) string {
	return fmt.Sprintf("Could the file at this path contain code or text that %s?\nPath: %s", query, display)
}

func topPaths(paths []string, scores []float64) []string {
	var likely []scoredPath
	for i, s := range scores {
		if s >= pathThreshold {
			likely = append(likely, scoredPath{path: paths[i], score: s})
		}
	}
	sort.Slice(likely, func(i, j int) bool {
		if likely[i].score != likely[j].score {
			return likely[i].score > likely[j].score
		}
		return likely[i].path < likely[j].path
	})
	if len(likely) > maxSearchPaths {
		likely = likely[:maxSearchPaths]
	}
	out := make([]string, len(likely))
	for i, l := range likely {
		out[i] = l.path
	}
	return out
}
