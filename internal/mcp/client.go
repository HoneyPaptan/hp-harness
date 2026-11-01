package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const (
	DefaultTimeout = 60 * time.Second
	maxAttempts    = 2
	maxHangs       = 3
	maxPages       = 1000
	redacted       = "[redacted]"
)

type Config struct {
	Name    string
	Command string
	Args    []string
	Dir     string
	Env     []string
	URL     string
	Headers map[string]string
	Secrets []string
	Version string
	Timeout time.Duration
}

type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type Client struct {
	cfg      Config
	gate     chan struct{}
	sess     *session
	attempts atomic.Int32
	hangs    atomic.Int32
	hung     atomic.Pointer[session]
	last     error
}

func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Client{cfg: cfg, gate: make(chan struct{}, 1)}
}

func (c *Client) Tools(ctx context.Context) ([]Tool, error) {
	var out []Tool
	cursor := ""
	for range maxPages {
		page, err := c.listPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, page.tools()...)
		if page.NextCursor == "" {
			return out, nil
		}
		cursor = page.NextCursor
	}
	return nil, c.clean(fmt.Errorf("tools/list did not finish after %d pages", maxPages))
}

func (c *Client) listPage(ctx context.Context, cursor string) (listResult, error) {
	var params any
	if cursor != "" {
		params = map[string]string{"cursor": cursor}
	}
	raw, err := c.request(ctx, "tools/list", params)
	if err != nil {
		return listResult{}, err
	}
	var page listResult
	if err := json.Unmarshal(raw, &page); err != nil {
		return listResult{}, c.clean(fmt.Errorf("tools/list: %w", err))
	}
	return page, nil
}

func (c *Client) Call(ctx context.Context, name string, args json.RawMessage) (tool.Result, error) {
	params, err := callParams(name, args)
	if err != nil {
		return tool.Result{}, c.clean(err)
	}
	raw, err := c.request(ctx, "tools/call", params)
	if err != nil {
		return tool.Result{}, err
	}
	res, err := decodeCall(raw)
	if err != nil {
		return tool.Result{}, c.clean(fmt.Errorf("tools/call: %w", err))
	}
	return res, nil
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s, err := c.ready(ctx)
	if err != nil {
		return nil, c.clean(err)
	}
	rctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	raw, err := s.call(rctx, method, params)
	if err != nil {
		return nil, c.clean(c.callError(ctx, rctx, s, method, err))
	}
	c.attempts.Store(0)
	c.hangs.Store(0)
	return raw, nil
}

func (c *Client) callError(ctx, rctx context.Context, s *session, method string, err error) error {
	if ctx.Err() == nil && errors.Is(rctx.Err(), context.DeadlineExceeded) {
		c.markHung(s)
		return s.describe(fmt.Errorf("%s timed out after %s", method, c.cfg.Timeout))
	}
	return s.describe(fmt.Errorf("%s: %w", method, err))
}

func (c *Client) markHung(s *session) {
	if !s.abort() {
		return
	}
	if c.hung.Swap(s) != s {
		c.hangs.Add(1)
	}
}

func (c *Client) ready(ctx context.Context) (*session, error) {
	if err := c.lock(ctx); err != nil {
		return nil, err
	}
	defer c.unlock()
	if c.sess != nil && c.sess.alive() {
		return c.sess, nil
	}
	free := c.freeRestart()
	c.dropDead()
	if free {
		return c.start(ctx)
	}
	if int(c.attempts.Load()) >= maxAttempts {
		return nil, fmt.Errorf("server stopped and was not restarted again: %w", c.last)
	}
	c.attempts.Add(1)
	return c.start(ctx)
}

func (c *Client) freeRestart() bool {
	n := c.hangs.Load()
	return c.sess != nil && c.sess == c.hung.Load() && n > 1 && n < maxHangs
}

func (c *Client) dropDead() {
	if c.sess == nil {
		return
	}
	c.last = c.sess.describe(errors.New("server stopped"))
	c.sess.close()
	c.sess = nil
	c.attempts.CompareAndSwap(0, 1)
}

func (c *Client) start(ctx context.Context) (*session, error) {
	s, err := open(ctx, c.cfg)
	if err != nil {
		c.last = err
		if ctx.Err() != nil {
			c.attempts.Add(-1)
		}
		return nil, err
	}
	c.sess = s
	return s, nil
}

func (c *Client) Close() {
	c.gate <- struct{}{}
	defer c.unlock()
	if c.sess != nil {
		c.sess.close()
		c.sess = nil
	}
	c.attempts.Store(0)
	c.hangs.Store(0)
	c.hung.Store(nil)
	c.last = nil
}

func (c *Client) lock(ctx context.Context) error {
	select {
	case c.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) unlock() {
	<-c.gate
}

func (c *Client) clean(err error) error {
	text := err.Error()
	for _, s := range c.cfg.Secrets {
		if s != "" {
			text = strings.ReplaceAll(text, s, redacted)
		}
	}
	return fmt.Errorf("mcp %s: %s", c.cfg.Name, text)
}
