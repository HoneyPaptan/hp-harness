//go:build linux || darwin

package term

import (
	"errors"
	"syscall"
	"testing"
	"unsafe"
)

type fakeTTY struct {
	state    syscall.Termios
	size     winsize
	sets     int
	failGet  bool
	failSets map[int]bool
}

var errFake = errors.New("ioctl failed")

func (f *fakeTTY) ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	switch req {
	case getTermios:
		if f.failGet {
			return errFake
		}
		*(*syscall.Termios)(arg) = f.state
	case setTermios:
		f.sets++
		if f.failSets[f.sets] {
			return errFake
		}
		f.state = *(*syscall.Termios)(arg)
	case syscall.TIOCGWINSZ:
		if f.failGet {
			return errFake
		}
		*(*winsize)(arg) = f.size
	}
	return nil
}

func useFake(t *testing.T, f *fakeTTY) {
	t.Helper()
	real := ioctl
	ioctl = f.ioctl
	t.Cleanup(func() { ioctl = real })
}

func cookedState() syscall.Termios {
	var s syscall.Termios
	s.Lflag = syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN | syscall.ECHOE
	s.Oflag = syscall.OPOST
	s.Iflag = syscall.ICRNL
	s.Cc[syscall.VMIN] = 4
	s.Cc[syscall.VTIME] = 9
	return s
}

func TestMakeRawFlags(t *testing.T) {
	f := &fakeTTY{state: cookedState()}
	useFake(t, f)
	if _, err := MakeRaw(0); err != nil {
		t.Fatal(err)
	}
	const cleared = syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	if f.state.Lflag&cleared != 0 {
		t.Fatalf("Lflag still has raw bits: %#x", f.state.Lflag)
	}
	if f.state.Lflag&syscall.ECHOE == 0 {
		t.Fatal("unrelated Lflag bit cleared")
	}
	if f.state.Oflag&syscall.OPOST == 0 {
		t.Fatal("OPOST cleared")
	}
	if f.state.Cc[syscall.VMIN] != 1 || f.state.Cc[syscall.VTIME] != 0 {
		t.Fatalf("VMIN %d VTIME %d", f.state.Cc[syscall.VMIN], f.state.Cc[syscall.VTIME])
	}
}

func TestRestore(t *testing.T) {
	tests := []struct {
		name     string
		failSets map[int]bool
		errs     []bool
		wantSets int
	}{
		{"second call does nothing", nil, []bool{false, false}, 2},
		{"third call does nothing", nil, []bool{false, false, false}, 2},
		{"failure can be retried", map[int]bool{2: true}, []bool{true, false, false}, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeTTY{state: cookedState(), failSets: tc.failSets}
			useFake(t, f)
			restore, err := MakeRaw(0)
			if err != nil {
				t.Fatal(err)
			}
			for i, wantErr := range tc.errs {
				if err := restore(); (err != nil) != wantErr {
					t.Fatalf("call %d: err %v, want error %v", i, err, wantErr)
				}
			}
			if f.sets != tc.wantSets {
				t.Fatalf("sets %d want %d", f.sets, tc.wantSets)
			}
			if f.state != cookedState() {
				t.Fatal("state not restored")
			}
		})
	}
}

func TestMakeRawErrors(t *testing.T) {
	tests := []struct {
		name string
		tty  fakeTTY
	}{
		{"get fails", fakeTTY{failGet: true}},
		{"set fails", fakeTTY{failSets: map[int]bool{1: true}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useFake(t, &tc.tty)
			restore, err := MakeRaw(0)
			if !errors.Is(err, errFake) || restore != nil {
				t.Fatalf("err %v, restore set %v", err, restore != nil)
			}
		})
	}
}

func TestSize(t *testing.T) {
	tests := []struct {
		name       string
		tty        fakeTTY
		cols, rows int
		wantErr    bool
	}{
		{"reports", fakeTTY{size: winsize{rows: 24, cols: 80}}, 80, 24, false},
		{"error", fakeTTY{failGet: true}, 0, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useFake(t, &tc.tty)
			cols, rows, err := Size(0)
			if (err != nil) != tc.wantErr || cols != tc.cols || rows != tc.rows {
				t.Fatalf("got %d %d %v", cols, rows, err)
			}
		})
	}
}
