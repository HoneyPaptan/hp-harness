package cli

import (
	"io"

	"github.com/HoneyPaptan/hp-harness/internal/host"
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
}
