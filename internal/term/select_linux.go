//go:build linux

package term

import "syscall"

func selectReadable(nfd int, set *syscall.FdSet) error {
	_, err := syscall.Select(nfd, set, nil, nil, nil)
	return err
}
