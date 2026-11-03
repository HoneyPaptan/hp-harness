package webtools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const (
	userAgent      = "hp-web-fetch"
	maxReadBody    = 2 * 1024 * 1024
	maxOutputBytes = 64 * 1024
	truncationLine = "[truncated at 64 KB]"
	errBodyLimit   = 200
)

var fetchSchema = json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"HTTP or HTTPS URL of a public page."}},"required":["url"],"additionalProperties":false}`)

type fetchArgs struct {
	URL string `json:"url"`
}

func FetchTool(c *http.Client) tool.Tool {
	if c == nil {
		c = NewClient(false)
	}
	return tool.Tool{
		Name:        "web_fetch",
		Description: "Read a public web page by URL and return its text. Public pages only: no sign in, no private or local addresses.",
		Schema:      fetchSchema,
		Run:         fetchRun(c),
	}
}

func fetchRun(c *http.Client) func(context.Context, tool.Env, json.RawMessage) (tool.Result, error) {
	return func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
		args, err := decodeFetchArgs(raw)
		if err != nil {
			return toolErr("%v", err), nil
		}
		parsed, err := validateFetchURL(args.URL)
		if err != nil {
			return toolErr("%v", err), nil
		}
		return doFetch(ctx, c, parsed)
	}
}

func decodeFetchArgs(raw json.RawMessage) (fetchArgs, error) {
	var args fetchArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return args, err
	}
	if strings.TrimSpace(args.URL) == "" {
		return args, fmt.Errorf("url is required")
	}
	return args, nil
}

func validateFetchURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid url: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid url: scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid url: host is required")
	}
	if u.User != nil {
		return nil, fmt.Errorf("invalid url: user info is not allowed")
	}
	return u, nil
}

func doFetch(ctx context.Context, c *http.Client, u *url.URL) (tool.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return toolErr("invalid url: %v", err), nil
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.Do(req)
	if err != nil {
		return networkErr(ctx, err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body, maxReadBody)
	if err != nil {
		return networkErr(ctx, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return httpStatusErr(resp.StatusCode, body), nil
	}
	text, title, err := bodyText(resp.Header.Get("Content-Type"), body)
	if err != nil {
		return toolErr("%v", err), nil
	}
	out := formatFetchOutput(resp.Request.URL.String(), title, text)
	return tool.Result{Text: out}, nil
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}

func httpStatusErr(code int, body []byte) tool.Result {
	msg := fmt.Sprintf("http %d", code)
	if snip := errBodySnip(body, ""); snip != "" {
		msg += " " + snip
	}
	return toolErr("%s", msg)
}

func errBodySnip(body []byte, redact string) string {
	if len(body) == 0 {
		return ""
	}
	snip := string(body)
	if redact != "" {
		snip = strings.ReplaceAll(snip, redact, "")
	}
	if len(snip) > errBodyLimit {
		snip = trimUTF8Bytes(snip, errBodyLimit)
	}
	return snip
}

func bodyText(contentType string, body []byte) (text, title string, err error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		mediaType = "application/octet-stream"
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	switch {
	case mediaType == "text/html" || mediaType == "application/xhtml+xml":
		title, text = htmlToText(body)
		return text, title, nil
	case strings.HasPrefix(mediaType, "text/"):
		return string(body), "", nil
	case mediaType == "application/json" || mediaType == "application/xml":
		return string(body), "", nil
	default:
		return "", "", fmt.Errorf("unsupported content type %s", mediaType)
	}
}

func formatFetchOutput(finalURL, title, text string) string {
	var b strings.Builder
	b.WriteString(finalURL)
	b.WriteByte('\n')
	if title != "" {
		b.WriteString(title)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(text)
	out := b.String()
	if len(out) <= maxOutputBytes {
		return out
	}
	cut := maxOutputBytes - len(truncationLine) - 1
	if cut < 0 {
		cut = 0
	}
	if cut > len(out) {
		cut = len(out)
	}
	return trimUTF8Bytes(out, cut) + "\n" + truncationLine
}

func toolErr(format string, a ...any) tool.Result {
	return tool.Result{Text: fmt.Sprintf(format, a...), IsError: true}
}

func networkErr(ctx context.Context, err error) (tool.Result, error) {
	if ctx.Err() != nil {
		return tool.Result{}, ctx.Err()
	}
	return toolErr("fetch failed: %v", err), nil
}
