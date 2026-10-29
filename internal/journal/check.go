package journal

import "errors"

var (
	ErrEmpty      = errors.New("empty journal")
	ErrNoRunStart = errors.New("missing run_start")
)

func checkStart(recs []Record) error {
	if len(recs) == 0 {
		return ErrEmpty
	}
	if recs[0].Type != TypeRunStart {
		return ErrNoRunStart
	}
	return nil
}
