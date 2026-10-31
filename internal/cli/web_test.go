package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/web"
)

const (
	webWait   = 5 * time.Second
	webSecret = "sekret-key-value"
)

var webURL = regexp.MustCompile(`http://\S+/\?token=[0-9a-f]{64}`)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type gatedProvider struct {
	gate chan struct{}
	next provider.Provider
	once sync.Once
}

func (p *gatedProvider) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	first := false
	p.once.Do(func() { first = true })
	if !first {
		return p.next.Stream(ctx, req)
	}
	return func(yield func(provider.Event, error) bool) {
		if !yield(provider.Event{Kind: provider.EventText, Text: "partial"}, nil) {
			return
		}
		select {
		case <-p.gate:
		case <-ctx.Done():
			yield(provider.Event{}, ctx.Err())
		}
	}
}

type webSession struct {
	t      *testing.T
	base   string
	client *http.Client
	stderr *syncBuffer
	cancel context.CancelFunc
	code   chan int
}

func webTestEnv(t *testing.T, h *host.Memory, prov provider.Provider, extra map[string]string) (Env, *syncBuffer) {
	t.Helper()
	all := map[string]string{"GROQ_API_KEY": webSecret}
	for k, v := range extra {
		all[k] = v
	}
	env, _, _ := testEnv(t, h, nil, all)
	env.NewProvider = func(Endpoint) provider.Provider { return prov }
	env.Rand = rand.Reader
	stderr := &syncBuffer{}
	env.Stderr = stderr
	return env, stderr
}

func startWeb(t *testing.T, env Env, stderr *syncBuffer, args ...string) *webSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	env.Args = append([]string{"web"}, args...)
	s := &webSession{t: t, stderr: stderr, cancel: cancel, code: make(chan int, 1)}
	go func() { s.code <- Main(ctx, env) }()
	t.Cleanup(cancel)
	s.login(s.awaitURL())
	return s
}

func (s *webSession) awaitURL() string {
	s.t.Helper()
	deadline := time.Now().Add(webWait)
	for time.Now().Before(deadline) {
		if u := webURL.FindString(s.stderr.String()); u != "" {
			return u
		}
		time.Sleep(time.Millisecond)
	}
	s.t.Fatalf("no url in %q", s.stderr.String())
	return ""
}

func (s *webSession) login(url string) {
	s.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		s.t.Fatal(err)
	}
	s.client = &http.Client{Jar: jar}
	s.base = url[:strings.Index(url, "/?token=")]
	resp, err := s.client.Get(url)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Request.URL.RawQuery != "" {
		s.t.Fatalf("login: %d %s", resp.StatusCode, resp.Request.URL)
	}
}

func (s *webSession) get(path string) string {
	s.t.Helper()
	resp, err := s.client.Get(s.base + path)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return string(data)
}

func (s *webSession) post(path, body string) (int, string) {
	s.t.Helper()
	resp, err := s.client.Post(s.base+path, "application/json", strings.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return resp.StatusCode, string(data)
}

func (s *webSession) start(prompt string) string {
	s.t.Helper()
	code, body := s.post("/api/runs", `{"prompt":`+strconv.Quote(prompt)+`}`)
	if code != http.StatusCreated {
		s.t.Fatalf("start %d %s", code, body)
	}
	var out struct{ ID string }
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		s.t.Fatal(err)
	}
	return out.ID
}

func (s *webSession) awaitStatus(id, status string, count int) []web.Event {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), webWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/api/runs/"+id+"/events", nil)
	if err != nil {
		s.t.Fatal(err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []web.Event
	seen := 0
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev web.Event
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			s.t.Fatal(err)
		}
		out = append(out, ev)
		if ev.Kind == web.KindRunStatus && ev.Status == status {
			seen++
		}
		if seen == count {
			return out
		}
	}
	s.t.Fatalf("stream ended, want %s: %+v %v", status, out, sc.Err())
	return nil
}

func (s *webSession) shutdown() int {
	s.t.Helper()
	s.cancel()
	select {
	case code := <-s.code:
		return code
	case <-time.After(webWait):
		s.t.Fatal("server did not stop")
	}
	return -1
}

func TestWebFlagErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "prompt", args: []string{"hello"}, want: "no prompt"},
		{name: "detach", args: []string{"-d"}, want: "cannot detach"},
		{name: "port and listen", args: []string{"--port", "1", "--listen", "127.0.0.1:0"}, want: "not both"},
		{name: "unknown preset", args: []string{"-m", "nope/x"}, want: "unknown preset"},
		{name: "missing key", args: []string{"-m", "openrouter/x"}, want: "OPENROUTER_API_KEY"},
		{name: "jev without key", args: []string{"--jev"}, want: "jev"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, stderr := webTestEnv(t, host.NewMemory(), nil, nil)
			env.NewJev = func(func(string) string) (*jev.Client, error) { return nil, fmt.Errorf("no jev key") }
			env.Args = append([]string{"web"}, c.args...)
			if code := Main(context.Background(), env); code != 2 {
				t.Fatalf("code %d", code)
			}
			if !strings.Contains(stderr.String(), c.want) {
				t.Fatalf("%q", stderr.String())
			}
		})
	}
}

func TestWebRunLifecycle(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("hello from the model"), fake.TextTurn("again")}}
	env, stderr := webTestEnv(t, h, fp, nil)
	s := startWeb(t, env, stderr, "-m", "groq/m", "-C", "/work", "--listen", "127.0.0.1:0")

	state := s.get("/api/state")
	if strings.Contains(state, webSecret) || strings.Contains(state, "GROQ_API_KEY") {
		t.Fatalf("state leaks: %s", state)
	}
	var st web.State
	if err := json.Unmarshal([]byte(state), &st); err != nil {
		t.Fatal(err)
	}
	if st.Model != "groq/m" || st.Dir != "/work" || strings.Join(st.Presets, ",") != "groq,ollama,xai" {
		t.Fatalf("%+v", st)
	}

	long := strings.Repeat("é", 100)
	id := s.start(long)
	if !web.ValidRunID(id) {
		t.Fatalf("id %q", id)
	}
	got := s.awaitStatus(id, web.StatusFinished, 1)
	last := got[len(got)-2]
	if last.Kind != web.KindTurnEnd || last.Reason != "final" {
		t.Fatalf("%+v", got)
	}
	s.awaitRunGone(id)

	var runs []web.Run
	if err := json.Unmarshal([]byte(s.get("/api/runs")), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != id || runs[0].Status != web.StatusFinished || runs[0].Model != "groq/m" {
		t.Fatalf("%+v", runs)
	}
	if n := len([]rune(runs[0].Title)); n != 80 {
		t.Fatalf("title runes %d", n)
	}

	if code, body := s.post("/api/runs/"+id+"/model", `{"model":"xai/other"}`); code != 200 || !strings.Contains(body, "xai/other") {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := s.post("/api/runs/"+id+"/model", `{"model":"openrouter/x"}`); code != 400 {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := s.post("/api/runs/"+id+"/messages", `{"prompt":"more"}`); code != 202 {
		t.Fatalf("%d %s", code, body)
	}
	s.awaitStatus(id, web.StatusFinished, 2)
	if code := s.shutdown(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	state2, err := replayRun(env, jpath)
	if err != nil {
		t.Fatal(err)
	}
	if !state2.Finished || state2.Current != "xai/other" {
		t.Fatalf("%+v", state2)
	}
	if _, ok := readRunPid(env, id); ok {
		t.Fatal("pid file left behind")
	}
	if strings.Contains(strings.Replace(stderr.String(), webURL.FindString(stderr.String()), "", 1), "token") {
		t.Fatalf("token logged twice: %s", stderr.String())
	}
}

func (s *webSession) awaitRunGone(id string) {
	s.t.Helper()
	deadline := time.Now().Add(webWait)
	for time.Now().Before(deadline) {
		if strings.Contains(s.get("/api/runs"), `"status":"finished"`) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	s.t.Fatalf("run %s never settled", id)
}

func TestWebStopContinuesInterrupted(t *testing.T) {
	h := host.NewMemory()
	gp := &gatedProvider{gate: make(chan struct{}), next: &fake.Provider{Turns: []fake.Turn{fake.TextTurn("after")}}}
	env, stderr := webTestEnv(t, h, gp, nil)
	s := startWeb(t, env, stderr, "-m", "groq/m", "--listen", "127.0.0.1:0")
	id := s.start("first")
	if code, body := s.post("/api/runs/"+id+"/messages", `{"prompt":"x"}`); code != http.StatusConflict {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := s.post("/api/runs/"+id+"/stop", `{}`); code != 200 || !strings.Contains(body, "true") {
		t.Fatalf("%d %s", code, body)
	}
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	st, err := replayRun(env, jpath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Finished {
		t.Fatal("stop finished the run")
	}
	if code, body := s.post("/api/runs/"+id+"/messages", `{"prompt":"go on"}`); code != 202 {
		t.Fatalf("%d %s", code, body)
	}
	s.awaitStatus(id, web.StatusFinished, 1)
	if st, err = replayRun(env, jpath); err != nil || !st.Finished {
		t.Fatalf("%v %+v", err, st)
	}
}

func TestWebRefusesRunLiveElsewhere(t *testing.T) {
	h := host.NewMemory()
	env, stderr := webTestEnv(t, h, &fake.Provider{}, nil)
	env.Alive = func(pid int) bool { return pid == 4242 }
	id := "20260101-100000-aaaa"
	seedRun(t, h, env.Getenv, id, "elsewhere", "", false)
	if err := writeRunPidFor(env, id, 4242); err != nil {
		t.Fatal(err)
	}
	s := startWeb(t, env, stderr, "-m", "groq/m", "--listen", "127.0.0.1:0")
	for _, row := range []struct{ path, body string }{
		{"/messages", `{"prompt":"x"}`},
		{"/model", `{"model":"xai/n"}`},
		{"/stop", `{}`},
	} {
		if code, body := s.post("/api/runs/"+id+row.path, row.body); code != http.StatusConflict {
			t.Fatalf("%s %d %s", row.path, code, body)
		}
	}
	if !strings.Contains(s.get("/api/runs"), `"status":"running"`) {
		t.Fatal("not shown as running")
	}
	if pid, ok := readRunPid(env, id); !ok || pid != 4242 {
		t.Fatalf("pid changed: %d", pid)
	}
	var records []journal.Record
	if records, _ = journal.Read(h, filepath.Join(runsDir(env.Getenv), id+".jsonl")); len(records) != 2 {
		t.Fatalf("journal touched: %d", len(records))
	}
}

func TestWebPortTakenFallsToFreePort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	env, stderr := webTestEnv(t, host.NewMemory(), &fake.Provider{}, nil)
	s := startWeb(t, env, stderr, "--port", strconv.Itoa(port))
	if !strings.Contains(stderr.String(), fmt.Sprintf("port %d is taken", port)) {
		t.Fatalf("%q", stderr.String())
	}
	if strings.Contains(s.base, ":"+strconv.Itoa(port)) {
		t.Fatalf("base %s", s.base)
	}
	if code := s.shutdown(); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestWebListenWarnings(t *testing.T) {
	cases := []struct {
		name, listen string
		warn         bool
	}{
		{name: "loopback", listen: "127.0.0.1:0"},
		{name: "wildcard", listen: "0.0.0.0:0", warn: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, stderr := webTestEnv(t, host.NewMemory(), &fake.Provider{}, nil)
			s := startWeb(t, env, stderr, "--listen", c.listen)
			if strings.Contains(stderr.String(), "without TLS") != c.warn {
				t.Fatalf("%q", stderr.String())
			}
			if !strings.HasPrefix(s.base, "http://127.0.0.1:") {
				t.Fatalf("base %s", s.base)
			}
			s.shutdown()
		})
	}
}

func TestWebHosts(t *testing.T) {
	got := strings.Join(webHosts("10.0.0.2:80", "box", "80"), " ")
	for _, want := range []string{"10.0.0.2:80", "box:80", "localhost:80", "127.0.0.1:80"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%s missing %s", got, want)
		}
	}
}

func TestNewRunIDMatchesWebShape(t *testing.T) {
	id, err := newRunID(time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !web.ValidRunID(id) {
		t.Fatalf("%q", id)
	}
}
