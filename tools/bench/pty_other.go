//go:build !linux

package main

import (
	"errors"
	"time"
)

func (h *harness) promptSamples(string, int) ([]time.Duration, error) {
	return nil, errors.New("the pty timing needs linux")
}
