//go:build !linux && !darwin

package term

import "os"

type Input struct{}

func NewInput(int) *Input {
	return &Input{}
}

func (*Input) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

func (*Input) Pause() func() {
	return func() {}
}
