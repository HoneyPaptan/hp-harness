package host

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
)

var _ Host = (*Memory)(nil)

type Memory struct {
	mu       sync.Mutex
	files    map[string][]byte
	dirs     map[string]struct{}
	ExecFunc func(ctx context.Context, req ExecRequest) (ExecResult, error)
	Clock    time.Time
	Slept    time.Duration
}

func NewMemory() *Memory {
	return &Memory{files: make(map[string][]byte), dirs: make(map[string]struct{})}
}
func (m *Memory) ReadFile(p string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p = cleanAbs(p)
	data, ok := m.files[p]
	if !ok {
		err := fs.ErrNotExist
		if m.isDirLocked(p) {
			err = fs.ErrInvalid
		}
		return nil, pathErr("read", p, err)
	}
	return append([]byte(nil), data...), nil
}

func (m *Memory) WriteFile(p string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.putLocked(p, data, false)
}

func (m *Memory) Append(p string, line []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.putLocked(p, line, true)
}

func (m *Memory) putLocked(p string, data []byte, appendData bool) error {
	p = cleanAbs(p)
	if _, ok := m.files[p]; !ok && m.isDirLocked(p) {
		return pathErr("write", p, fs.ErrInvalid)
	}
	if err := m.ensureParentsLocked(p); err != nil {
		return err
	}
	if appendData {
		m.files[p] = append(append([]byte(nil), m.files[p]...), data...)
		return nil
	}
	m.files[p] = append([]byte(nil), data...)
	return nil
}

func (m *Memory) Stat(p string) (Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p = cleanAbs(p)
	if data, ok := m.files[p]; ok {
		return Entry{Name: path.Base(p), Size: int64(len(data))}, nil
	}
	if m.isDirLocked(p) {
		return Entry{Name: path.Base(p), IsDir: true}, nil
	}
	return Entry{}, pathErr("stat", p, fs.ErrNotExist)
}

func (m *Memory) ReadDir(p string) ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p = cleanAbs(p)
	if !m.isDirLocked(p) {
		if _, ok := m.files[p]; ok {
			return nil, pathErr("read", p, fs.ErrInvalid)
		}
		return nil, pathErr("read", p, fs.ErrNotExist)
	}
	byName := map[string]Entry{}
	for fp, data := range m.files {
		if n, ok := childName(p, fp); ok {
			byName[n] = Entry{Name: n, Size: int64(len(data))}
		}
	}
	for d := range m.dirs {
		if n, ok := childName(p, d); ok && byName[n].Name == "" {
			byName[n] = Entry{Name: n, IsDir: true}
		}
	}
	out := make([]Entry, 0, len(byName))
	for _, e := range byName {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (m *Memory) Exec(ctx context.Context, req ExecRequest) (ExecResult, error) {
	if m.ExecFunc == nil {
		return ExecResult{}, errors.New("exec not configured")
	}
	return m.ExecFunc(ctx, req)
}
func (m *Memory) Now() time.Time { return m.Clock }

func (m *Memory) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	m.Clock = m.Clock.Add(d)
	m.Slept += d
	m.mu.Unlock()
	return nil
}
func pathErr(op, p string, err error) error { return &fs.PathError{Op: op, Path: p, Err: err} }

func cleanAbs(p string) string {
	return path.Clean("/" + p)
}

func (m *Memory) ensureParentsLocked(p string) error {
	parents := parentDirs(p)
	for _, dir := range parents {
		if _, ok := m.files[dir]; ok {
			return pathErr("write", dir, fs.ErrInvalid)
		}
	}
	for _, dir := range parents {
		m.dirs[dir] = struct{}{}
	}
	return nil
}

func parentDirs(p string) []string {
	var dirs []string
	for dir := path.Dir(p); dir != "/"; dir = path.Dir(dir) {
		dirs = append(dirs, dir)
	}
	return dirs
}

func (m *Memory) isDirLocked(p string) bool { _, ok := m.dirs[p]; return p == "/" || ok }
func childName(dir, full string) (string, bool) {
	dir, full = cleanAbs(dir), cleanAbs(full)
	if path.Dir(full) != dir {
		return "", false
	}
	return path.Base(full), true
}
