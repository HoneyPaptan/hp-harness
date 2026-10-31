package jev

import (
	"context"
	"fmt"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const (
	maxWindowBytes = 32 << 10
	windowLines    = 60
	windowOverlap  = 10
)

type window struct {
	path  string
	start int
	end   int
	text  string
}

func (c *Client) windowPass(ctx context.Context, env tool.Env, query string, paths []string) ([]hit, error) {
	windows, err := collectWindows(ctx, env, paths)
	if err != nil {
		return nil, err
	}
	instructions := make([]string, len(windows))
	for i, w := range windows {
		instructions[i] = windowInstructions(w)
	}
	scores, err := c.score(ctx, query, instructions)
	if err != nil {
		return nil, err
	}
	return keepWindows(windows, scores), nil
}

func collectWindows(ctx context.Context, env tool.Env, paths []string) ([]window, error) {
	var out []window
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text, ok := readShareable(env, p)
		if ok {
			out = append(out, splitWindows(tool.DisplayPath(env.Dir, p), text)...)
		}
	}
	return out, nil
}

func splitWindows(path, text string) []window {
	lines := fileLines(text)
	var out []window
	for start := 0; start < len(lines); start += windowLines - windowOverlap {
		end := min(start+windowLines, len(lines))
		body := strings.Join(lines[start:end], "\n")
		if strings.TrimSpace(body) != "" {
			out = append(out, window{path: path, start: start + 1, end: end, text: body})
		}
		if end == len(lines) {
			break
		}
	}
	return out
}

func fileLines(text string) []string {
	if len(text) > maxWindowBytes {
		text = text[:maxWindowBytes]
	}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func windowInstructions(w window) string {
	return fmt.Sprintf("Does this excerpt of %s, lines %d-%d, contain code or text that the query in the state describes?\n\n%s", w.path, w.start, w.end, w.text)
}

func keepWindows(windows []window, scores []float64) []hit {
	var out []hit
	for i, s := range scores {
		if s >= windowThreshold {
			w := windows[i]
			out = append(out, hit{path: w.path, start: w.start, end: w.end, score: s})
		}
	}
	return out
}
