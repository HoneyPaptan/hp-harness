package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const (
	testToken = "test-token"
	testHost  = "127.0.0.1:7878"
	testWait  = 5 * time.Second
)

type step struct {
	text string
	gate <-chan struct{}
	err  error
	call provider.ToolCall
}

type scripted struct {
	mu    sync.Mutex
	turns [][]step
}

func (p *scripted) take() ([]step, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.turns) == 0 {
		return nil, false
	}
	t := p.turns[0]
	p.turns = p.turns[1:]
	return t, true
}

func (p *scripted) Stream(ctx context.Context, _ provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		turn, ok := p.take()
		if !ok {
			yield(provider.Event{}, fmt.Errorf("script exhausted"))
			return
		}
		for _, s := range turn {
			ev, err := playStep(ctx, s)
			if !yield(ev, err) || err != nil {
				return
			}
		}
		yield(provider.Event{Kind: provider.EventDone, StopReason: "stop"}, nil)
	}
}

func playStep(ctx context.Context, s step) (provider.Event, error) {
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return provider.Event{}, ctx.Err()
		}
	}
	if s.err != nil {
		return provider.Event{}, s.err
	}
	if s.call.ID != "" {
		return provider.Event{Kind: provider.EventToolCall, Call: s.call}, nil
	}
	return provider.Event{Kind: provider.EventText, Text: s.text}, nil
}

type testBackend struct {
	preconnects atomic.Int32
	h           *host.Memory
	prov        provider.Provider
	tools       *tool.Registry
	mu          sync.Mutex
	next        int
	started     int
	foreign     map[string]bool
}

func newBackend(prov provider.Provider, tools ...tool.Tool) *testBackend {
	return &testBackend{h: host.NewMemory(), prov: prov, tools: tool.NewRegistry(tools...), foreign: make(map[string]bool)}
}

func journalFile(id string) string { return "/runs/" + id + ".jsonl" }

func (b *testBackend) State() State {
	return State{Model: "fake/m", Presets: []string{"fake"}, Dir: "/w"}
}

func (b *testBackend) Preconnect() {
	b.preconnects.Add(1)
}

func (b *testBackend) Runs() ([]Run, error) {
	return []Run{{ID: "20260101-120000-0001", Status: StatusFinished}}, nil
}

func (b *testBackend) Records(id string) ([]journal.Record, error) {
	return journal.Read(b.h, journalFile(id))
}

func (b *testBackend) setForeign(id string, on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.foreign[id] = on
}

func (b *testBackend) Busy(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.foreign[id]
}

func (b *testBackend) Start(model, prompt string) (Agent, error) {
	if model == "bad/model" {
		return nil, fmt.Errorf("%w: unknown preset", ErrInvalid)
	}
	b.mu.Lock()
	b.next++
	b.started++
	id := fmt.Sprintf("20260101-120000-%04x", b.next)
	b.mu.Unlock()
	cfg := core.Config{RunID: id, Model: "m", Preset: "fake", Dir: "/w", JournalPath: journalFile(id), Prompt: prompt}
	if err := core.Seed(core.Deps{Host: b.h}, cfg); err != nil {
		return nil, err
	}
	return &testAgent{b: b, id: id, model: "fake/m"}, nil
}

func (b *testBackend) Open(id string) (Agent, error) {
	if b.Busy(id) {
		return nil, ErrBusy
	}
	return &testAgent{b: b, id: id, model: "fake/m"}, nil
}

func (b *testBackend) startCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.started
}

type testAgent struct {
	b     *testBackend
	id    string
	model string
}

func (a *testAgent) ID() string    { return a.id }
func (a *testAgent) Model() string { return a.model }
func (a *testAgent) Release()      {}

func (a *testAgent) SetModel(spec string) error {
	w, _, err := journal.Open(a.b.h, journalFile(a.id))
	if err != nil {
		return err
	}
	if err := w.ModelSwitch(a.model, spec); err != nil {
		return err
	}
	a.model = spec
	return nil
}

func (a *testAgent) deps(obs core.Observer) core.Deps {
	return core.Deps{Host: a.b.h, Provider: a.b.prov, Tools: a.b.tools, Observer: obs}
}

func (a *testAgent) Send(ctx context.Context, prompt string, obs core.Observer) (Stats, error) {
	recs, err := a.b.Records(a.id)
	if err != nil {
		return Stats{}, err
	}
	st, err := journal.Replay(recs)
	if err != nil {
		return Stats{}, err
	}
	if st.Finished {
		return outcomeStats(core.Continue(ctx, a.deps(obs), journalFile(a.id), prompt))
	}
	return outcomeStats(core.ContinueInterrupted(ctx, a.deps(obs), journalFile(a.id), prompt))
}

func (a *testAgent) Resume(ctx context.Context, obs core.Observer) (Stats, error) {
	return outcomeStats(core.Resume(ctx, a.deps(obs), journalFile(a.id)))
}

func outcomeStats(out core.Outcome, err error) (Stats, error) {
	return Stats{
		Reason: out.Reason, Steps: out.Steps, TTFT: 7 * time.Millisecond,
		InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens,
		Jev: &JevSpend{Requests: 1},
	}, err
}

type harness struct {
	t   *testing.T
	b   *testBackend
	srv *Server
	ts  *httptest.Server
}

func newHarness(t *testing.T, b *testBackend, tweak func(*Options)) *harness {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	o := Options{Backend: b, Token: testToken, Hosts: append(AllowedHosts(ts.Listener.Addr().String()), testHost)}
	if tweak != nil {
		tweak(&o)
	}
	srv, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = srv.Handler()
	ts.Start()
	t.Cleanup(func() {
		srv.Close()
		ts.CloseClientConnections()
		ts.Close()
	})
	return &harness{t: t, b: b, srv: srv, ts: ts}
}

func (h *harness) post(path, body string) (int, string) {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.ts.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.ts.Client().Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp.StatusCode, string(data)
}

func (h *harness) start(prompt string) string {
	h.t.Helper()
	code, body := h.post("/api/runs", `{"prompt":`+strconv.Quote(prompt)+`}`)
	if code != http.StatusCreated {
		h.t.Fatalf("start: %d %s", code, body)
	}
	var out struct{ ID string }
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		h.t.Fatal(err)
	}
	return out.ID
}

type sseClient struct {
	t      *testing.T
	resp   *http.Response
	events chan Event
	cancel context.CancelFunc
}

func (h *harness) connect(id string, last int64) *sseClient {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.ts.URL+"/api/runs/"+id+"/events", nil)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	if last > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(last, 10))
	}
	resp, err := h.ts.Client().Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("connect: %d", resp.StatusCode)
	}
	c := &sseClient{t: h.t, resp: resp, events: make(chan Event, 4096), cancel: cancel}
	go c.pump()
	h.t.Cleanup(c.close)
	return c
}

func (c *sseClient) close() {
	c.cancel()
	c.resp.Body.Close()
}

func (c *sseClient) pump() {
	defer close(c.events)
	sc := bufio.NewScanner(c.resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	var id int64
	for sc.Scan() {
		line := sc.Text()
		if v, ok := strings.CutPrefix(line, "id: "); ok {
			id, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := strings.CutPrefix(line, "data: "); ok {
			var ev Event
			if json.Unmarshal([]byte(v), &ev) == nil && ev.ID == id {
				c.events <- ev
			}
		}
	}
}

func (c *sseClient) next() Event {
	c.t.Helper()
	select {
	case ev, ok := <-c.events:
		if !ok {
			c.t.Fatal("stream closed")
		}
		return ev
	case <-time.After(testWait):
		c.t.Fatal("no event")
	}
	return Event{}
}

func (c *sseClient) until(match func(Event) bool) []Event {
	c.t.Helper()
	var out []Event
	for {
		ev := c.next()
		out = append(out, ev)
		if match(ev) {
			return out
		}
	}
}

func statusIs(status string) func(Event) bool {
	return func(ev Event) bool { return ev.Kind == KindRunStatus && ev.Status == status }
}

func textOf(events []Event) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Kind == KindText {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

func kinds(events []Event) string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.Kind
	}
	return strings.Join(out, " ")
}

func checkIncreasing(t *testing.T, events []Event) {
	t.Helper()
	var last int64
	for _, ev := range events {
		if ev.ID <= last {
			t.Fatalf("id %d after %d in %+v", ev.ID, last, events)
		}
		last = ev.ID
	}
}
