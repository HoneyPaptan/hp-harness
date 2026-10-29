package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestBash(t *testing.T) {
	ctx := context.Background()
	dir := "/w"
	tests := []struct {
		name    string
		exec    func(context.Context, host.ExecRequest) (host.ExecResult, error)
		raw     string
		wantErr bool
		hostErr bool
		check   func(*testing.T, Result)
	}{
		{
			name: "success",
			exec: func(ctx context.Context, req host.ExecRequest) (host.ExecResult, error) {
				return host.ExecResult{Output: []byte("hi\n"), ExitCode: 0}, nil
			},
			raw: `{"command":"echo hi"}`,
			check: func(t *testing.T, r Result) {
				if !strings.Contains(r.Text, "hi") || !strings.Contains(r.Text, "exit code: 0") {
					t.Fatalf("%q", r.Text)
				}
			},
		},
		{
			name: "exit 3",
			exec: func(ctx context.Context, req host.ExecRequest) (host.ExecResult, error) {
				return host.ExecResult{Output: []byte("hi\n"), ExitCode: 3}, nil
			},
			raw: `{"command":"echo hi; exit 3"}`,
			check: func(t *testing.T, r Result) {
				if r.IsError || !strings.Contains(r.Text, "exit code: 3") {
					t.Fatalf("IsError=%v %q", r.IsError, r.Text)
				}
			},
		},
		{
			name: "timeout",
			exec: func(ctx context.Context, req host.ExecRequest) (host.ExecResult, error) {
				return host.ExecResult{Output: []byte("partial"), TimedOut: true}, nil
			},
			raw:     `{"command":"sleep 9","timeout_ms":1000}`,
			wantErr: true,
			check: func(t *testing.T, r Result) {
				if !strings.Contains(r.Text, "command timed out after 1s") {
					t.Fatalf("%q", r.Text)
				}
			},
		},
		{
			name: "clamp",
			exec: func(ctx context.Context, req host.ExecRequest) (host.ExecResult, error) {
				if req.Timeout != MaxTimeout {
					t.Fatalf("timeout %v", req.Timeout)
				}
				return host.ExecResult{ExitCode: 0}, nil
			},
			raw: `{"command":"x","timeout_ms":999999999}`,
		},
		{
			name: "default timeout",
			exec: func(ctx context.Context, req host.ExecRequest) (host.ExecResult, error) {
				if req.Timeout != DefaultTimeout {
					t.Fatalf("timeout %v", req.Timeout)
				}
				return host.ExecResult{ExitCode: 0}, nil
			},
			raw: `{"command":"x"}`,
		},
		{
			name: "dir",
			exec: func(ctx context.Context, req host.ExecRequest) (host.ExecResult, error) {
				if req.Dir != dir {
					t.Fatalf("dir %q", req.Dir)
				}
				return host.ExecResult{ExitCode: 0}, nil
			},
			raw: `{"command":"pwd"}`,
		},
		{
			name:    "empty",
			raw:     `{"command":""}`,
			wantErr: true,
		},
		{
			name: "host error",
			exec: func(ctx context.Context, req host.ExecRequest) (host.ExecResult, error) {
				return host.ExecResult{}, context.Canceled
			},
			raw:     `{"command":"x"}`,
			hostErr: true,
		},
		{
			name: "huge output",
			exec: func(ctx context.Context, req host.ExecRequest) (host.ExecResult, error) {
				return host.ExecResult{Output: []byte(strings.Repeat("a", 50000)), ExitCode: 0}, nil
			},
			raw: `{"command":"big"}`,
			check: func(t *testing.T, r Result) {
				if !strings.Contains(r.Text, "[truncated:") {
					t.Fatal("expected truncation")
				}
				if !strings.HasPrefix(r.Text, strings.Repeat("a", 100)) {
					t.Fatal("missing head")
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			if tc.exec != nil {
				h.ExecFunc = tc.exec
			}
			res, err := bashTool.Run(ctx, Env{Host: h, Dir: dir}, json.RawMessage(tc.raw))
			if tc.hostErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.IsError != tc.wantErr {
				t.Fatalf("IsError=%v text=%q", res.IsError, res.Text)
			}
			if tc.check != nil {
				tc.check(t, res)
			}
		})
	}
}

func TestBashTimeoutParse(t *testing.T) {
	if bashTimeout(0) != DefaultTimeout {
		t.Fatal()
	}
	if bashTimeout(int(MaxTimeout/time.Millisecond)+1) != MaxTimeout {
		t.Fatal()
	}
}
