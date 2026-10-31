package jev

import (
	"fmt"
	"sort"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

type hit struct {
	path  string
	start int
	end   int
	score float64
}

func mergeHits(hits []hit) []hit {
	ordered := append([]hit(nil), hits...)
	sort.Slice(ordered, func(i, j int) bool { return positionLess(ordered[i], ordered[j]) })
	var out []hit
	for _, h := range ordered {
		if n := len(out); n > 0 && touches(out[n-1], h) {
			out[n-1].end = max(out[n-1].end, h.end)
			out[n-1].score = max(out[n-1].score, h.score)
			continue
		}
		out = append(out, h)
	}
	return out
}

func touches(a, b hit) bool {
	return a.path == b.path && b.start <= a.end+1
}

func positionLess(a, b hit) bool {
	if a.path != b.path {
		return a.path < b.path
	}
	return a.start < b.start
}

func rankHits(hits []hit) {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return positionLess(hits[i], hits[j])
	})
}

func formatHits(hits []hit, limit int) tool.Result {
	merged := mergeHits(hits)
	if len(merged) == 0 {
		return tool.Result{Text: noMatches}
	}
	rankHits(merged)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	lines := make([]string, len(merged))
	for i, h := range merged {
		lines[i] = fmt.Sprintf("%s:%d-%d  %.2f", h.path, h.start, h.end, h.score)
	}
	return tool.Result{Text: strings.Join(lines, "\n")}
}
