//go:build unix

package host

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var _ Host = Native{}

type Native struct{}

func (Native) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (Native) WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (Native) Append(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(line)
	serr := f.Sync()
	cerr := f.Close()
	return firstErr(werr, serr, cerr)
}

func (Native) Stat(path string) (Entry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Entry{}, err
	}
	return entryFromFileInfo(filepath.Base(path), info), nil
}

func (Native) ReadDir(path string) ([]Entry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		info, err := entryInfo(path, e)
		if err != nil {
			continue
		}
		out = append(out, entryFromFileInfo(e.Name(), info))
	}
	return out, nil
}

func entryInfo(dir string, e os.DirEntry) (fs.FileInfo, error) {
	if e.Type()&fs.ModeSymlink != 0 {
		return os.Stat(filepath.Join(dir, e.Name()))
	}
	return e.Info()
}

func (Native) Exec(ctx context.Context, req ExecRequest) (ExecResult, error) {
	runCtx := ctx
	var stop context.CancelFunc
	if req.Timeout > 0 {
		runCtx, stop = context.WithTimeout(ctx, req.Timeout)
		defer stop()
	}
	cmd := exec.CommandContext(runCtx, "bash", "-c", req.Command)
	cmd.Dir = req.Dir
	cmd.WaitDelay = 100 * time.Millisecond
	configureExecCmd(cmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if err == nil {
		return ExecResult{Output: buf.Bytes(), ExitCode: 0}, nil
	}
	if req.Timeout > 0 && ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return ExecResult{Output: buf.Bytes(), ExitCode: -1, TimedOut: true}, nil
	}
	if ctx.Err() != nil {
		return ExecResult{}, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return execResultFromExit(exitErr, buf.Bytes()), nil
	}
	return ExecResult{}, err
}

func (Native) Now() time.Time {
	return time.Now()
}

func entryFromFileInfo(name string, info fs.FileInfo) Entry {
	return Entry{Name: name, IsDir: info.IsDir(), Size: info.Size()}
}

func execResultFromExit(err *exec.ExitError, output []byte) ExecResult {
	return ExecResult{Output: output, ExitCode: err.ExitCode()}
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
