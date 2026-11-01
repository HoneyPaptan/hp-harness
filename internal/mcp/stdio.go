package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	drainDelay = 200 * time.Millisecond
	stderrWait = time.Second
)

type reply struct {
	raw json.RawMessage
	err error
}

type stdioConn struct {
	cmd     *exec.Cmd
	stdin   *os.File
	stdout  *os.File
	stderr  tailBuffer
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[int64]chan reply
	failure error
	done    chan struct{}
	exited  chan struct{}
	waitErr error
}

func startStdio(cfg Config) (*stdioConn, error) {
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer inRead.Close()
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		inWrite.Close()
		return nil, err
	}
	defer outWrite.Close()
	c := newStdioConn(cfg, inRead, inWrite, outRead, outWrite)
	if err := c.cmd.Start(); err != nil {
		inWrite.Close()
		outRead.Close()
		return nil, fmt.Errorf("start %s: %w", cfg.Command, err)
	}
	go c.wait()
	go c.read()
	return c, nil
}

func newStdioConn(cfg Config, inRead, inWrite, outRead, outWrite *os.File) *stdioConn {
	c := &stdioConn{
		stdin: inWrite, stdout: outRead,
		pending: make(map[int64]chan reply),
		done:    make(chan struct{}), exited: make(chan struct{}),
	}
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Dir = cfg.Dir
	cmd.Env = append([]string{}, cfg.Env...)
	cmd.Stdin = inRead
	cmd.Stdout = outWrite
	cmd.Stderr = &c.stderr
	cmd.WaitDelay = stderrWait
	setProcessGroup(cmd)
	c.cmd = cmd
	return c
}

func (c *stdioConn) wait() {
	c.waitErr = c.cmd.Wait()
	close(c.exited)
	time.AfterFunc(drainDelay, func() { c.stdout.Close() })
}

func (c *stdioConn) read() {
	sc := bufio.NewScanner(c.stdout)
	sc.Buffer(make([]byte, 0, 64<<10), maxMessage)
	for sc.Scan() {
		c.dispatch(sc.Bytes())
	}
	failure := scanFailure(sc.Err())
	killProcessGroup(c.cmd)
	<-c.exited
	c.fail(c.exitError(failure))
}

func scanFailure(err error) error {
	if errors.Is(err, bufio.ErrTooLong) {
		return fmt.Errorf("server message over %d bytes", maxMessage)
	}
	return nil
}

func (c *stdioConn) exitError(failure error) error {
	if failure != nil {
		return failure
	}
	if c.waitErr != nil {
		return fmt.Errorf("server exited: %w", c.waitErr)
	}
	return errors.New("server exited")
}

func (c *stdioConn) dispatch(line []byte) {
	var m message
	if json.Unmarshal(line, &m) != nil {
		return
	}
	if m.isServerRequest() {
		go c.answer(m)
		return
	}
	if m.isResponse() {
		c.deliver(m)
	}
}

func (c *stdioConn) answer(m message) {
	payload, err := replyTo(m)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cancelTimeout)
	defer cancel()
	_ = c.write(ctx, payload)
}

func (c *stdioConn) deliver(m message) {
	id, ok := responseID(m.ID)
	if !ok {
		return
	}
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		raw, err := m.outcome()
		ch <- reply{raw: raw, err: err}
	}
}

func (c *stdioConn) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failure = err
	close(c.done)
}

func (c *stdioConn) register(id int64) chan reply {
	ch := make(chan reply, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	return ch
}

func (c *stdioConn) unregister(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *stdioConn) call(ctx context.Context, id int64, payload []byte) (json.RawMessage, error) {
	ch := c.register(id)
	defer c.unregister(id)
	if err := c.write(ctx, payload); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		return r.raw, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return c.resultAfterExit(ch)
	}
}

func (c *stdioConn) resultAfterExit(ch chan reply) (json.RawMessage, error) {
	select {
	case r := <-ch:
		return r.raw, r.err
	default:
		return nil, c.deadError()
	}
}

func (c *stdioConn) deadError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failure
}

func (c *stdioConn) send(ctx context.Context, payload []byte) error {
	return c.write(ctx, payload)
}

func (c *stdioConn) write(ctx context.Context, payload []byte) error {
	errc := make(chan error, 1)
	go func() { errc <- c.writeLine(payload) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.deadError()
	}
}

func (c *stdioConn) writeLine(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := io.WriteString(c.stdin, string(payload)+"\n")
	return err
}

func (c *stdioConn) setVersion(string) {}

func (c *stdioConn) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *stdioConn) tail() string {
	return c.stderr.String()
}

func (c *stdioConn) close() {
	c.stdin.Close()
	killProcessGroup(c.cmd)
	<-c.done
}
