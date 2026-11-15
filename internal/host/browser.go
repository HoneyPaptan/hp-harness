package host

import (
	"context"
	"errors"
	"sync"
	"time"
)

var _ Host = (*Browser)(nil)

var ErrNoShell = errors.New("no shell in the browser sandbox; use the read, write, edit, grep and list tools")

type ChangeOp string

const (
	OpWrite  ChangeOp = "write"
	OpAppend ChangeOp = "append"
	OpRemove ChangeOp = "remove"
)

type Change struct {
	Op   ChangeOp
	Path string
	Data []byte
}

type Browser struct {
	mem *Memory
	mu  sync.Mutex
	on  func(Change)
}

func NewBrowser() *Browser {
	return &Browser{mem: NewMemory()}
}

func (b *Browser) OnChange(fn func(Change)) {
	b.mu.Lock()
	b.on = fn
	b.mu.Unlock()
}

func (b *Browser) Restore(p string, data []byte) error {
	return b.mem.WriteFile(p, data)
}

func (b *Browser) Mkdir(p string) {
	b.mem.mkdirAll(p)
}

func (b *Browser) ReadFile(p string) ([]byte, error) { return b.mem.ReadFile(p) }
func (b *Browser) Stat(p string) (Entry, error)      { return b.mem.Stat(p) }
func (b *Browser) Lstat(p string) (Entry, error)     { return b.mem.Lstat(p) }
func (b *Browser) ReadDir(p string) ([]Entry, error) { return b.mem.ReadDir(p) }
func (b *Browser) Lock(p string) (func(), error)     { return b.mem.Lock(p) }

func (b *Browser) WriteFile(p string, data []byte) error {
	return b.notify(b.mem.WriteFile(p, data), Change{Op: OpWrite, Path: cleanAbs(p), Data: data})
}

func (b *Browser) Append(p string, line []byte) error {
	return b.notify(b.mem.Append(p, line), Change{Op: OpAppend, Path: cleanAbs(p), Data: line})
}

func (b *Browser) Remove(p string) error {
	return b.notify(b.mem.Remove(p), Change{Op: OpRemove, Path: cleanAbs(p)})
}

func (b *Browser) notify(err error, c Change) error {
	if err != nil {
		return err
	}
	b.mu.Lock()
	on := b.on
	b.mu.Unlock()
	if on != nil {
		c.Data = append([]byte(nil), c.Data...)
		on(c)
	}
	return nil
}

func (b *Browser) Exec(context.Context, ExecRequest) (ExecResult, error) {
	return ExecResult{}, ErrNoShell
}

func (b *Browser) Now() time.Time { return time.Now() }

func (b *Browser) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
