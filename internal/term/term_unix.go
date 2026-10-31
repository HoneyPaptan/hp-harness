//go:build linux || darwin

package term

import (
	"sync"
	"syscall"
	"unsafe"
)

type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

var ioctl = func(fd int, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func IsTerminal(fd int) bool {
	var state syscall.Termios
	return ioctl(fd, getTermios, unsafe.Pointer(&state)) == nil
}

func MakeRaw(fd int) (func() error, error) {
	var saved syscall.Termios
	if err := ioctl(fd, getTermios, unsafe.Pointer(&saved)); err != nil {
		return nil, err
	}
	raw := rawState(saved)
	if err := ioctl(fd, setTermios, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return restorer(fd, saved), nil
}

func Size(fd int) (int, int, error) {
	var size winsize
	if err := ioctl(fd, syscall.TIOCGWINSZ, unsafe.Pointer(&size)); err != nil {
		return 0, 0, err
	}
	return int(size.cols), int(size.rows), nil
}

func rawState(state syscall.Termios) syscall.Termios {
	state.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	state.Cc[syscall.VMIN] = 1
	state.Cc[syscall.VTIME] = 0
	return state
}

func restorer(fd int, saved syscall.Termios) func() error {
	var mu sync.Mutex
	restored := false
	return func() error {
		mu.Lock()
		defer mu.Unlock()
		if restored {
			return nil
		}
		if err := ioctl(fd, setTermios, unsafe.Pointer(&saved)); err != nil {
			return err
		}
		restored = true
		return nil
	}
}
