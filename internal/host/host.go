package host

import (
	"context"
	"time"
)

type Entry struct {
	Name  string
	IsDir bool
	Size  int64
}

type ExecRequest struct {
	Command string
	Dir     string
	Timeout time.Duration
}

type ExecResult struct {
	Output   []byte
	ExitCode int
	TimedOut bool
}

type Host interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	Append(path string, line []byte) error
	Stat(path string) (Entry, error)
	ReadDir(path string) ([]Entry, error)
	Exec(ctx context.Context, req ExecRequest) (ExecResult, error)
	Now() time.Time
}
