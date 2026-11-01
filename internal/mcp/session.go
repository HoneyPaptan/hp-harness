package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

const cancelTimeout = 2 * time.Second

var errNoResponse = errors.New("server closed the stream without a response")

type conn interface {
	call(ctx context.Context, id int64, payload []byte) (json.RawMessage, error)
	send(ctx context.Context, payload []byte) error
	setVersion(version string)
	alive() bool
	tail() string
	close()
}

type session struct {
	conn conn
	next atomic.Int64
}

func open(ctx context.Context, cfg Config) (*session, error) {
	cn, err := dial(cfg)
	if err != nil {
		return nil, err
	}
	s := &session{conn: cn}
	hctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if err := s.handshake(hctx, cfg.Version); err != nil {
		err = s.describe(fmt.Errorf("initialize: %w", err))
		cn.close()
		return nil, err
	}
	return s, nil
}

func dial(cfg Config) (conn, error) {
	if cfg.URL != "" {
		return newHTTPConn(cfg), nil
	}
	return startStdio(cfg)
}

func (s *session) handshake(ctx context.Context, version string) error {
	raw, err := s.call(ctx, "initialize", initParams(version))
	if err != nil {
		return err
	}
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return err
	}
	if !acceptedVersions[res.ProtocolVersion] {
		return fmt.Errorf("unsupported protocol version %q", res.ProtocolVersion)
	}
	s.conn.setVersion(res.ProtocolVersion)
	return s.notify(ctx, "notifications/initialized", nil)
}

func initParams(version string) map[string]any {
	if version == "" {
		version = "dev"
	}
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "hp", "version": version},
	}
}

func (s *session) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := s.next.Add(1)
	payload, err := encodeRequest(id, method, params)
	if err != nil {
		return nil, err
	}
	raw, err := s.conn.call(ctx, id, payload)
	if err != nil && ctx.Err() != nil {
		go s.cancelled(id)
	}
	return raw, err
}

func (s *session) cancelled(id int64) {
	ctx, cancel := context.WithTimeout(context.Background(), cancelTimeout)
	defer cancel()
	_ = s.notify(ctx, "notifications/cancelled", map[string]any{"requestId": id})
}

func (s *session) notify(ctx context.Context, method string, params any) error {
	payload, err := encodeNotification(method, params)
	if err != nil {
		return err
	}
	return s.conn.send(ctx, payload)
}

func (s *session) alive() bool {
	return s.conn.alive()
}

func (s *session) close() {
	s.conn.close()
}

func (s *session) describe(err error) error {
	tail := strings.TrimSpace(s.conn.tail())
	if tail == "" {
		return err
	}
	return fmt.Errorf("%w\nserver stderr:\n%s", err, tail)
}
