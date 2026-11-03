package host

import (
	"context"
	"errors"
	"time"
)

var ErrLocked = errors.New("locked by another process")

type Entry struct {
	Name   string
	IsDir  bool
	IsLink bool
	Size   int64
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
	Remove(path string) error
	Append(path string, line []byte) error
	Stat(path string) (Entry, error)
	Lstat(path string) (Entry, error)
	ReadDir(path string) ([]Entry, error)
	Lock(path string) (func(), error)
	Exec(ctx context.Context, req ExecRequest) (ExecResult, error)
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}
