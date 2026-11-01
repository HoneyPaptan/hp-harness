//go:build !unix

package main

import "errors"

func hostLock(path string) (func(), error) {
	return func() {}, errors.New("locking is not supported on this platform")
}
