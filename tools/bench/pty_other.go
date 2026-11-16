//go:build !linux

package main

import "errors"

func (h *harness) promptSamples(string, int) (timed, error) {
	return timed{}, errors.New("the pty timing needs linux")
}
