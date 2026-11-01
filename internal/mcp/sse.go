package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

func (c *httpConn) readStream(ctx context.Context, body io.Reader, id int64) (json.RawMessage, error) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64<<10), maxMessage)
	var data []string
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if raw, done, err := c.event(ctx, data, id); done {
				return raw, err
			}
			data = nil
			continue
		}
		if v, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimPrefix(v, " "))
		}
	}
	return nil, streamFailure(sc.Err())
}

func streamFailure(err error) error {
	if errors.Is(err, bufio.ErrTooLong) {
		return fmt.Errorf("server message over %d bytes", maxMessage)
	}
	if err != nil {
		return err
	}
	return errNoResponse
}

func (c *httpConn) event(ctx context.Context, data []string, id int64) (json.RawMessage, bool, error) {
	var m message
	if len(data) == 0 || json.Unmarshal([]byte(strings.Join(data, "\n")), &m) != nil {
		return nil, false, nil
	}
	if m.isServerRequest() {
		c.answer(ctx, m)
		return nil, false, nil
	}
	if got, ok := responseID(m.ID); ok && m.isResponse() && got == id {
		raw, err := m.outcome()
		return raw, true, err
	}
	return nil, false, nil
}

func (c *httpConn) answer(ctx context.Context, m message) {
	if payload, err := replyTo(m); err == nil {
		_ = c.send(ctx, payload)
	}
}
