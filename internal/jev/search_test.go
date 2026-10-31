package jev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

type linkHost struct {
	*host.Memory
	links map[string]bool
}

func (h linkHost) ReadDir(path string) ([]host.Entry, error) {
	ents, err := h.Memory.ReadDir(path)
	for i := range ents {
		ents[i].IsLink = h.links[filepath.Join(path, ents[i].Name)]
	}
	return ents, err
}

type fakeJev struct {
	mu     sync.Mutex
	bodies []string
	asked  []string
}

func (f *fakeJev) record(body string, questions []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies = append(f.bodies, body)
	f.asked = append(f.asked, questions...)
}

func (f *fakeJev) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func (f *fakeJev) sent() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.bodies, "\n")
}

func (f *fakeJev) count(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, q := range f.asked {
		if strings.Contains(q, substr) {
			n++
		}
	}
	return n
}

func (f *fakeJev) handle(score func(string) float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			w.WriteHeader(500)
			return
		}
		var req wireRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			w.WriteHeader(500)
			return
		}
		answers := make(map[string]wireAnswer, len(req.Questions))
		questions := make([]string, 0, len(req.Questions))
		for name, q := range req.Questions {
			text, _ := q.Instructions.(string)
			questions = append(questions, text)
			answers[name] = wireAnswer{Type: "noul", Noul: score(text)}
		}
		f.record(string(raw), questions)
		out, _ := json.Marshal(wireResponse{Model: req.Model, Answers: answers})
		_, _ = w.Write(out)
	}
}

func searchServer(t *testing.T, score func(string) float64) (*Client, *fakeJev) {
	t.Helper()
	f := &fakeJev{}
	srv := httptest.NewServer(f.handle(score))
	t.Cleanup(srv.Close)
	return &Client{URL: srv.URL, Key: "k", Model: "m"}, f
}

func isPathQuestion(text string) bool {
	return strings.HasPrefix(text, "Could the file at this path")
}

func scoreWindows(marks map[string]float64) func(string) float64 {
	return func(text string) float64 {
		if isPathQuestion(text) {
			return 0.9
		}
		best := 0.0
		for mark, s := range marks {
			if strings.Contains(text, mark) {
				best = max(best, s)
			}
		}
		return best
	}
}

func treeEnv(t *testing.T, files map[string]string, links ...string) tool.Env {
	t.Helper()
	h := linkHost{Memory: host.NewMemory(), links: map[string]bool{}}
	if err := h.WriteFile("/w/.keep", nil); err != nil {
		t.Fatal(err)
	}
	if err := h.Remove("/w/.keep"); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := h.WriteFile("/w/"+name, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range links {
		h.links["/w/"+l] = true
	}
	return tool.Env{Host: h, Dir: "/w"}
}

func numbered(n int, marks map[int]string) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
		if m, ok := marks[i+1]; ok {
			lines[i] = m
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func runSearch(t *testing.T, c *Client, env tool.Env, args string) tool.Result {
	t.Helper()
	res, err := SearchTool(c).Run(context.Background(), env, json.RawMessage(args))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSearchToolShape(t *testing.T) {
	st := SearchTool(&Client{})
	if st.Name != "search" {
		t.Fatalf("name %q", st.Name)
	}
	var doc map[string]any
	if err := json.Unmarshal(st.Schema, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["additionalProperties"] != false || !reflect.DeepEqual(doc["required"], []any{"query"}) {
		t.Fatalf("schema %v", doc)
	}
}

func TestSearchBestWindowFirst(t *testing.T) {
	env := treeEnv(t, map[string]string{
		"internal/retry.go": numbered(200, map[int]string{120: "func parseRetryAfter(h string) int {"}),
		"internal/other.go": numbered(30, map[int]string{5: "func weakMatch() {}"}),
	})
	c, _ := searchServer(t, scoreWindows(map[string]float64{"parseRetryAfter": 0.87, "weakMatch": 0.6}))
	res := runSearch(t, c, env, `{"query":"parses retry headers"}`)
	want := "internal/retry.go:101-160  0.87\ninternal/other.go:1-30  0.60"
	if res.IsError || res.Text != want {
		t.Fatalf("result %+v", res)
	}
}

func TestSearchMergesOverlappingWindows(t *testing.T) {
	env := treeEnv(t, map[string]string{
		"a.go": numbered(200, map[int]string{20: "MARK_LOW", 80: "MARK_HIGH", 190: "MARK_FAR"}),
	})
	c, _ := searchServer(t, scoreWindows(map[string]float64{"MARK_LOW": 0.6, "MARK_HIGH": 0.8, "MARK_FAR": 0.7}))
	res := runSearch(t, c, env, `{"query":"q"}`)
	want := "a.go:1-110  0.80\na.go:151-200  0.70"
	if res.Text != want {
		t.Fatalf("got %q", res.Text)
	}
}

func TestMergeHits(t *testing.T) {
	tests := []struct {
		name string
		in   []hit
		want []hit
	}{
		{
			name: "adjacent",
			in:   []hit{{"a", 61, 120, 0.9}, {"a", 1, 60, 0.6}},
			want: []hit{{"a", 1, 120, 0.9}},
		},
		{
			name: "overlapping",
			in:   []hit{{"a", 1, 60, 0.9}, {"a", 51, 110, 0.6}},
			want: []hit{{"a", 1, 110, 0.9}},
		},
		{
			name: "gap stays apart",
			in:   []hit{{"a", 1, 60, 0.9}, {"a", 62, 120, 0.6}},
			want: []hit{{"a", 1, 60, 0.9}, {"a", 62, 120, 0.6}},
		},
		{
			name: "other file stays apart",
			in:   []hit{{"a", 1, 60, 0.9}, {"b", 61, 120, 0.6}},
			want: []hit{{"a", 1, 60, 0.9}, {"b", 61, 120, 0.6}},
		},
		{
			name: "nested keeps outer end",
			in:   []hit{{"a", 1, 120, 0.5}, {"a", 51, 60, 0.7}},
			want: []hit{{"a", 1, 120, 0.7}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeHits(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestSplitWindows(t *testing.T) {
	tests := []struct {
		name string
		text string
		want [][2]int
	}{
		{name: "empty", text: "", want: nil},
		{name: "blank lines only", text: "\n\n\n", want: nil},
		{name: "no trailing newline", text: "a\nb", want: [][2]int{{1, 2}}},
		{name: "one full window", text: numbered(60, nil), want: [][2]int{{1, 60}}},
		{name: "one line over", text: numbered(61, nil), want: [][2]int{{1, 60}, {51, 61}}},
		{name: "three windows", text: numbered(150, nil), want: [][2]int{{1, 60}, {51, 110}, {101, 150}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got [][2]int
			for _, w := range splitWindows("a", tt.text) {
				got = append(got, [2]int{w.start, w.end})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestSearchSendsOnlyFirst32KB(t *testing.T) {
	env := treeEnv(t, map[string]string{"big.txt": strings.Repeat("x\n", 16384) + "BEYOND_THE_CUT\n"})
	c, f := searchServer(t, scoreWindows(nil))
	runSearch(t, c, env, `{"query":"q"}`)
	if f.count("excerpt") == 0 {
		t.Fatal("no windows sent")
	}
	if strings.Contains(f.sent(), "BEYOND_THE_CUT") {
		t.Fatal("content past 32 KB sent")
	}
}

func TestSearchLimitAndTieOrder(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"c.go", "a.go", "b.go"} {
		files[name] = numbered(200, map[int]string{5: "MARK", 190: "MARK"})
	}
	marks := map[string]float64{"MARK": 0.7}
	lines := []string{"a.go:1-60  0.70", "a.go:151-200  0.70", "b.go:1-60  0.70", "b.go:151-200  0.70", "c.go:1-60  0.70", "c.go:151-200  0.70"}
	tests := []struct {
		name string
		args string
		want []string
	}{
		{name: "limit one", args: `{"query":"q","limit":1}`, want: lines[:1]},
		{name: "limit three", args: `{"query":"q","limit":3}`, want: lines[:3]},
		{name: "limit above hits", args: `{"query":"q","limit":50}`, want: lines},
		{name: "default", args: `{"query":"q"}`, want: lines},
		{name: "zero is default", args: `{"query":"q","limit":0}`, want: lines},
		{name: "negative is default", args: `{"query":"q","limit":-3}`, want: lines},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := searchServer(t, scoreWindows(marks))
			res := runSearch(t, c, treeEnv(t, files), tt.args)
			if res.Text != strings.Join(tt.want, "\n") {
				t.Fatalf("got %q", res.Text)
			}
		})
	}
}

func TestSearchDefaultLimitIsTen(t *testing.T) {
	files := map[string]string{}
	for i := range 12 {
		files[fmt.Sprintf("f%02d.go", i)] = "MARK\n"
	}
	c, _ := searchServer(t, scoreWindows(map[string]float64{"MARK": 0.7}))
	res := runSearch(t, c, treeEnv(t, files), `{"query":"q"}`)
	if n := strings.Count(res.Text, "\n") + 1; n != 10 {
		t.Fatalf("lines %d", n)
	}
}

func TestSearchPathScope(t *testing.T) {
	env := treeEnv(t, map[string]string{"in/a.go": "MARK\n", "out/b.go": "MARK\n"})
	c, f := searchServer(t, scoreWindows(map[string]float64{"MARK": 0.7}))
	res := runSearch(t, c, env, `{"query":"q","path":"in"}`)
	if res.Text != "in/a.go:1-1  0.70" {
		t.Fatalf("got %q", res.Text)
	}
	if strings.Contains(f.sent(), "out/b.go") {
		t.Fatal("left the path scope")
	}
}

func TestSearchKeepsTopTwentyPaths(t *testing.T) {
	files := map[string]string{}
	for i := range 25 {
		files[fmt.Sprintf("f%02d.go", i)] = "body\n"
	}
	c, f := searchServer(t, func(text string) float64 {
		if isPathQuestion(text) && strings.Contains(text, "f24.go") {
			return 0.95
		}
		return 0.5
	})
	runSearch(t, c, treeEnv(t, files), `{"query":"q"}`)
	if n := f.count("excerpt"); n != 20 {
		t.Fatalf("windows %d", n)
	}
	if f.count("excerpt of f24.go") != 1 {
		t.Fatal("best path dropped")
	}
	if f.count("excerpt of f20.go") != 0 {
		t.Fatal("tie not broken by path")
	}
}

func TestSearchCapsFiles(t *testing.T) {
	files := map[string]string{}
	for i := range maxSearchFiles + 1 {
		files[fmt.Sprintf("f%05d.go", i)] = "x\n"
	}
	c, f := searchServer(t, func(string) float64 { return 0 })
	runSearch(t, c, treeEnv(t, files), `{"query":"q"}`)
	if n := f.count("at this path"); n != maxSearchFiles {
		t.Fatalf("paths %d", n)
	}
}

func TestSearchWithholdsSecrets(t *testing.T) {
	const secret = "TOPSECRETVALUE"
	const safe = "safe.go"
	block := "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n"
	tests := []struct {
		name  string
		files map[string]string
		links []string
		args  string
		wants []string
	}{
		{name: ".env", files: map[string]string{".env": secret}},
		{name: ".env.local", files: map[string]string{".env.local": secret}},
		{name: "upper case .ENV", files: map[string]string{".ENV": secret}},
		{name: "id_rsa", files: map[string]string{"id_rsa": secret}},
		{name: "id_rsa.pub", files: map[string]string{"id_rsa.pub": secret}},
		{name: "upper case ID_RSA", files: map[string]string{"ID_RSA": secret}},
		{name: "id_ed25519", files: map[string]string{"id_ed25519": secret}},
		{name: "id_ecdsa", files: map[string]string{"id_ecdsa": secret}},
		{name: "server.key", files: map[string]string{"server.key": secret}},
		{name: "upper case KEY", files: map[string]string{"Server.KEY": secret}},
		{name: "cert.pem", files: map[string]string{"cert.pem": secret}},
		{name: "credentials", files: map[string]string{"aws_Credentials.json": secret}},
		{name: ".netrc", files: map[string]string{".netrc": secret}},
		{name: ".npmrc", files: map[string]string{".npmrc": secret}},
		{name: "nested", files: map[string]string{"deploy/prod/.env": secret}},
		{name: "symlinked file", files: map[string]string{"notes.txt": secret}, links: []string{"notes.txt"}},
		{name: "private key block", files: map[string]string{"notes.txt": secret + "\n" + block}},
		{name: "private key past the cut", files: map[string]string{"notes.txt": secret + "\n" + strings.Repeat("x\n", 20000) + block}},
		{name: "secret named root", files: map[string]string{".env": secret}, args: `{"query":"q","path":".env"}`},
		{name: "symlinked file root", files: map[string]string{"notes.txt": secret}, links: []string{"notes.txt"}, args: `{"query":"q","path":"notes.txt"}`},
		{name: "symlinked directory root", files: map[string]string{"linked/notes.txt": secret}, links: []string{"linked"}, args: `{"query":"q","path":"linked"}`},
		{name: "root under symlinked directory", files: map[string]string{"linked/notes.txt": secret}, links: []string{"linked"}, args: `{"query":"q","path":"linked/notes.txt"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{safe: "harmless\n"}
			for name, text := range tt.files {
				files[name] = text
			}
			args := tt.args
			if args == "" {
				args = `{"query":"q"}`
			}
			c, f := searchServer(t, func(string) float64 { return 0.9 })
			res := runSearch(t, c, treeEnv(t, files, tt.links...), args)
			sent := f.sent()
			for name := range tt.files {
				if strings.Contains(sent, filepath.Base(name)) || strings.Contains(res.Text, filepath.Base(name)) {
					t.Fatalf("path of %s leaked", name)
				}
			}
			if strings.Contains(sent, secret) || strings.Contains(sent, "PRIVATE KEY") {
				t.Fatal("secret content sent")
			}
			if tt.args == "" && !strings.Contains(sent, safe) {
				t.Fatal("safe file was not searched")
			}
		})
	}
}

func TestSearchNoMatches(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		score    func(string) float64
		requests int
	}{
		{name: "empty tree", requests: 0},
		{name: "only secrets", files: map[string]string{".env": "x"}, requests: 0},
		{name: "only binary", files: map[string]string{"a.bin": "\x00\x01"}, requests: 0},
		{
			name:     "no path above threshold",
			files:    map[string]string{"a.go": "x\n"},
			score:    func(string) float64 { return 0.29 },
			requests: 1,
		},
		{
			name:     "no window above threshold",
			files:    map[string]string{"a.go": "x\n"},
			score:    func(s string) float64 { return map[bool]float64{true: 0.3, false: 0.49}[isPathQuestion(s)] },
			requests: 2,
		},
		{
			name:     "kept file has nothing to send",
			files:    map[string]string{"empty.go": ""},
			score:    func(string) float64 { return 0.9 },
			requests: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := tt.score
			if score == nil {
				score = func(string) float64 { return 0.9 }
			}
			c, f := searchServer(t, score)
			res := runSearch(t, c, treeEnv(t, tt.files), `{"query":"q"}`)
			if res.IsError || res.Text != "no matches" {
				t.Fatalf("result %+v", res)
			}
			if f.requests() != tt.requests {
				t.Fatalf("requests %d want %d", f.requests(), tt.requests)
			}
		})
	}
}

func TestSearchThresholdsAreInclusive(t *testing.T) {
	env := treeEnv(t, map[string]string{"a.go": "x\n"})
	c, _ := searchServer(t, func(s string) float64 {
		if isPathQuestion(s) {
			return 0.3
		}
		return 0.5
	})
	if res := runSearch(t, c, env, `{"query":"q"}`); res.Text != "a.go:1-1  0.50" {
		t.Fatalf("got %q", res.Text)
	}
}

func TestSearchQuestionNamesAreIdentifiers(t *testing.T) {
	var names []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req wireRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		for name := range req.Questions {
			names = append(names, name)
		}
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL, Key: "k", Model: "m"}
	runSearch(t, c, treeEnv(t, map[string]string{"weird-name.go": "x\n", "sub/b.go": "y\n"}), `{"query":"q"}`)
	sortedWant := []string{"q0", "q1"}
	if len(names) != 2 || !reflect.DeepEqual(sortedQuestionNames(map[string]Question{names[0]: {}, names[1]: {}}), sortedWant) {
		t.Fatalf("names %v", names)
	}
}

func TestSearchJevErrorIsResult(t *testing.T) {
	failWindows := func(w http.ResponseWriter, r *http.Request) {
		var req wireRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, q := range req.Questions {
			if text, _ := q.Instructions.(string); !isPathQuestion(text) {
				w.WriteHeader(400)
				_, _ = w.Write([]byte("bad request detail"))
				return
			}
		}
		out, _ := json.Marshal(wireResponse{Answers: map[string]wireAnswer{"q0": {Type: "noul", Noul: 0.9}}})
		_, _ = w.Write(out)
	}
	failAll := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte("bad request detail"))
	}
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "path pass", handler: failAll},
		{name: "window pass", handler: failWindows},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			c := &Client{URL: srv.URL, Key: "k", Model: "m"}
			res := runSearch(t, c, treeEnv(t, map[string]string{"a.go": "x\n"}), `{"query":"q"}`)
			if !res.IsError || !strings.Contains(res.Text, "400") || !strings.Contains(res.Text, "bad request detail") {
				t.Fatalf("result %+v", res)
			}
		})
	}
}

func TestSearchBadArgsAreResults(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "empty query", args: `{"query":""}`, want: "query is required"},
		{name: "blank query", args: `{"query":"  "}`, want: "query is required"},
		{name: "missing query", args: `{}`, want: "query is required"},
		{name: "malformed", args: `{`, want: "EOF"},
		{name: "unknown field", args: `{"query":"q","glob":"*.go"}`, want: "glob"},
		{name: "missing path", args: `{"query":"q","path":"nope"}`, want: "path not found: nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, f := searchServer(t, func(string) float64 { return 0.9 })
			res := runSearch(t, c, treeEnv(t, map[string]string{"a.go": "x\n"}), tt.args)
			if !res.IsError || !strings.Contains(res.Text, tt.want) {
				t.Fatalf("result %+v", res)
			}
			if f.requests() != 0 {
				t.Fatalf("requests %d", f.requests())
			}
		})
	}
}

func TestSearchCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, f := searchServer(t, func(string) float64 { return 0.9 })
	env := treeEnv(t, map[string]string{"a.go": "x\n"})
	_, err := SearchTool(c).Run(ctx, env, json.RawMessage(`{"query":"q"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if f.requests() != 0 {
		t.Fatalf("requests %d", f.requests())
	}
}

func TestSearchCancelledBetweenPasses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeJev{}
	inner := f.handle(func(string) float64 { return 0.9 })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner(w, r)
		cancel()
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL, Key: "k", Model: "m"}
	env := treeEnv(t, map[string]string{"a.go": "x\n"})
	_, err := SearchTool(c).Run(ctx, env, json.RawMessage(`{"query":"q"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if f.count("excerpt") != 0 {
		t.Fatal("window pass ran after cancel")
	}
}
