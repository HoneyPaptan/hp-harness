//go:build linux || darwin

package term

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

type fakeTTY struct {
	state    syscall.Termios
	size     winsize
	sets     int
	setState []syscall.Termios
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
		f.setState = append(f.setState, f.state)
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

func TestReadSecret(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strips newline", "s3cret\n", "s3cret"},
		{"strips carriage return", "s3cret\r\n", "s3cret"},
		{"keeps spaces", " a b \n", " a b "},
		{"empty line", "\n", ""},
		{"eof without newline", "tail", "tail"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if _, err := w.WriteString(tc.input); err != nil {
				t.Fatal(err)
			}
			w.Close()
			f := &fakeTTY{state: cookedState()}
			useFake(t, f)
			got, err := ReadSecret(int(r.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			if len(f.setState) != 2 {
				t.Fatalf("sets %d", len(f.setState))
			}
			quiet := f.setState[0]
			if quiet.Lflag&syscall.ECHO != 0 {
				t.Fatal("ECHO not cleared")
			}
			if quiet.Lflag&syscall.ICANON == 0 || quiet.Lflag&syscall.ISIG == 0 {
				t.Fatalf("unrelated Lflag cleared: %#x", quiet.Lflag)
			}
			if quiet.Iflag != cookedState().Iflag || quiet.Oflag != cookedState().Oflag {
				t.Fatal("input or output flags changed")
			}
			if f.state != cookedState() {
				t.Fatal("state not restored")
			}
		})
	}
}

func TestReadSecretErrors(t *testing.T) {
	tests := []struct {
		name string
		tty  fakeTTY
	}{
		{"get fails", fakeTTY{failGet: true}},
		{"set fails", fakeTTY{failSets: map[int]bool{1: true}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			w.Close()
			useFake(t, &tc.tty)
			got, err := ReadSecret(int(r.Fd()))
			if !errors.Is(err, errFake) || got != "" {
				t.Fatalf("got %q %v", got, err)
			}
		})
	}
}
