//go:build linux || darwin

package term

import (
	"io"
	"sync"
	"syscall"
	"unsafe"
)

type Input struct {
	fd    int
	once  sync.Once
	wakeR int
	wakeW int
	ready error
	mu    sync.Mutex
	state *sync.Cond
	held  bool
	idle  bool
}

func NewInput(fd int) *Input {
	in := &Input{fd: fd, idle: true}
	in.state = sync.NewCond(&in.mu)
	return in
}

func (in *Input) open() error {
	in.once.Do(func() {
		var fds [2]int
		if in.ready = syscall.Pipe(fds[:]); in.ready != nil {
			return
		}
		in.wakeR, in.wakeW = fds[0], fds[1]
		for _, fd := range fds {
			if in.ready = syscall.SetNonblock(fd, true); in.ready != nil {
				return
			}
		}
	})
	return in.ready
}

func (in *Input) Read(p []byte) (int, error) {
	if err := in.open(); err != nil {
		return 0, err
	}
	for {
		in.mu.Lock()
		for in.held {
			in.state.Wait()
		}
		in.idle = false
		in.mu.Unlock()
		n, retry, err := in.readOnce(p)
		in.mu.Lock()
		in.idle = true
		in.state.Broadcast()
		in.mu.Unlock()
		if !retry {
			return n, err
		}
	}
}

func (in *Input) readOnce(p []byte) (int, bool, error) {
	var set syscall.FdSet
	setFd(&set, in.fd)
	setFd(&set, in.wakeR)
	if err := selectReadable(max(in.fd, in.wakeR)+1, &set); err != nil {
		if err == syscall.EINTR {
			return 0, true, nil
		}
		return 0, false, err
	}
	if isSet(&set, in.wakeR) {
		in.drain()
		return 0, true, nil
	}
	n, err := syscall.Read(in.fd, p)
	switch {
	case err == syscall.EINTR || err == syscall.EAGAIN:
		return 0, true, nil
	case err != nil:
		return 0, false, err
	case n == 0:
		return 0, false, io.EOF
	}
	return n, false, nil
}

func (in *Input) drain() {
	var buf [64]byte
	for {
		if n, err := syscall.Read(in.wakeR, buf[:]); n <= 0 || err != nil {
			return
		}
	}
}

func (in *Input) Pause() func() {
	if in.open() != nil {
		return func() {}
	}
	in.mu.Lock()
	in.held = true
	if !in.idle {
		_, _ = syscall.Write(in.wakeW, []byte{1})
	}
	for !in.idle {
		in.state.Wait()
	}
	in.mu.Unlock()
	return func() {
		in.mu.Lock()
		in.held = false
		in.state.Broadcast()
		in.mu.Unlock()
	}
}

func fdWord(set *syscall.FdSet, fd int) (*syscall.FdSet, int, uint) {
	width := int(unsafe.Sizeof(set.Bits[0])) * 8
	return set, fd / width, uint(fd % width)
}

func setFd(set *syscall.FdSet, fd int) {
	_, i, bit := fdWord(set, fd)
	set.Bits[i] |= 1 << bit
}

func isSet(set *syscall.FdSet, fd int) bool {
	_, i, bit := fdWord(set, fd)
	return set.Bits[i]&(1<<bit) != 0
}
