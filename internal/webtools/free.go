package webtools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const (
	WikipediaURL    = "https://en.wikipedia.org"
	DuckDuckGoURL   = "https://api.duckduckgo.com/"
	maxArticleBytes = 24 * 1024
	maxSnippetBytes = 400
)

type FreeConfig struct {
	Client     *http.Client
	Wikipedia  string
	DuckDuckGo string
	UserAgent  string
}

type freeResult struct {
	title   string
	url     string
	source  string
	snippet string
}

var (
	tagPattern     = regexp.MustCompile(`<[^>]*>`)
	readSchema     = json.RawMessage(`{"type":"object","properties":{"page":{"type":"string","description":"A Wikipedia article title or a wikipedia.org URL, usually one returned by web_search."}},"required":["page"],"additionalProperties":false}`)
	freeSearchNote = "Search the public web for free through Wikipedia and DuckDuckGo instant answers. Returns titles, URLs and short snippets. Good for facts, definitions and well known topics, not for news or niche pages. Use wiki_read to read a Wikipedia result in full."
)

func FreeTools(cfg FreeConfig) []tool.Tool {
	return []tool.Tool{FreeSearchTool(cfg), WikiReadTool(cfg)}
}

func FreeSearchTool(cfg FreeConfig) tool.Tool {
	return tool.Tool{
		Name:        "web_search",
		Description: freeSearchNote,
		Schema:      searchSchema,
		Run: func(ctx context.Context, _ tool.Env, raw json.RawMessage) (tool.Result, error) {
			query, count, errRes, ok := parseSearchArgs(raw)
			if !ok {
				return errRes, nil
			}
			return cfg.search(ctx, query, count)
		},
	}
}

func WikiReadTool(cfg FreeConfig) tool.Tool {
	return tool.Tool{
		Name:        "wiki_read",
		Description: "Read a Wikipedia article as plain text. Pass a title or a wikipedia.org URL from web_search.",
		Schema:      readSchema,
		Run: func(ctx context.Context, _ tool.Env, raw json.RawMessage) (tool.Result, error) {
			var args struct {
				Page string `json:"page"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return toolErr("%v", err), nil
			}
			base, title, err := cfg.wikiPage(strings.TrimSpace(args.Page))
			if err != nil {
				return toolErr("%v", err), nil
			}
			return cfg.read(ctx, base, title)
		},
	}
}

func (cfg FreeConfig) search(ctx context.Context, query string, count int) (tool.Result, error) {
	instant, errInstant := cfg.instant(ctx, query)
	wiki, errWiki := cfg.wikiSearch(ctx, cfg.Wikipedia, query, count)
	if errInstant != nil && errWiki != nil {
		return networkErr(ctx, errWiki)
	}
	results := mergeResults(instant, wiki, count)
	return tool.Result{Text: formatFree(results)}, nil
}

func mergeResults(first, rest []freeResult, count int) []freeResult {
	seen := map[string]bool{}
	var out []freeResult
	for _, r := range append(first, rest...) {
		if seen[r.url] || len(out) >= count {
			continue
		}
		seen[r.url] = true
		out = append(out, r)
	}
	return out
}

func formatFree(results []freeResult) string {
	if len(results) == 0 {
		return "no results"
	}
	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%d. %s (%s)\n%s", i+1, r.title, r.source, r.url)
		if r.snippet != "" {
			b.WriteByte('\n')
			b.WriteString(trimUTF8Bytes(r.snippet, maxSnippetBytes))
		}
	}
	return trimUTF8Bytes(b.String(), maxSearchOutputBytes)
}

func (cfg FreeConfig) instant(ctx context.Context, query string) ([]freeResult, error) {
	q := url.Values{"q": {query}, "format": {"json"}, "no_html": {"1"}, "skip_disambig": {"1"}}
	var body struct {
		Heading        string
		AbstractText   string
		AbstractURL    string
		AbstractSource string
		Answer         json.RawMessage
		Definition     string
		DefinitionURL  string
	}
	if err := cfg.getJSON(ctx, cfg.DuckDuckGo+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	var out []freeResult
	if answer := answerText(body.Answer); answer != "" {
		out = append(out, freeResult{title: "Instant answer", url: "https://duckduckgo.com/?q=" + url.QueryEscape(query), source: "DuckDuckGo", snippet: answer})
	}
	if body.AbstractText != "" && body.AbstractURL != "" {
		out = append(out, freeResult{title: body.Heading, url: body.AbstractURL, source: orDefault(body.AbstractSource, "DuckDuckGo"), snippet: body.AbstractText})
	}
	if body.Definition != "" && body.DefinitionURL != "" {
		out = append(out, freeResult{title: "Definition", url: body.DefinitionURL, source: "DuckDuckGo", snippet: body.Definition})
	}
	return out, nil
}

func answerText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func (cfg FreeConfig) wikiSearch(ctx context.Context, base, query string, count int) ([]freeResult, error) {
	q := url.Values{"action": {"query"}, "list": {"search"}, "srsearch": {query}, "srlimit": {fmt.Sprint(count)}, "format": {"json"}, "origin": {"*"}}
	var body struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := cfg.getJSON(ctx, base+"/w/api.php?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]freeResult, 0, len(body.Query.Search))
	for _, hit := range body.Query.Search {
		out = append(out, freeResult{title: hit.Title, url: articleURL(base, hit.Title), source: "Wikipedia", snippet: plainText(hit.Snippet)})
	}
	return out, nil
}

func (cfg FreeConfig) read(ctx context.Context, base, title string) (tool.Result, error) {
	q := url.Values{"action": {"query"}, "prop": {"extracts"}, "explaintext": {"1"}, "redirects": {"1"}, "titles": {title}, "format": {"json"}, "origin": {"*"}}
	var body struct {
		Query struct {
			Pages map[string]struct {
				Title   string  `json:"title"`
				Extract string  `json:"extract"`
				Missing *string `json:"missing"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := cfg.getJSON(ctx, base+"/w/api.php?"+q.Encode(), &body); err != nil {
		return networkErr(ctx, err)
	}
	for _, page := range body.Query.Pages {
		if page.Missing != nil || strings.TrimSpace(page.Extract) == "" {
			return toolErr("no Wikipedia article named %q", title), nil
		}
		text := fmt.Sprintf("%s\n%s\n\n%s", page.Title, articleURL(base, page.Title), strings.TrimSpace(page.Extract))
		return tool.Result{Text: trimUTF8Bytes(text, maxArticleBytes)}, nil
	}
	return toolErr("no Wikipedia article named %q", title), nil
}

func (cfg FreeConfig) wikiPage(page string) (base, title string, err error) {
	if page == "" {
		return "", "", fmt.Errorf("page is required")
	}
	if !strings.Contains(page, "://") {
		return cfg.Wikipedia, page, nil
	}
	u, err := url.Parse(page)
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".wikipedia.org") || !strings.HasPrefix(u.Path, "/wiki/") {
		return "", "", fmt.Errorf("only https wikipedia.org article URLs can be read")
	}
	title = strings.ReplaceAll(strings.TrimPrefix(u.Path, "/wiki/"), "_", " ")
	if u.Hostname() == hostOf(cfg.Wikipedia) {
		return cfg.Wikipedia, title, nil
	}
	return "https://" + u.Hostname(), title, nil
}

func (cfg FreeConfig) getJSON(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body, maxReadBody)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("http %d from %s", resp.StatusCode, hostOf(target))
	}
	return json.Unmarshal(body, out)
}

func articleURL(base, title string) string {
	return base + "/wiki/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
}

func plainText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagPattern.ReplaceAllString(s, ""))), " ")
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
