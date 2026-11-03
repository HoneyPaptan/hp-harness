package webtools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const (
	ExaSearchURL         = "https://api.exa.ai/search"
	maxSearchOutputBytes = 16 * 1024
	maxHighlightBytes    = 300
)

var searchSchema = json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query."},"count":{"type":"integer","minimum":1,"maximum":10,"description":"Number of results, 1 to 10."}},"required":["query"],"additionalProperties":false}`)

type exaSearchArgs struct {
	Query string `json:"query"`
	Count *int   `json:"count"`
}

type exaSearchRequest struct {
	Query      string `json:"query"`
	Type       string `json:"type"`
	NumResults int    `json:"numResults"`
	Contents   struct {
		Highlights bool `json:"highlights"`
	} `json:"contents"`
}

type exaResult struct {
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Highlights []string `json:"highlights"`
}

func SearchTool(c *http.Client, endpoint, key string) tool.Tool {
	t := exaTool{c: c, endpoint: endpoint, key: key}
	return tool.Tool{
		Name:        "web_search",
		Description: "Search the public web and return titles, URLs and short highlights. Queries leave the machine. Use web_fetch to read a result.",
		Schema:      searchSchema,
		Run:         t.run,
	}
}

type exaTool struct {
	c        *http.Client
	endpoint string
	key      string
}

func (t *exaTool) run(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
	query, count, errRes, ok := parseSearchArgs(raw)
	if !ok {
		return errRes, nil
	}
	httpReq, errRes, ok := newExaRequest(ctx, t.endpoint, t.key, query, count)
	if !ok {
		return errRes, nil
	}
	resp, err := t.c.Do(httpReq)
	if err != nil {
		return networkErr(ctx, err)
	}
	defer resp.Body.Close()
	respBody, err := readLimited(resp.Body, maxReadBody)
	if err != nil {
		return networkErr(ctx, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := fmt.Sprintf("exa http %d:", resp.StatusCode)
		if snip := errBodySnip(respBody, t.key); snip != "" {
			msg += " " + snip
		}
		return toolErr("%s", msg), nil
	}
	results, errRes, ok := decodeExaResults(respBody)
	if !ok {
		return errRes, nil
	}
	return tool.Result{Text: formatExaOutput(results)}, nil
}

func parseSearchArgs(raw json.RawMessage) (query string, count int, errRes tool.Result, ok bool) {
	var args exaSearchArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return "", 0, toolErr("%v", err), false
	}
	query = strings.TrimSpace(args.Query)
	if query == "" {
		return "", 0, toolErr("query is required"), false
	}
	return query, clampCount(args.Count), tool.Result{}, true
}

func clampCount(count *int) int {
	if count == nil {
		return 5
	}
	n := *count
	if n < 1 {
		return 1
	}
	if n > 10 {
		return 10
	}
	return n
}

func newExaRequest(ctx context.Context, endpoint, key, query string, count int) (*http.Request, tool.Result, bool) {
	var req exaSearchRequest
	req.Query = query
	req.Type = "auto"
	req.NumResults = count
	req.Contents.Highlights = true
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, toolErr("%v", err), false
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, toolErr("%v", err), false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", key)
	return httpReq, tool.Result{}, true
}

func decodeExaResults(body []byte) ([]exaResult, tool.Result, bool) {
	var parsed struct {
		Results []exaResult `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, toolErr("invalid exa response: %v", err), false
	}
	return parsed.Results, tool.Result{}, true
}

func formatExaOutput(results []exaResult) string {
	if len(results) == 0 {
		return "no results"
	}
	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%d. %s\n%s", i+1, r.Title, r.URL)
		if hl := joinHighlights(r.Highlights); hl != "" {
			b.WriteByte('\n')
			b.WriteString(hl)
		}
	}
	return trimUTF8Bytes(b.String(), maxSearchOutputBytes)
}

func joinHighlights(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	trimmed := make([]string, 0, len(parts))
	for _, p := range parts {
		p = trimUTF8Bytes(p, maxHighlightBytes)
		if p != "" {
			trimmed = append(trimmed, p)
		}
	}
	return strings.Join(trimmed, " … ")
}
