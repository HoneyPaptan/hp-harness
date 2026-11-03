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
	if c == nil {
		c = http.DefaultClient
	}
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
	var args exaSearchArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return toolErr("%v", err), nil
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return toolErr("query is required"), nil
	}
	count := 5
	if args.Count != nil {
		count = *args.Count
		if count < 1 {
			count = 1
		} else if count > 10 {
			count = 10
		}
	}
	var req exaSearchRequest
	req.Query = query
	req.Type = "auto"
	req.NumResults = count
	req.Contents.Highlights = true
	reqBody, err := json.Marshal(req)
	if err != nil {
		return toolErr("%v", err), nil
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return toolErr("%v", err), nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", t.key)
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
	var parsed struct {
		Results []exaResult `json:"results"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return toolErr("invalid exa response: %v", err), nil
	}
	return tool.Result{Text: formatExaOutput(parsed.Results)}, nil
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
		if len(r.Highlights) > 0 {
			parts := make([]string, 0, len(r.Highlights))
			for _, p := range r.Highlights {
				if len(p) > maxHighlightBytes {
					p = p[:maxHighlightBytes]
				}
				if p != "" {
					parts = append(parts, p)
				}
			}
			if len(parts) > 0 {
				b.WriteByte('\n')
				b.WriteString(strings.Join(parts, " … "))
			}
		}
	}
	out := b.String()
	if len(out) > maxSearchOutputBytes {
		return out[:maxSearchOutputBytes]
	}
	return out
}
