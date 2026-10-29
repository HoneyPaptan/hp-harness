package core

import (
	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

func Seed(d Deps, c Config) error {
	start := runStartFromConfig(c)
	w, err := journal.Create(d.Host, c.JournalPath, start)
	if err != nil {
		return err
	}
	return w.User(c.Prompt)
}
