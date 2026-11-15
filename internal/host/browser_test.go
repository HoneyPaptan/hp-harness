package host

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBrowserReportsChanges(t *testing.T) {
	b := NewBrowser()
	var got []Change
	b.OnChange(func(c Change) { got = append(got, c) })
	if err := b.WriteFile("/w/a.txt", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := b.Append("w/a.txt", []byte("two")); err != nil {
		t.Fatal(err)
	}
	if err := b.Remove("/w/a.txt"); err != nil {
		t.Fatal(err)
	}
	want := []Change{{OpWrite, "/w/a.txt", []byte("one")}, {OpAppend, "/w/a.txt", []byte("two")}, {Op: OpRemove, Path: "/w/a.txt"}}
	if len(got) != len(want) {
		t.Fatalf("changes = %+v", got)
	}
	for i := range want {
		if got[i].Op != want[i].Op || got[i].Path != want[i].Path || string(got[i].Data) != string(want[i].Data) {
			t.Fatalf("change %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBrowserRestoreIsSilent(t *testing.T) {
	b := NewBrowser()
	b.OnChange(func(c Change) { t.Fatalf("unexpected change %+v", c) })
	if err := b.Restore("/w/a.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	b.Mkdir("/w/empty")
	if e, err := b.Stat("/w/empty"); err != nil || !e.IsDir {
		t.Fatalf("stat = %+v, %v", e, err)
	}
}

func TestBrowserFailedWriteIsSilent(t *testing.T) {
	b := NewBrowser()
	b.Mkdir("/w/dir")
	b.OnChange(func(c Change) { t.Fatalf("unexpected change %+v", c) })
	if err := b.WriteFile("/w/dir", []byte("x")); err == nil {
		t.Fatal("writing over a directory succeeded")
	}
}

func TestBrowserHasNoShell(t *testing.T) {
	_, err := NewBrowser().Exec(context.Background(), ExecRequest{Command: "ls"})
	if !errors.Is(err, ErrNoShell) {
		t.Fatalf("err = %v", err)
	}
}

func TestBrowserSleepHonorsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewBrowser().Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
