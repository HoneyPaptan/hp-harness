package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/HoneyPaptan/hp-harness/internal/term"
)

type stdTerminal struct{}

func (stdTerminal) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

func (stdTerminal) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (stdTerminal) Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func (stdTerminal) MakeRaw() (func() error, error) {
	return term.MakeRaw(int(os.Stdin.Fd()))
}

func (stdTerminal) Width() int {
	cols, _, err := term.Size(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return cols
}

func (stdTerminal) Signals(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, syscall.SIGHUP)
}
