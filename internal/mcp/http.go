package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	sessionHeader = "Mcp-Session-Id"
	versionHeader = "MCP-Protocol-Version"
	maxErrorBody  = 512
)

type httpConn struct {
	url     string
	headers map[string]string
	client  *http.Client
	mu      sync.Mutex
	session string
	version string
	gone    atomic.Bool
	closed  atomic.Bool
}

func newHTTPConn(cfg Config) *httpConn {
	return &httpConn{url: cfg.URL, headers: cfg.Headers, client: noRedirectClient()}
}

func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (c *httpConn) call(ctx context.Context, id int64, payload []byte) (json.RawMessage, error) {
	resp, err := c.post(ctx, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := c.check(resp); err != nil {
		return nil, err
	}
	c.keepSession(resp)
	if isEventStream(resp) {
		return c.readStream(ctx, resp.Body, id)
	}
	return decodeBody(resp.Body, id)
}

func (c *httpConn) send(ctx context.Context, payload []byte) error {
	resp, err := c.post(ctx, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.check(resp)
}

func (c *httpConn) post(ctx context.Context, payload []byte) (*http.Response, error) {
	req, err := c.newRequest(ctx, http.MethodPost, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	return c.client.Do(req)
}

func (c *httpConn) newRequest(ctx context.Context, method string, payload []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	session, version := c.ids()
	if session != "" {
		req.Header.Set(sessionHeader, session)
	}
	if version != "" {
		req.Header.Set(versionHeader, version)
	}
	return req, nil
}

func (c *httpConn) ids() (string, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session, c.version
}

func (c *httpConn) check(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	session, _ := c.ids()
	if resp.StatusCode == http.StatusNotFound && session != "" {
		c.gone.Store(true)
		return errors.New("session expired")
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (c *httpConn) keepSession(resp *http.Response) {
	id := resp.Header.Get(sessionHeader)
	if id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == "" {
		c.session = id
	}
}

func isEventStream(resp *http.Response) bool {
	kind, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return err == nil && kind == "text/event-stream"
}

func decodeBody(body io.Reader, id int64) (json.RawMessage, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxMessage+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMessage {
		return nil, fmt.Errorf("server message over %d bytes", maxMessage)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errNoResponse
	}
	var m message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if got, ok := responseID(m.ID); !ok || got != id {
		return nil, fmt.Errorf("response id %s does not match request %d", m.ID, id)
	}
	return m.outcome()
}

func (c *httpConn) setVersion(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version = version
}

func (c *httpConn) abort() bool {
	return false
}

func (c *httpConn) alive() bool {
	return !c.gone.Load() && !c.closed.Load()
}

func (c *httpConn) tail() string {
	return ""
}

func (c *httpConn) close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	defer c.client.CloseIdleConnections()
	session, _ := c.ids()
	if session == "" || c.gone.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cancelTimeout)
	defer cancel()
	req, err := c.newRequest(ctx, http.MethodDelete, nil)
	if err != nil {
		return
	}
	if resp, err := c.client.Do(req); err == nil {
		resp.Body.Close()
	}
}
