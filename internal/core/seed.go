package core

import (
	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

func Seed(d Deps, c Config) error {
	_, err := seed(d, c)
	return err
}

func seed(d Deps, c Config) (*journal.Writer, error) {
	return journal.CreateWithUser(d.Host, c.JournalPath, runStartFromConfig(c), c.Prompt)
}
