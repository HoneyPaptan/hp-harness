//go:build unix

package host

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNative(t *testing.T) {
	n := Native{}
	root := t.TempDir()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "write creates parents",
			run: func(t *testing.T) {
				p := filepath.Join(root, "a", "b", "c.txt")
				if err := n.WriteFile(p, []byte("x")); err != nil {
					t.Fatal(err)
				}
				ent, err := n.Stat(filepath.Join(root, "a"))
				if err != nil {
					t.Fatal(err)
				}
				if !ent.IsDir {
					t.Fatal("parent not directory")
				}
			},
		},
		{
			name: "append twice concatenated",
			run: func(t *testing.T) {
				p := filepath.Join(root, "log.txt")
				if err := n.Append(p, []byte("a")); err != nil {
					t.Fatal(err)
				}
				if err := n.Append(p, []byte("b")); err != nil {
					t.Fatal(err)
				}
				data, err := n.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "ab" {
					t.Fatalf("got %q", data)
				}
			},
		},
		{
			name: "stat and read dir",
			run: func(t *testing.T) {
				dir := filepath.Join(root, "root")
				if err := n.WriteFile(filepath.Join(dir, "z.txt"), []byte("z")); err != nil {
					t.Fatal(err)
				}
				if err := n.WriteFile(filepath.Join(dir, "a.txt"), []byte("a")); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
					t.Fatal(err)
				}
				st, err := n.Stat(filepath.Join(dir, "a.txt"))
				if err != nil {
					t.Fatal(err)
				}
				if st.Name != "a.txt" || st.IsDir || st.Size != 1 {
					t.Fatalf("stat %+v", st)
				}
				ents, err := n.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				if len(ents) != 3 {
					t.Fatalf("len %d", len(ents))
				}
				if ents[0].Name != "a.txt" || ents[1].Name != "sub" || ents[2].Name != "z.txt" {
					t.Fatalf("order %+v", ents)
				}
				if !ents[1].IsDir {
					t.Fatal("sub not dir")
				}
			},
		},
		{
			name: "missing path errors",
			run: func(t *testing.T) {
				missing := filepath.Join(root, "nope")
				_, err := n.ReadFile(missing)
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("ReadFile: %v", err)
				}
				_, err = n.Stat(missing)
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("Stat: %v", err)
				}
				_, err = n.ReadDir(missing)
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("ReadDir: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

func TestNativeExec(t *testing.T) {
	n := Native{}
	root := t.TempDir()

	t.Run("exit code and output", func(t *testing.T) {
		res, err := n.Exec(context.Background(), ExecRequest{
			Command: "echo hi; echo err >&2; exit 3",
		})
		if err != nil {
			t.Fatal(err)
		}
		out := string(res.Output)
		if !strings.Contains(out, "hi") || !strings.Contains(out, "err") {
			t.Fatalf("output %q", out)
		}
		if res.ExitCode != 3 {
			t.Fatalf("exit %d", res.ExitCode)
		}
	})

	t.Run("runs in dir", func(t *testing.T) {
		sub := filepath.Join(root, "wd")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		res, err := n.Exec(context.Background(), ExecRequest{Command: "pwd", Dir: sub})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(res.Output), sub) {
			t.Fatalf("output %q dir %q", res.Output, sub)
		}
	})

	t.Run("timeout sleep", func(t *testing.T) {
		start := time.Now()
		res, err := n.Exec(context.Background(), ExecRequest{
			Command: "sleep 5",
			Timeout: 100 * time.Millisecond,
		})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		if !res.TimedOut || res.ExitCode != -1 {
			t.Fatalf("%+v", res)
		}
		if elapsed >= 2*time.Second {
			t.Fatalf("elapsed %v", elapsed)
		}
	})

	t.Run("timeout process group", func(t *testing.T) {
		start := time.Now()
		res, err := n.Exec(context.Background(), ExecRequest{
			Command: "sleep 5 & wait",
			Timeout: 100 * time.Millisecond,
		})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		if !res.TimedOut || res.ExitCode != -1 {
			t.Fatalf("%+v", res)
		}
		if elapsed >= 2*time.Second {
			t.Fatalf("elapsed %v", elapsed)
		}
	})

	t.Run("cancelled parent ctx", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := n.Exec(ctx, ExecRequest{Command: "sleep 1"})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	})
}
