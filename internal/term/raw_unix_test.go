//go:build unix

package term

import (
	"syscall"
	"testing"
)

func TestRawStateKeepsEnterApartFromLineFeed(t *testing.T) {
	var cooked syscall.Termios
	cooked.Iflag = syscall.ICRNL | syscall.INLCR | syscall.IGNCR
	cooked.Lflag = syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	raw := rawState(cooked)
	if raw.Iflag&(syscall.ICRNL|syscall.INLCR|syscall.IGNCR) != 0 {
		t.Fatalf("input flags %#x still translate carriage returns", raw.Iflag)
	}
	if raw.Lflag&(syscall.ICANON|syscall.ECHO|syscall.ISIG|syscall.IEXTEN) != 0 {
		t.Fatalf("local flags %#x still cook the input", raw.Lflag)
	}
}
