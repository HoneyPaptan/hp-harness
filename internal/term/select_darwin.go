//go:build darwin

package term

import "syscall"

func selectReadable(nfd int, set *syscall.FdSet) error {
	return syscall.Select(nfd, set, nil, nil, nil)
}
