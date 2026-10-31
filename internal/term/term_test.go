package term

import (
	"os"
	"testing"
)

func pipeFd(t *testing.T) int {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	return int(r.Fd())
}

func TestPipeIsNotATerminal(t *testing.T) {
	fd := pipeFd(t)
	if IsTerminal(fd) {
		t.Fatal("pipe reported as terminal")
	}
	restore, err := MakeRaw(fd)
	if err == nil || restore != nil {
		t.Fatalf("MakeRaw on pipe: restore set %v, err %v", restore != nil, err)
	}
	if _, _, err := Size(fd); err == nil {
		t.Fatal("Size on pipe: want error")
	}
}
