package tool

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	MaxDirect    = 8
	SearchName   = "tools"
	maxLoaded    = 5
	maxListed    = 20
	maxSummary   = 200
	loadedPrefix = "loaded: "
)

var searchSchema = json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Words to match against tool names and descriptions, or exact tool names."}},"required":["query"],"additionalProperties":false}`)

type searchArgs struct {
	Query string `json:"query"`
}

func Compose(direct, connected []Tool) []Tool {
	if len(direct)+len(connected) <= MaxDirect {
		return slices.Concat(direct, connected)
	}
	deferred := make([]Tool, len(connected))
	for i, t := range connected {
		t.Deferred = true
		deferred[i] = t
	}
	return slices.Concat(direct, []Tool{searchTool(connected)}, deferred)
}

func searchTool(connected []Tool) Tool {
	return Tool{
		Name:        SearchName,
		Description: fmt.Sprintf("Find tools from connected servers. %d more tools are available. Search by words or exact names; the best %d matches become callable on your next turn.", len(connected), maxLoaded),
		Schema:      searchSchema,
		Run: func(_ context.Context, _ Env, raw json.RawMessage) (Result, error) {
			args, err := decodeArgs[searchArgs](raw)
			if err != nil {
				return errorf("%v", err), nil
			}
			return runSearch(connected, args.Query), nil
		},
	}
}

func runSearch(connected []Tool, query string) Result {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return errorf("query is empty")
	}
	found := rankTools(connected, words)
	if len(found) == 0 {
		return Result{Text: fmt.Sprintf("no tools match %q", query)}
	}
	return Result{Text: formatFound(found)}
}

func rankTools(connected []Tool, words []string) []Tool {
	type scored struct {
		tool  Tool
		score int
	}
	var hits []scored
	for _, t := range connected {
		if s := matchScore(t, words); s > 0 {
			hits = append(hits, scored{t, s})
		}
	}
	slices.SortStableFunc(hits, func(a, b scored) int { return cmp.Compare(b.score, a.score) })
	out := make([]Tool, len(hits))
	for i, h := range hits {
		out[i] = h.tool
	}
	return out
}

func matchScore(t Tool, words []string) int {
	name, desc := strings.ToLower(t.Name), strings.ToLower(t.Description)
	score := 0
	for _, w := range words {
		switch {
		case w == name:
			score += 100
		case strings.Contains(name, w):
			score += 3
		case strings.Contains(desc, w):
			score++
		}
	}
	return score
}

func formatFound(found []Tool) string {
	shown := found[:min(len(found), maxLoaded)]
	names := make([]string, len(shown))
	var b strings.Builder
	for i, t := range shown {
		names[i] = t.Name
		fmt.Fprintf(&b, "%s: %s\n", t.Name, oneLineSummary(t.Description))
	}
	text := loadedPrefix + strings.Join(names, ", ") + "\n" + b.String()
	if more := found[len(shown):]; len(more) > 0 {
		text += "Also matching, search their exact names to load them: " + joinNames(more[:min(len(more), maxListed)]) + "\n"
	}
	return text
}

func oneLineSummary(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return s[:utf8SafeCut(s, maxSummary)]
}

func joinNames(tools []Tool) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

func LoadedNames(text string) []string {
	line, _, _ := strings.Cut(text, "\n")
	rest, ok := strings.CutPrefix(line, loadedPrefix)
	if !ok {
		return nil
	}
	return strings.Split(rest, ", ")
}
