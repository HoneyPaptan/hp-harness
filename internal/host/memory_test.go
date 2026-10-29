package host

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestMemory(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		run  func(t *testing.T, h *Memory)
	}{
		{
			name: "write creates parents",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/a/b/c.txt", []byte("x")); err != nil {
					t.Fatal(err)
				}
				ent, err := h.Stat("/a")
				if err != nil {
					t.Fatal(err)
				}
				if !ent.IsDir {
					t.Fatal("parent not directory")
				}
			},
		},
		{
			name: "write replaces content",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/f.txt", []byte("one")); err != nil {
					t.Fatal(err)
				}
				if err := h.WriteFile("/f.txt", []byte("two")); err != nil {
					t.Fatal(err)
				}
				data, err := h.ReadFile("/f.txt")
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "two" {
					t.Fatalf("got %q", data)
				}
			},
		},
		{
			name: "append new file",
			run: func(t *testing.T, h *Memory) {
				if err := h.Append("/log.txt", []byte("a")); err != nil {
					t.Fatal(err)
				}
				data, err := h.ReadFile("/log.txt")
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "a" {
					t.Fatalf("got %q", data)
				}
			},
		},
		{
			name: "append existing file",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/log.txt", []byte("a")); err != nil {
					t.Fatal(err)
				}
				if err := h.Append("/log.txt", []byte("b")); err != nil {
					t.Fatal(err)
				}
				data, err := h.ReadFile("/log.txt")
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "ab" {
					t.Fatalf("got %q", data)
				}
			},
		},
		{
			name: "read dir sorted",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/root/z.txt", []byte("z")); err != nil {
					t.Fatal(err)
				}
				if err := h.WriteFile("/root/a.txt", []byte("a")); err != nil {
					t.Fatal(err)
				}
				if err := h.WriteFile("/root/sub/x.txt", []byte("x")); err != nil {
					t.Fatal(err)
				}
				ents, err := h.ReadDir("/root")
				if err != nil {
					t.Fatal(err)
				}
				if len(ents) != 3 {
					t.Fatalf("len %d", len(ents))
				}
				want := []Entry{
					{Name: "a.txt", IsDir: false, Size: 1},
					{Name: "sub", IsDir: true, Size: 0},
					{Name: "z.txt", IsDir: false, Size: 1},
				}
				for i := range want {
					if ents[i] != want[i] {
						t.Fatalf("[%d] got %+v want %+v", i, ents[i], want[i])
					}
				}
			},
		},
		{
			name: "missing paths",
			run: func(t *testing.T, h *Memory) {
				_, err := h.ReadFile("/nope")
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("ReadFile: %v", err)
				}
				if !strings.Contains(err.Error(), "/nope") {
					t.Fatalf("ReadFile error %q", err)
				}
				_, err = h.Stat("/nope")
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("Stat: %v", err)
				}
				if !strings.Contains(err.Error(), "/nope") {
					t.Fatalf("Stat error %q", err)
				}
				_, err = h.ReadDir("/nope")
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("ReadDir: %v", err)
				}
				if !strings.Contains(err.Error(), "/nope") {
					t.Fatalf("ReadDir error %q", err)
				}
			},
		},
		{
			name: "write onto directory",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/d/f.txt", []byte("x")); err != nil {
					t.Fatal(err)
				}
				if err := h.WriteFile("/d", []byte("nope")); !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("got %v", err)
				}
				if _, ok := h.files["/d"]; ok {
					t.Fatal("directory path became file")
				}
			},
		},
		{
			name: "append onto directory",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/d/f.txt", []byte("x")); err != nil {
					t.Fatal(err)
				}
				if err := h.Append("/d", []byte("nope")); !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("got %v", err)
				}
				if _, ok := h.files["/d"]; ok {
					t.Fatal("directory path became file")
				}
			},
		},
		{
			name: "parent path is file",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/a", []byte("file")); err != nil {
					t.Fatal(err)
				}
				err := h.WriteFile("/a/b", []byte("x"))
				if !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("got %v", err)
				}
				if !strings.Contains(err.Error(), "/a") {
					t.Fatalf("error %q", err)
				}
				if _, ok := h.files["/a/b"]; ok {
					t.Fatal("file created under file parent")
				}
			},
		},
		{
			name: "deep parent path is file leaves no dirs",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/a", []byte("file")); err != nil {
					t.Fatal(err)
				}
				if err := h.WriteFile("/a/b/c", []byte("x")); !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("got %v", err)
				}
				if _, err := h.Stat("/a/b"); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("stat /a/b: %v", err)
				}
			},
		},
		{
			name: "read file on directory",
			run: func(t *testing.T, h *Memory) {
				if err := h.WriteFile("/d/f.txt", []byte("x")); err != nil {
					t.Fatal(err)
				}
				if _, err := h.ReadFile("/d"); err == nil {
					t.Fatal("expected error")
				}
			},
		},
		{
			name: "exec nil func",
			run: func(t *testing.T, h *Memory) {
				_, err := h.Exec(context.Background(), ExecRequest{Command: "echo"})
				if err == nil {
					t.Fatal("expected error")
				}
			},
		},
		{
			name: "exec delegates",
			run: func(t *testing.T, h *Memory) {
				h.ExecFunc = func(ctx context.Context, req ExecRequest) (ExecResult, error) {
					if req.Command != "hi" {
						t.Fatalf("command %q", req.Command)
					}
					return ExecResult{Output: []byte("ok"), ExitCode: 0}, nil
				}
				res, err := h.Exec(context.Background(), ExecRequest{Command: "hi"})
				if err != nil {
					t.Fatal(err)
				}
				if string(res.Output) != "ok" || res.ExitCode != 0 {
					t.Fatalf("%+v", res)
				}
			},
		},
		{
			name: "now returns clock",
			run: func(t *testing.T, h *Memory) {
				if !h.Now().Equal(fixed) {
					t.Fatalf("got %v want %v", h.Now(), fixed)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewMemory()
			h.Clock = fixed
			tc.run(t, h)
		})
	}
}

func TestMemorySleep(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	h := NewMemory()
	h.Clock = fixed
	d := 2*time.Hour + 30*time.Minute
	if err := h.Sleep(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if !h.Now().Equal(fixed.Add(d)) {
		t.Fatalf("clock = %v want %v", h.Now(), fixed.Add(d))
	}
	if h.Slept != d {
		t.Fatalf("Slept = %v want %v", h.Slept, d)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Sleep(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
