package core

import (
	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

func Seed(d Deps, c Config) error {
	_, err := seed(d, c)
	return err
}

func seed(d Deps, c Config) (*journal.Writer, error) {
	w, err := journal.Create(d.Host, c.JournalPath, runStartFromConfig(c))
	if err != nil {
		return nil, err
	}
	return w, w.User(c.Prompt)
}
