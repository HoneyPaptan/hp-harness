package cli

import (
	"context"
	"io"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type Env struct {
	Args        []string
	Getenv      func(string) string
	Cwd         string
	Stdout      io.Writer
	Stderr      io.Writer
	Host        host.Host
	Rand        io.Reader
	Version     string
	NewProvider func(Endpoint) provider.Provider
	NewJev      func(getenv func(string) string) (*jev.Client, error)
	LoginEnv    func(dir string) func() []string
	Spawn       func(args []string, logPath string) (int, error)
	Alive       func(pid int) bool
	ProcStart   func(pid int) string
	Signal      func(pid int) error
	Lock        func(dir string) (func(), error)
	Exe         string
	Os          string
	Pid         int
	Term        Terminal
	RunCtl      func(argv []string) error
	realGetenv  func(string) string
}

type Terminal interface {
	io.Reader
	io.Writer
	Interactive() bool
	MakeRaw() (func() error, error)
	Width() int
	Signals(ctx context.Context) (context.Context, context.CancelFunc)
	ReadSecret() (string, error)
}
