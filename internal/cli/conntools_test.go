package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/mcp/mcptest"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

func connCall(id, name, args string) provider.ToolCall {
	return provider.ToolCall{ID: id, Name: name, Args: json.RawMessage(args)}
}

func addFake(t *testing.T, e *addEnv, name string, extra ...string) {
	t.Helper()
	if code := e.run(e.serverArgs(name, extra...)...); code != 0 {
		t.Fatalf("add: %d %s", code, e.stderr)
	}
	if err := os.Remove(e.mark); err != nil {
		t.Fatal(err)
	}
}

func requestToolNames(r provider.Request) []string {
	out := make([]string, len(r.Tools))
	for i, d := range r.Tools {
		out[i] = d.Name
	}
	return out
}

func toolResults(r provider.Request) []provider.ToolResult {
	var out []provider.ToolResult
	for _, m := range r.Messages {
		out = append(out, m.ToolResults...)
	}
	return out
}

func TestUnusedConnectionStartsNoProcess(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("hello")}}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if n := e.starts(t); n != 0 {
		t.Fatalf("starts %d", n)
	}
	if !slices.Contains(requestToolNames(fp.Requests[0]), tool.SearchName) {
		t.Fatalf("tools %v", requestToolNames(fp.Requests[0]))
	}
}

func TestFirstCallStartsOneServerAndRunEndsIt(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("1", "fs__echo", `{"text":"a"}`), connCall("2", "fs__echo", `{"text":"b"}`)),
		fake.ToolTurn(connCall("3", "fs__group", `{}`)),
		fake.TextTurn("done"),
	}}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if n := e.starts(t); n != 1 {
		t.Fatalf("starts %d", n)
	}
	results := toolResults(fp.Requests[2])
	if len(results) != 3 || results[0].Text != `echo:{"text":"a"}` {
		t.Fatalf("results %+v", results)
	}
	data, _ := os.ReadFile(e.mark)
	pid := strings.Fields(string(data))[1]
	proc := filepath.Join("/proc", pid, "comm")
	if _, err := os.Stat(proc); err == nil && !procIsZombieText(pid) {
		t.Fatalf("server %s outlived the run", pid)
	}
}

func procIsZombieText(pid string) bool {
	data, err := os.ReadFile(filepath.Join("/proc", pid, "stat"))
	return err != nil || strings.Contains(string(data), ") Z ")
}

func TestFailedStartIsAResultNotAnEnd(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("1", "fs__echo", `{}`)),
		fake.TextTurn("recovered"),
	}}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs")
	setServerEnv(t, e, "fs", mcptest.EnvFail, "server cannot start")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	results := toolResults(fp.Requests[1])
	if len(results) != 1 || !results[0].IsError || !strings.Contains(results[0].Text, "server cannot start") {
		t.Fatalf("results %+v", results)
	}
}

func setServerEnv(t *testing.T, e *addEnv, name, key, value string) {
	t.Helper()
	var set connectionSet
	readTestJSON(t, e.configFile(connectionsFile), &set)
	c := set.Servers[name]
	c.Env[key] = value
	set.Servers[name] = c
	if err := saveConnections(filepath.Dir(e.configFile(connectionsFile)), set); err != nil {
		t.Fatal(err)
	}
}

func TestMissingSecretAtRunIsAResult(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("1", "fs__echo", `{}`)),
		fake.TextTurn("ok"),
	}}
	e := newAddEnv(t, fp)
	e.vars["TOKEN_X"] = "value-long"
	addFake(t, e, "fs", "--env", "TOKEN=$TOKEN_X")
	delete(e.vars, "TOKEN_X")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	results := toolResults(fp.Requests[1])
	if len(results) != 1 || !results[0].IsError || !strings.Contains(results[0].Text, "$TOKEN_X is not set") {
		t.Fatalf("results %+v", results)
	}
	if n := e.starts(t); n != 0 {
		t.Fatalf("starts %d", n)
	}
}

func TestResolvedSecretReachesTheServer(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("1", "fs__env", `{}`)),
		fake.TextTurn("ok"),
	}}
	e := newAddEnv(t, fp)
	e.vars["TOKEN_X"] = "value-long"
	addFake(t, e, "fs", "--env", "TOKEN=$TOKEN_X", "--env", mcptest.EnvEcho+"=TOKEN")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if results := toolResults(fp.Requests[1]); results[0].Text != "value-long" {
		t.Fatalf("results %+v", results)
	}
}

func TestDeferredToolLoadsThenRunsThroughTheServer(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("1", tool.SearchName, `{"query":"fs__echo"}`)),
		fake.ToolTurn(connCall("2", "fs__echo", `{"text":"x"}`)),
		fake.TextTurn("done"),
	}}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if slices.Contains(requestToolNames(fp.Requests[0]), "fs__echo") {
		t.Fatal("schema sent before it was loaded")
	}
	if !slices.Contains(requestToolNames(fp.Requests[1]), "fs__echo") {
		t.Fatalf("schema missing: %v", requestToolNames(fp.Requests[1]))
	}
	if !strings.Contains(e.stderr.String(), "tools fs__echo") {
		t.Fatalf("stderr %q", e.stderr)
	}
}

func TestFewConnectionToolsAreSentDirectly(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("hi")}}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs", "--env", mcptest.EnvTools+"=2")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	want := []string{"read", "write", "edit", "bash", "grep", "fs__echo", "fs__tool01"}
	if got := requestToolNames(fp.Requests[0]); !slices.Equal(got, want) {
		t.Fatalf("tools %v", got)
	}
}

func TestJevKeepsEveryToolInTheRegistry(t *testing.T) {
	e := newAddEnv(t, nil)
	addFake(t, e, "fs")
	set := openToolset(e.Env, nil, &jev.Client{})
	defer set.close()
	if _, ok := set.reg.Get(tool.SearchName); ok {
		t.Fatal("no tools tool with Jev")
	}
	if got := len(set.reg.Request(nil)); got != len(set.reg.Defs()) || got < 14 {
		t.Fatalf("request %d defs %d", got, len(set.reg.Defs()))
	}
}

func TestResumeKeepsLoadedToolsAndRemovedConnectionIsSafe(t *testing.T) {
	first := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("1", tool.SearchName, `{"query":"fs__echo"}`)),
		fake.ToolTurn(connCall("2", "fs__echo", `{"text":"x"}`)),
		{Err: context.DeadlineExceeded},
	}}
	e := newAddEnv(t, first)
	addFake(t, e, "fs")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 1 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	id := runIDFrom(t, e)

	second := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("3", "fs__echo", `{"text":"y"}`)),
		{Err: context.DeadlineExceeded},
	}}
	e.NewProvider = func(Endpoint) provider.Provider { return second }
	if code := e.run("resume", id); code != 1 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if !slices.Contains(requestToolNames(second.Requests[0]), "fs__echo") {
		t.Fatalf("lost on resume: %v", requestToolNames(second.Requests[0]))
	}
	if results := toolResults(second.Requests[1]); results[len(results)-1].Text != `echo:{"text":"y"}` {
		t.Fatalf("results %+v", results)
	}

	if code := e.run("remove", "fs"); code != 0 {
		t.Fatalf("remove %d", code)
	}
	third := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("4", "fs__echo", `{}`)),
		fake.TextTurn("done"),
	}}
	e.NewProvider = func(Endpoint) provider.Provider { return third }
	if code := e.run("resume", id); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if slices.Contains(requestToolNames(third.Requests[0]), "fs__echo") {
		t.Fatalf("removed tool sent: %v", requestToolNames(third.Requests[0]))
	}
	results := toolResults(third.Requests[1])
	last := results[len(results)-1]
	if !last.IsError || !strings.Contains(last.Text, "unknown tool") {
		t.Fatalf("result %+v", last)
	}
}

func runIDFrom(t *testing.T, e *addEnv) string {
	t.Helper()
	ents, err := os.ReadDir(runsDir(e.Getenv))
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if id, ok := strings.CutSuffix(ent.Name(), ".jsonl"); ok {
			return id
		}
	}
	t.Fatal("no run")
	return ""
}

func TestSessionAgentSharesServersAcrossTurnsAndReleases(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("1", "fs__echo", `{}`)),
		fake.TextTurn("one"),
		fake.ToolTurn(connCall("2", "fs__echo", `{}`)),
		fake.TextTurn("two"),
	}}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs")
	ep, bySpec, err := resolveRunSpecs(runOpts{model: "xai/grok"}, e.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	a, err := newSessionAgent(e.Env, runOpts{}, ep, bySpec, nil, e.home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.release()
	for range 2 {
		if _, err := a.Send(context.Background(), "go", func(tui.Event) {}); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.starts(t); n != 1 {
		t.Fatalf("starts %d", n)
	}
	data, _ := os.ReadFile(e.mark)
	pid := strings.Fields(string(data))[1]
	a.release()
	if !procIsZombieText(pid) {
		if _, err := os.Stat(filepath.Join("/proc", pid)); err == nil {
			t.Fatal("server outlived release")
		}
	}
}

func webObserver() core.Observer {
	return core.Observer{
		OnTurnStart: func() {}, OnEvent: func(provider.Event) {},
		OnToolStart: func(provider.ToolCall) {}, OnSwitch: func(string, string) {},
	}
}

func counterTurn(id string) []fake.Turn {
	return []fake.Turn{fake.ToolTurn(connCall(id, "fs__"+mcptest.Counter, `{}`)), fake.TextTurn("done")}
}

func newWebFixture(t *testing.T, idle time.Duration, turns int) (*addEnv, *webBackend, *fake.Provider) {
	t.Helper()
	fp := &fake.Provider{}
	for i := range turns {
		fp.Turns = append(fp.Turns, counterTurn(strconv.Itoa(i))...)
	}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs")
	b := &webBackend{env: e.Env, opts: runOpts{model: "xai/grok"}, dir: e.home, pool: newToolPool(idle)}
	t.Cleanup(b.pool.close)
	return e, b, fp
}

func lastCount(t *testing.T, fp *fake.Provider) string {
	t.Helper()
	results := toolResults(fp.Requests[len(fp.Requests)-1])
	return results[len(results)-1].Text
}

func webTurn(t *testing.T, b *webBackend, id string) string {
	t.Helper()
	a, err := b.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	if _, err := a.Send(context.Background(), "again", webObserver()); err != nil {
		t.Fatal(err)
	}
	return a.ID()
}

func webFirstTurn(t *testing.T, b *webBackend) string {
	t.Helper()
	a, err := b.Start("", "go")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	if _, err := a.Resume(context.Background(), webObserver()); err != nil {
		t.Fatal(err)
	}
	return a.ID()
}

func serverGone(t *testing.T, e *addEnv) bool {
	t.Helper()
	data, err := os.ReadFile(e.mark)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(data))
	pid := fields[len(fields)-1]
	_, err = os.Stat(filepath.Join("/proc", pid))
	return err != nil || procIsZombieText(pid)
}

func waitServerGone(t *testing.T, e *addEnv) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !serverGone(t, e) {
		if time.Now().After(deadline) {
			t.Fatal("server still running")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWebServerStartsLazily(t *testing.T) {
	e, b, _ := newWebFixture(t, time.Minute, 1)
	a, err := b.Start("", "go")
	if err != nil {
		t.Fatal(err)
	}
	a.Release()
	if n := e.starts(t); n != 0 {
		t.Fatalf("starts %d", n)
	}
}

func TestWebStatefulServerSurvivesBetweenTurns(t *testing.T) {
	e, b, fp := newWebFixture(t, time.Minute, 3)
	id := webFirstTurn(t, b)
	if got := lastCount(t, fp); got != "1" {
		t.Fatalf("count %s", got)
	}
	for _, want := range []string{"2", "3"} {
		webTurn(t, b, id)
		if got := lastCount(t, fp); got != want {
			t.Fatalf("count %s want %s", got, want)
		}
	}
	if n := e.starts(t); n != 1 {
		t.Fatalf("starts %d", n)
	}
	if serverGone(t, e) {
		t.Fatal("server should wait for the next turn")
	}
}

func TestWebRunsKeepSeparateServers(t *testing.T) {
	e, b, fp := newWebFixture(t, time.Minute, 2)
	e.Rand = bytes.NewReader([]byte{1, 2, 3, 4})
	b.env = e.Env
	webFirstTurn(t, b)
	webFirstTurn(t, b)
	if got := lastCount(t, fp); got != "1" {
		t.Fatalf("count %s", got)
	}
	if n := e.starts(t); n != 2 {
		t.Fatalf("starts %d", n)
	}
}

func TestWebIdleRunClosesItsServers(t *testing.T) {
	e, b, fp := newWebFixture(t, 50*time.Millisecond, 2)
	id := webFirstTurn(t, b)
	waitServerGone(t, e)
	webTurn(t, b, id)
	if got := lastCount(t, fp); got != "1" {
		t.Fatalf("count %s", got)
	}
	if n := e.starts(t); n != 2 {
		t.Fatalf("starts %d", n)
	}
}

func TestWebActiveTurnIsNotIdle(t *testing.T) {
	e, b, _ := newWebFixture(t, 50*time.Millisecond, 2)
	id := webFirstTurn(t, b)
	a, err := b.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	time.Sleep(150 * time.Millisecond)
	if serverGone(t, e) {
		t.Fatal("server closed during a turn")
	}
}

func TestWebShutdownClosesParkedServers(t *testing.T) {
	e, b, _ := newWebFixture(t, time.Hour, 1)
	webFirstTurn(t, b)
	if serverGone(t, e) {
		t.Fatal("server should be parked")
	}
	b.pool.close()
	waitServerGone(t, e)
}

func TestReleaseAfterShutdownClosesTheServer(t *testing.T) {
	e, b, _ := newWebFixture(t, time.Hour, 1)
	a, err := b.Start("", "go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Resume(context.Background(), webObserver()); err != nil {
		t.Fatal(err)
	}
	b.pool.close()
	a.Release()
	waitServerGone(t, e)
}

func TestStoppingHpWebClosesServers(t *testing.T) {
	fp := &fake.Provider{Turns: counterTurn("1")}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs")
	stderr := &syncBuffer{}
	env := e.Env
	env.Stderr = stderr
	env.Rand = rand.Reader
	s := startWeb(t, env, stderr, "-m", "xai/grok")
	s.start("go")
	deadline := time.Now().Add(5 * time.Second)
	for e.starts(t) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("server never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.cancel()
	<-s.code
	waitServerGone(t, e)
}

func TestToolPoolRace(t *testing.T) {
	p := newToolPool(time.Microsecond)
	defer p.close()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				if got := p.take("r"); got != nil {
					p.park("r", got)
					continue
				}
				p.park("r", &toolset{})
			}
		}()
	}
	wg.Wait()
}

type readSpy struct {
	host.Native
	read []string
}

func (s *readSpy) ReadFile(path string) ([]byte, error) {
	s.read = append(s.read, path)
	return s.Native.ReadFile(path)
}

func TestReadOnlyCommandsNeverReadConnections(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"version"}, {"runs"}, {"help"}} {
		e := newAddEnv(t, nil)
		spy := &readSpy{}
		e.Host = spy
		e.run(args...)
		for _, p := range spy.read {
			if strings.Contains(p, "connections") {
				t.Fatalf("%v read %s", args, p)
			}
		}
	}
}

func TestProjectFilesNeverProvideConnections(t *testing.T) {
	tests := []struct {
		name, home string
	}{
		{name: "project dot hp", home: ""},
		{name: "empty home falls back to nothing", home: "unset"},
		{name: "relative home", home: "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAddEnv(t, &fake.Provider{Turns: []fake.Turn{fake.TextTurn("hi")}})
			proj := filepath.Join(e.home, "proj")
			e.Cwd = proj
			body := `{"servers":{"evil":{"command":"/bin/sh"}}}`
			for _, rel := range []string{".hp/connections.json", ".config/hp/connections.json", "connections.json"} {
				file := filepath.Join(proj, rel)
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.home == "unset" {
				e.vars["HOME"] = ""
			} else if tt.home != "" {
				e.vars["HOME"] = tt.home
			}
			connected, clients := connectionTools(e.Env)
			if len(connected) != 0 || len(clients) != 0 {
				t.Fatalf("loaded %d tools %d clients", len(connected), len(clients))
			}
		})
	}
}

func TestBrokenConfigWarnsAndTheRunGoesOn(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("hi")}}
	e := newAddEnv(t, fp)
	file := e.configFile(connectionsFile)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if !strings.Contains(e.stderr.String(), "connections not loaded") {
		t.Fatalf("stderr %q", e.stderr)
	}
}

func TestToolsCallIsJournaled(t *testing.T) {
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(connCall("1", tool.SearchName, `{"query":"echo"}`)),
		fake.TextTurn("done"),
	}}
	e := newAddEnv(t, fp)
	addFake(t, e, "fs")
	if code := e.run("run", "-m", "xai/grok", "go"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	jpath := filepath.Join(runsDir(e.Getenv), runIDFrom(t, e)+".jsonl")
	recs, err := journal.Read(e.Host, jpath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range st.Messages {
		for _, r := range m.ToolResults {
			found = found || strings.HasPrefix(r.Text, "loaded: fs__echo")
		}
	}
	if !found {
		t.Fatalf("messages %+v", st.Messages)
	}
}
