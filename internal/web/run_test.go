package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func waitStatus(t *testing.T, h *harness, id, status string) {
	t.Helper()
	r, err := h.srv.mgr.run(id)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		got, busy := r.status, r.busy
		r.mu.Unlock()
		if got == status && !busy {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("status never %s", status)
}

func recordTypes(t *testing.T, b *testBackend, id string) string {
	t.Helper()
	recs, err := b.Records(id)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = string(r.Type)
	}
	return strings.Join(out, " ")
}

func TestStartAndContinue(t *testing.T) {
	h := newHarness(t, newBackend(&scripted{turns: [][]step{{{text: "one"}}, {{text: "two"}}}}), nil)
	id := h.start("first")
	waitStatus(t, h, id, StatusFinished)
	code, body := h.post("/api/runs/"+id+"/messages", `{"prompt":"second"}`)
	if code != http.StatusAccepted {
		t.Fatalf("%d %s", code, body)
	}
	waitStatus(t, h, id, StatusFinished)
	want := "run_start user assistant run_end user assistant run_end"
	if got := recordTypes(t, h.b, id); got != want {
		t.Fatalf("%s", got)
	}
}

func TestBusyRun(t *testing.T) {
	gate := make(chan struct{})
	h := newHarness(t, newBackend(&scripted{turns: [][]step{{{gate: gate, text: "x"}}}}), nil)
	id := h.start("first")
	rows := []struct {
		name, path, body string
		want             int
	}{
		{"message", "/api/runs/" + id + "/messages", `{"prompt":"again"}`, 409},
		{"model", "/api/runs/" + id + "/model", `{"model":"fake/other"}`, 409},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if code, body := h.post(row.path, row.body); code != row.want {
				t.Fatalf("%d %s", code, body)
			}
		})
	}
	close(gate)
	waitStatus(t, h, id, StatusFinished)
}

func TestForeignProcessRefused(t *testing.T) {
	h := newHarness(t, newBackend(&scripted{turns: [][]step{{{text: "x"}}}}), nil)
	id := h.start("first")
	waitStatus(t, h, id, StatusFinished)
	h.b.setForeign(id, true)
	rows := []struct{ path, body string }{
		{"/messages", `{"prompt":"again"}`},
		{"/model", `{"model":"fake/other"}`},
		{"/stop", `{}`},
	}
	for _, row := range rows {
		t.Run(row.path, func(t *testing.T) {
			if code, body := h.post("/api/runs/"+id+row.path, row.body); code != http.StatusConflict {
				t.Fatalf("%d %s", code, body)
			}
		})
	}
}

func TestStopThenMessage(t *testing.T) {
	gate := make(chan struct{})
	prov := &scripted{turns: [][]step{{{text: "par"}, {gate: gate, text: "tial"}}, {{text: "after"}}}}
	h := newHarness(t, newBackend(prov), nil)
	id := h.start("first")
	c := h.connect(id, 0)
	c.until(func(ev Event) bool { return ev.Kind == KindText })
	code, body := h.post("/api/runs/"+id+"/stop", `{}`)
	if code != 200 || !strings.Contains(body, "true") {
		t.Fatalf("%d %s", code, body)
	}
	c.until(statusIs(StatusInterrupted))
	if got := recordTypes(t, h.b, id); got != "run_start user" {
		t.Fatalf("journal %s", got)
	}
	if code, body := h.post("/api/runs/"+id+"/messages", `{"prompt":"go on"}`); code != 202 {
		t.Fatalf("%d %s", code, body)
	}
	got := c.until(statusIs(StatusFinished))
	if textOf(got) != "after" {
		t.Fatalf("%+v", got)
	}
	if code, body := h.post("/api/runs/"+id+"/stop", `{}`); code != 200 || !strings.Contains(body, "false") {
		t.Fatalf("%d %s", code, body)
	}
}

func TestErrorEvent(t *testing.T) {
	h := newHarness(t, newBackend(&scripted{turns: [][]step{{{err: fmt.Errorf("boom")}}}}), nil)
	id := h.start("first")
	got := h.connect(id, 0).until(statusIs(StatusInterrupted))
	if !strings.Contains(kinds(got), KindError) {
		t.Fatalf("%s", kinds(got))
	}
}

func TestSetModel(t *testing.T) {
	h := newHarness(t, newBackend(&scripted{turns: [][]step{{{text: "x"}}}}), nil)
	id := h.start("first")
	waitStatus(t, h, id, StatusFinished)
	c := h.connect(id, 0)
	c.until(statusIs(StatusFinished))
	code, body := h.post("/api/runs/"+id+"/model", `{"model":"fake/other"}`)
	if code != 200 || !strings.Contains(body, "fake/other") {
		t.Fatalf("%d %s", code, body)
	}
	ev := c.next()
	if ev.Kind != KindSwitch || ev.From != "fake/m" || ev.To != "fake/other" {
		t.Fatalf("%+v", ev)
	}
	if !strings.Contains(recordTypes(t, h.b, id), "model_switch") {
		t.Fatal("not journaled")
	}
	r, _ := h.srv.mgr.run(id)
	r.refresh()
	if switches := strings.Count(kinds(r.log.since(0)), KindSwitch); switches != 1 {
		t.Fatalf("switch events %d", switches)
	}
}

func TestSSEReplayThenLive(t *testing.T) {
	gate := make(chan struct{})
	call := provider.ToolCall{ID: "c1", Name: "echo", Args: json.RawMessage(`{"a":1}`)}
	prov := &scripted{turns: [][]step{{{text: "plan"}, {call: call}}, {{text: "done"}}, {{gate: gate, text: "live"}}}}
	big := strings.Repeat("r", toolTextCap+5)
	echo := tool.Tool{Name: "echo", Run: func(context.Context, tool.Env, json.RawMessage) (tool.Result, error) {
		return tool.Result{Text: big}, nil
	}}
	b := newBackend(prov, echo)
	h := newHarness(t, b, nil)
	id := h.start("first")
	waitStatus(t, h, id, StatusFinished)
	h.srv.Close()

	restarted := newHarness(t, b, nil)
	c := restarted.connect(id, 0)
	got := c.until(statusIs(StatusFinished))
	want := "user text tool_start tool_result text turn_end run_status"
	if kinds(got) != want {
		t.Fatalf("%s", kinds(got))
	}
	checkIncreasing(t, got)
	if !got[3].Truncated || len(got[3].Text) != toolTextCap || got[5].Reason != "final" {
		t.Fatalf("%+v", got[3:6])
	}
	if code, body := restarted.post("/api/runs/"+id+"/messages", `{"prompt":"more"}`); code != 202 {
		t.Fatalf("%d %s", code, body)
	}
	close(gate)
	live := c.until(statusIs(StatusFinished))
	if kinds(live) != "user run_status text turn_end run_status" {
		t.Fatalf("%s", kinds(live))
	}
	checkIncreasing(t, append(got, live...))
	end := live[3]
	if end.TTFTMS == nil || *end.TTFTMS != 7 || end.Jev == nil || end.Jev.Requests != 1 {
		t.Fatalf("%+v", end)
	}
}

func TestReconnectMidTurn(t *testing.T) {
	gates := []chan struct{}{make(chan struct{}), make(chan struct{})}
	turn := []step{{text: "aa"}, {text: "bb"}, {gate: gates[0], text: "cc"}, {gate: gates[1], text: "dd"}}
	h := newHarness(t, newBackend(&scripted{turns: [][]step{turn}}), nil)
	id := h.start("first")
	c := h.connect(id, 0)
	seen := c.until(func(ev Event) bool { return ev.Kind == KindText })
	cursor := seen[len(seen)-1]
	c.close()
	close(gates[0])

	middle := h.connect(id, cursor.ID)
	part := middle.until(func(ev Event) bool { return ev.Kind == KindText && strings.HasSuffix(ev.Text, "cc") })
	all := textOf(seen) + textOf(part)
	if all != "aabbcc" {
		t.Fatalf("%q", all)
	}
	fresh := h.connect(id, 0)
	if got := textOf(fresh.until(func(ev Event) bool { return ev.Kind == KindText })); got != "aabbcc" {
		t.Fatalf("fresh %q", got)
	}
	close(gates[1])
	rest := middle.until(statusIs(StatusFinished))
	if textOf(rest) != "dd" {
		t.Fatalf("%q", textOf(rest))
	}
	checkIncreasing(t, append(append(seen, part...), rest...))
}

func TestReconnectAfterJournaled(t *testing.T) {
	gate := make(chan struct{})
	turn := []step{{text: "aa"}, {gate: gate, text: "bb"}}
	h := newHarness(t, newBackend(&scripted{turns: [][]step{turn}}), nil)
	id := h.start("first")
	c := h.connect(id, 0)
	seen := c.until(func(ev Event) bool { return ev.Kind == KindText })
	c.close()
	close(gate)
	waitStatus(t, h, id, StatusFinished)
	got := h.connect(id, seen[len(seen)-1].ID).until(statusIs(StatusFinished))
	if kinds(got) != "text turn_end run_status" || textOf(got) != "bb" {
		t.Fatalf("%+v", got)
	}
}

func TestRetryDiscardsPartial(t *testing.T) {
	gate := make(chan struct{})
	fail := &provider.TransientError{Err: fmt.Errorf("reset")}
	turns := [][]step{{{text: "dead"}, {gate: gate, err: fail}}, {{text: "alive"}}}
	h := newHarness(t, newBackend(&scripted{turns: turns}), nil)
	id := h.start("first")
	c := h.connect(id, 0)
	before := c.until(func(ev Event) bool { return ev.Kind == KindText })
	close(gate)
	got := c.until(statusIs(StatusFinished))
	if got[0].Kind != KindRetry || !got[0].Discard || got[0].Attempt != 1 {
		t.Fatalf("%+v", got[0])
	}
	if textOf(got) != "alive" {
		t.Fatalf("%q", textOf(got))
	}
	checkIncreasing(t, append(before, got...))
	fresh := h.connect(id, 0).until(statusIs(StatusFinished))
	if textOf(fresh) != "alive" {
		t.Fatalf("fresh %q", textOf(fresh))
	}
}

func TestClosedClientLeavesRunGoing(t *testing.T) {
	gate := make(chan struct{})
	h := newHarness(t, newBackend(&scripted{turns: [][]step{{{text: "a"}, {gate: gate, text: "b"}}}}), nil)
	id := h.start("first")
	c := h.connect(id, 0)
	c.until(func(ev Event) bool { return ev.Kind == KindText })
	c.close()
	close(gate)
	waitStatus(t, h, id, StatusFinished)
	if got := recordTypes(t, h.b, id); !strings.HasSuffix(got, "run_end") {
		t.Fatal(got)
	}
}

type stuckWriter struct {
	header http.Header
	once   sync.Once
	stuck  chan struct{}
	free   chan struct{}
}

func (w *stuckWriter) Header() http.Header { return w.header }
func (w *stuckWriter) WriteHeader(int)     {}
func (w *stuckWriter) Flush()              {}

func (w *stuckWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.stuck) })
	<-w.free
	return len(p), nil
}

func TestSlowClientDroppedWithoutStall(t *testing.T) {
	gate := make(chan struct{})
	turn := []step{{text: "x"}, {gate: gate, text: "y"}}
	for i := 0; i < 32; i++ {
		turn = append(turn, step{text: "z"})
	}
	h := newHarness(t, newBackend(&scripted{turns: [][]step{turn}}), func(o *Options) { o.Queue = 4 })
	id := h.start("first")
	req := httptest.NewRequest("GET", "/api/runs/"+id+"/events", nil)
	req.Host = testHost
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := &stuckWriter{header: http.Header{}, stuck: make(chan struct{}), free: make(chan struct{})}
	served := make(chan struct{})
	go func() {
		defer close(served)
		h.srv.Handler().ServeHTTP(w, req)
	}()
	<-w.stuck
	close(gate)
	waitStatus(t, h, id, StatusFinished)
	r, _ := h.srv.mgr.run(id)
	r.mu.Lock()
	left := len(r.subs)
	r.mu.Unlock()
	if left != 0 {
		t.Fatalf("subscribers %d", left)
	}
	close(w.free)
	select {
	case <-served:
	case <-time.After(testWait):
		t.Fatal("handler still running")
	}
}

func TestHeartbeat(t *testing.T) {
	b := newBackend(&scripted{turns: [][]step{{{text: "x"}}}})
	h := newHarness(t, b, func(o *Options) { o.Heartbeat = time.Millisecond })
	id := h.start("first")
	waitStatus(t, h, id, StatusFinished)
	req, _ := http.NewRequest("GET", h.ts.URL+"/api/runs/"+id+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	resp, err := h.ts.Client().Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var seen strings.Builder
	buf := make([]byte, 4096)
	for !strings.Contains(seen.String(), ": heartbeat") {
		n, err := resp.Body.Read(buf)
		if err != nil {
			t.Fatalf("%v after %q", err, seen.String())
		}
		seen.Write(buf[:n])
	}
}

func TestForeignJournalGrowthAppears(t *testing.T) {
	b := newBackend(&scripted{turns: [][]step{{{text: "x"}}}})
	h := newHarness(t, b, func(o *Options) { o.Heartbeat = time.Millisecond })
	id := h.start("first")
	waitStatus(t, h, id, StatusFinished)
	c := h.connect(id, 0)
	c.until(statusIs(StatusFinished))
	w, _, err := journal.Open(b.h, journalFile(id))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.User("from a terminal"); err != nil {
		t.Fatal(err)
	}
	got := c.until(statusIs(StatusInterrupted))
	if kinds(got) != "user run_status" || got[0].Text != "from a terminal" {
		t.Fatalf("%+v", got)
	}
}

func TestShutdownInterruptsTurns(t *testing.T) {
	gate := make(chan struct{})
	defer close(gate)
	h := newHarness(t, newBackend(&scripted{turns: [][]step{{{text: "a"}, {gate: gate, text: "b"}}}}), nil)
	id := h.start("first")
	c := h.connect(id, 0)
	c.until(func(ev Event) bool { return ev.Kind == KindText })
	h.srv.Close()
	r, _ := h.srv.mgr.run(id)
	if r.status != StatusInterrupted {
		t.Fatalf("status %s", r.status)
	}
	if got := recordTypes(t, h.b, id); got != "run_start user" {
		t.Fatalf("journal %s", got)
	}
	if code, _ := h.post("/api/runs", `{"prompt":"late"}`); code != http.StatusServiceUnavailable {
		t.Fatalf("code %d", code)
	}
}

func TestListAndState(t *testing.T) {
	h := newHarness(t, newBackend(&scripted{}), nil)
	for _, path := range []string{"/api/state", "/api/runs"} {
		req, _ := http.NewRequest("GET", h.ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := h.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("%s %d", path, resp.StatusCode)
		}
	}
}
