package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/mcp/mcptest"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestMain(m *testing.M) {
	if mcptest.Active() {
		mcptest.Run()
		return
	}
	os.Exit(m.Run())
}

func fakeConfig(t *testing.T, extra ...string) (Config, string) {
	t.Helper()
	markPath := filepath.Join(t.TempDir(), "marks")
	env := []string{"PATH=" + os.Getenv("PATH"), mcptest.EnvMode + "=1", mcptest.EnvMark + "=" + markPath}
	return Config{
		Name: "fake", Command: os.Args[0], Env: append(env, extra...), Timeout: 5 * time.Second,
	}, markPath
}

func startCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "start")
}

func TestToolsPaginatesAcrossPages(t *testing.T) {
	cfg, _ := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	got, err := c.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 || got[0].Name != "echo" || got[9].Name != "first.crash" {
		t.Fatalf("tools %+v", got)
	}
	if !json.Valid(got[0].Schema) {
		t.Fatalf("schema %s", got[0].Schema)
	}
}

func TestNewStartsNothing(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	c.Close()
	if n := startCount(t, mark); n != 0 {
		t.Fatalf("starts %d", n)
	}
}

func TestCallResults(t *testing.T) {
	cfg, _ := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	tests := []struct {
		name, tool, args string
		want             string
		isError          bool
	}{
		{name: "pretty args stay on one line", tool: "echo", args: "{\n  \"text\": \"hi\"\n}", want: `echo:{"text":"hi"}`},
		{name: "empty args become an object", tool: "echo", want: "echo:{}"},
		{name: "null args become an object", tool: "echo", args: "null", want: "echo:{}"},
		{name: "isError maps through", tool: "fail", args: "{}", want: "tool said no", isError: true},
		{name: "non text is summarised", tool: "image", args: "{}", want: "before\n[image image/png, 6 bytes]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := c.Call(context.Background(), tt.tool, json.RawMessage(tt.args))
			if err != nil {
				t.Fatal(err)
			}
			if res.Text != tt.want || res.IsError != tt.isError {
				t.Fatalf("got %+v", res)
			}
		})
	}
}

func TestCallRejectsBadArgs(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	if _, err := c.Call(context.Background(), "echo", json.RawMessage("{nope")); err == nil {
		t.Fatal("want error")
	}
	if n := startCount(t, mark); n != 0 {
		t.Fatalf("starts %d", n)
	}
}

func TestOutputIsCapped(t *testing.T) {
	cfg, _ := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	res, err := c.Call(context.Background(), "big", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Text) > tool.MaxOutput || !strings.Contains(res.Text, "truncated") {
		t.Fatalf("len %d", len(res.Text))
	}
}

func TestHungServerIsKilledAndRestarted(t *testing.T) {
	cfg, mark := fakeConfig(t)
	cfg.Timeout = 300 * time.Millisecond
	c := New(cfg)
	defer c.Close()
	_, err := c.Call(context.Background(), mcptest.HangOnce, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out after 300ms") {
		t.Fatalf("err %v", err)
	}
	data, _ := os.ReadFile(mark)
	pid, _ := strconv.Atoi(strings.Fields(string(data))[1])
	if syscall.Kill(pid, 0) == nil && !isZombie(pid) {
		t.Fatalf("hung server %d survived", pid)
	}
	began := time.Now()
	res, err := c.Call(context.Background(), mcptest.HangOnce, nil)
	if err != nil || res.Text != "recovered" {
		t.Fatalf("restart %+v %v", res, err)
	}
	if time.Since(began) >= cfg.Timeout {
		t.Fatal("next call hung again")
	}
	if n := startCount(t, mark); n != 2 {
		t.Fatalf("starts %d", n)
	}
}

func TestTimeoutKillsTheGroup(t *testing.T) {
	cfg, _ := fakeConfig(t)
	cfg.Timeout = 2 * time.Second
	c := New(cfg)
	defer c.Close()
	res, err := c.Call(context.Background(), "group", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, _ := strconv.Atoi(res.Text)
	c.cfg.Timeout = 100 * time.Millisecond
	if _, err := c.Call(context.Background(), "hang", nil); err == nil {
		t.Fatal("want timeout")
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(child, 0) == nil && !isZombie(child) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d survived the timeout", child)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRepeatedHangsStopBounded(t *testing.T) {
	cfg, mark := fakeConfig(t)
	cfg.Timeout = 100 * time.Millisecond
	c := New(cfg)
	defer c.Close()
	for range maxHangs {
		_, err := c.Call(context.Background(), "hang", nil)
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("err %v", err)
		}
	}
	for range 2 {
		_, err := c.Call(context.Background(), "echo", nil)
		if err == nil || !strings.Contains(err.Error(), "not restarted") {
			t.Fatalf("err %v", err)
		}
	}
	if n := startCount(t, mark); n != maxHangs {
		t.Fatalf("starts %d", n)
	}
}

func TestSuccessClearsTheHangStreak(t *testing.T) {
	cfg, mark := fakeConfig(t)
	cfg.Timeout = 200 * time.Millisecond
	c := New(cfg)
	defer c.Close()
	for range 4 {
		if _, err := c.Call(context.Background(), "hang", nil); err == nil {
			t.Fatal("want timeout")
		}
		if _, err := c.Call(context.Background(), "echo", nil); err != nil {
			t.Fatal(err)
		}
	}
	if n := startCount(t, mark); n != 5 {
		t.Fatalf("starts %d", n)
	}
}

func TestHangsCountAsOneCrash(t *testing.T) {
	cfg, mark := fakeConfig(t)
	cfg.Timeout = 200 * time.Millisecond
	c := New(cfg)
	defer c.Close()
	for range 2 {
		if _, err := c.Call(context.Background(), "hang", nil); err == nil {
			t.Fatal("want timeout")
		}
	}
	if _, err := c.Call(context.Background(), "crash", nil); err == nil {
		t.Fatal("want crash")
	}
	_, err := c.Call(context.Background(), "echo", nil)
	if err == nil || !strings.Contains(err.Error(), "not restarted") {
		t.Fatalf("err %v", err)
	}
	if n := startCount(t, mark); n != 3 {
		t.Fatalf("starts %d", n)
	}
}

func TestHTTPTimeoutKeepsTheSession(t *testing.T) {
	block := make(chan struct{})
	f := &fakeHTTP{session: "s"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if bytes.Contains(body, []byte(`"hang"`)) {
			<-block
			return
		}
		f.ServeHTTP(w, r)
	}))
	defer srv.Close()
	defer close(block)
	c := New(Config{Name: "web", URL: srv.URL, Timeout: 200 * time.Millisecond})
	defer c.Close()
	if _, err := c.Call(context.Background(), "hang", nil); err == nil {
		t.Fatal("want timeout")
	}
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inits != 1 {
		t.Fatalf("inits %d", f.inits)
	}
}

func TestContextCancelsCallButNotServer(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	if _, err := c.Call(ctx, "hang", nil); err == nil {
		t.Fatal("want cancel error")
	}
	if !c.sess.alive() || startCount(t, mark) != 1 {
		t.Fatal("a cancelled call must not stop the shared server")
	}
}

func TestStartContextDoesNotOwnTheProcess(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := c.Call(ctx, "echo", nil); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	if n := startCount(t, mark); n != 1 {
		t.Fatalf("starts %d", n)
	}
}

func TestCrashErrorsThenRestartsOnce(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	_, err := c.Call(context.Background(), "crash", nil)
	if err == nil || !strings.Contains(err.Error(), "fake crash") {
		t.Fatalf("err %v", err)
	}
	res, err := c.Call(context.Background(), "echo", nil)
	if err != nil || res.Text != "echo:{}" {
		t.Fatalf("restart %+v %v", res, err)
	}
	if n := startCount(t, mark); n != 2 {
		t.Fatalf("starts %d", n)
	}
}

func TestCrashLoopStopsRestarting(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	for range 2 {
		if _, err := c.Call(context.Background(), "crash", nil); err == nil {
			t.Fatal("want crash error")
		}
	}
	_, err := c.Call(context.Background(), "echo", nil)
	if err == nil || !strings.Contains(err.Error(), "not restarted") {
		t.Fatalf("err %v", err)
	}
	if n := startCount(t, mark); n != 2 {
		t.Fatalf("starts %d", n)
	}
}

func TestSuccessAfterRestartRestoresTheBudget(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	for range 3 {
		if _, err := c.Call(context.Background(), "crash", nil); err == nil {
			t.Fatal("want crash error")
		}
		if _, err := c.Call(context.Background(), "echo", nil); err != nil {
			t.Fatal(err)
		}
	}
	if n := startCount(t, mark); n != 4 {
		t.Fatalf("starts %d", n)
	}
}

func TestFailedStartReportsStderrAndRetriesOnce(t *testing.T) {
	cfg, mark := fakeConfig(t, mcptest.EnvFail+"=token sk-secret rejected")
	cfg.Secrets = []string{"sk-secret"}
	c := New(cfg)
	defer c.Close()
	_, err := c.Call(context.Background(), "echo", nil)
	if err == nil || !strings.Contains(err.Error(), "token [redacted] rejected") {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("secret leaked: %v", err)
	}
	for range 3 {
		if _, err := c.Call(context.Background(), "echo", nil); err == nil {
			t.Fatal("want error")
		}
	}
	if n := startCount(t, mark); n != 2 {
		t.Fatalf("starts %d", n)
	}
}

func TestMissingCommandIsAnError(t *testing.T) {
	c := New(Config{Name: "gone", Command: "/nonexistent/hp-mcp", Timeout: time.Second})
	defer c.Close()
	if _, err := c.Tools(context.Background()); err == nil {
		t.Fatal("want error")
	}
}

func TestParallelCallsShareOneProcess(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			args := json.RawMessage(`{"n":` + strconv.Itoa(i) + `}`)
			res, err := c.Call(context.Background(), "echo", args)
			if err == nil && res.Text != "echo:"+string(args) {
				err = errors.New("crossed reply: " + res.Text)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := startCount(t, mark); n != 1 {
		t.Fatalf("starts %d", n)
	}
}

func TestCloseKillsTheProcessGroup(t *testing.T) {
	cfg, _ := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	res, err := c.Call(context.Background(), "group", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := strconv.Atoi(res.Text)
	if err != nil {
		t.Fatalf("pid %q", res.Text)
	}
	c.Close()
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(child, 0) == nil && !isZombie(child) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d survived close", child)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func isZombie(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	rest := string(data)
	return strings.Contains(rest[strings.LastIndex(rest, ")"):], ") Z")
}

func TestCloseAllowsAFreshStart(t *testing.T) {
	cfg, mark := fakeConfig(t)
	c := New(cfg)
	defer c.Close()
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	c.Close()
	if _, err := c.Call(context.Background(), "echo", nil); err != nil {
		t.Fatal(err)
	}
	if n := startCount(t, mark); n != 2 {
		t.Fatalf("starts %d", n)
	}
}

func TestTailKeepsTheEnd(t *testing.T) {
	var b tailBuffer
	b.Write([]byte(strings.Repeat("a", maxTail)))
	b.Write([]byte("end"))
	got := b.String()
	if len(got) != maxTail || !strings.HasSuffix(got, "end") {
		t.Fatalf("len %d", len(got))
	}
}
