package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/HoneyPaptan/hp-harness/internal/term"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

type stdTerminal struct {
	in      *term.Input
	mu      sync.Mutex
	restore func() error
}

var terminal = &stdTerminal{in: term.NewInput(int(os.Stdin.Fd()))}

func (t *stdTerminal) Read(p []byte) (int, error) {
	return t.in.Read(p)
}

func (*stdTerminal) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (*stdTerminal) Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func (t *stdTerminal) MakeRaw() (func() error, error) {
	restore, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}
	t.setRestore(restore)
	return t.restoreCurrent, nil
}

func (t *stdTerminal) setRestore(restore func() error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.restore = restore
}

func (t *stdTerminal) restoreCurrent() error {
	t.mu.Lock()
	restore := t.restore
	t.mu.Unlock()
	if restore == nil {
		return nil
	}
	return restore()
}

func (*stdTerminal) Width() int {
	cols, _, err := term.Size(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return cols
}

func (*stdTerminal) Signals(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, syscall.SIGHUP)
}

func (*stdTerminal) ReadSecret() (string, error) {
	return term.ReadSecret(int(os.Stdin.Fd()))
}

func (t *stdTerminal) Suspend() {
	if !t.Interactive() {
		return
	}
	resume := t.in.Pause()
	defer resume()
	_ = tui.DisableBracketedPaste(t)
	if err := t.restoreCurrent(); err != nil {
		return
	}
	stopSelf()
	restore, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return
	}
	t.setRestore(restore)
	_ = tui.EnableBracketedPaste(t)
}

func (t *stdTerminal) Handover(argv []string) error {
	if len(argv) == 0 {
		return errors.New("empty command")
	}
	if !t.Interactive() {
		return errors.New("not a terminal")
	}
	resume := t.in.Pause()
	defer resume()
	_ = tui.DisableBracketedPaste(t)
	if err := t.restoreCurrent(); err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	runErr := cmd.Run()
	restore, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return errors.Join(runErr, err)
	}
	t.setRestore(restore)
	_ = tui.EnableBracketedPaste(t)
	return runErr
}
