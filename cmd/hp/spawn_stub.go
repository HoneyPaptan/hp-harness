//go:build !unix

package main

import "errors"

func spawnDetached(args []string, logPath string) (int, error) {
	return 0, errors.New("detach is not supported on this platform")
}

func processAlive(pid int) bool {
	return false
}

func processSignal(pid int) error {
	return errors.New("detach is not supported on this platform")
}
