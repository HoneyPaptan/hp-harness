//go:build !linux && !darwin

package term

import "errors"

func IsTerminal(fd int) bool {
	return false
}

func MakeRaw(fd int) (func() error, error) {
	return nil, errors.ErrUnsupported
}

func Size(fd int) (int, int, error) {
	return 0, 0, errors.ErrUnsupported
}
