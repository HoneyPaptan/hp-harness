//go:build linux || darwin

package term

import (
	"io"
	"os"
	"syscall"
	"testing"
	"time"
)

type readResult struct {
	text string
	err  error
}

func startReader(in *Input) <-chan readResult {
	out := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 16)
		n, err := in.Read(buf)
		out <- readResult{string(buf[:n]), err}
	}()
	return out
}

func newPipeInput(t *testing.T) (*Input, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	return NewInput(int(r.Fd())), w
}

func TestInputReadsData(t *testing.T) {
	in, w := newPipeInput(t)
	got := startReader(in)
	if _, err := w.WriteString("abc"); err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-got:
		if res.text != "abc" || res.err != nil {
			t.Fatalf("read %q, %v", res.text, res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read did not return")
	}
}

func TestInputReportsEnd(t *testing.T) {
	in, w := newPipeInput(t)
	got := startReader(in)
	w.Close()
	select {
	case res := <-got:
		if res.err != io.EOF {
			t.Fatalf("err %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read did not return")
	}
}

func TestInputPauseKeepsDataForAfterResume(t *testing.T) {
	in, w := newPipeInput(t)
	got := startReader(in)
	time.Sleep(20 * time.Millisecond)
	resume := in.Pause()
	if _, err := w.WriteString("typed in the editor"); err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-got:
		t.Fatalf("read %q while paused", res.text)
	case <-time.After(100 * time.Millisecond):
	}
	var left [32]byte
	if err := syscall.SetNonblock(in.fd, true); err != nil {
		t.Fatal(err)
	}
	n, err := syscall.Read(in.fd, left[:])
	if err != nil || string(left[:n]) != "typed in the editor" {
		t.Fatalf("data %q, %v", left[:max(n, 0)], err)
	}
	if err := syscall.SetNonblock(in.fd, false); err != nil {
		t.Fatal(err)
	}
	resume()
	if _, err := w.WriteString("back"); err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-got:
		if res.text != "back" {
			t.Fatalf("read %q", res.text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read did not resume")
	}
}

func TestInputPauseWithoutReader(t *testing.T) {
	in, _ := newPipeInput(t)
	resume := in.Pause()
	resume()
	resume = in.Pause()
	resume()
}
