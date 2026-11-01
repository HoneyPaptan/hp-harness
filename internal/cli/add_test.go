package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/mcp"
	"github.com/HoneyPaptan/hp-harness/internal/mcp/mcptest"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestMain(m *testing.M) {
	if mcptest.Active() {
		mcptest.Run()
		return
	}
	os.Exit(m.Run())
}

type addEnv struct {
	Env
	home           string
	stdout, stderr *bytes.Buffer
	vars           map[string]string
	mark           string
}

func newAddEnv(t *testing.T, fp *fake.Provider) *addEnv {
	t.Helper()
	home := t.TempDir()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	vars := map[string]string{
		"HOME": home, "XDG_STATE_HOME": filepath.Join(home, "state"), "PATH": os.Getenv("PATH"),
		"XAI_API_KEY": "key",
	}
	env := Env{
		Getenv: func(k string) string { return vars[k] }, Cwd: home,
		Stdout: stdout, Stderr: stderr, Host: host.Native{},
		Rand: bytes.NewReader(make([]byte, 64)), Version: "test",
		NewProvider: func(Endpoint) provider.Provider { return fp },
		Spawn:       func([]string, string) (int, error) { return 0, errors.New("spawn") },
		Alive:       func(int) bool { return false },
		Signal:      func(int) error { return nil },
		Pid:         1,
	}
	return &addEnv{Env: env, home: home, stdout: stdout, stderr: stderr, vars: vars, mark: filepath.Join(home, "marks")}
}

func (e *addEnv) run(args ...string) int {
	e.stdout.Reset()
	e.stderr.Reset()
	env := e.Env
	env.Args = args
	return Main(context.Background(), env)
}

func (e *addEnv) serverArgs(name string, extra ...string) []string {
	args := []string{"add", name, "--env", mcptest.EnvMode + "=1", "--env", mcptest.EnvMark + "=" + e.mark}
	args = append(args, extra...)
	return append(args, "--", os.Args[0])
}

func (e *addEnv) configFile(name string) string {
	return filepath.Join(e.home, ".config", "hp", name)
}

func (e *addEnv) starts(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(e.mark)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "start")
}

func TestAddStdioCachesAndStops(t *testing.T) {
	e := newAddEnv(t, nil)
	if code := e.run(e.serverArgs("fs")...); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if got := e.stdout.String(); got != "added fs: 9 tools\n" {
		t.Fatalf("stdout %q", got)
	}
	if e.starts(t) != 1 {
		t.Fatal("server should start once")
	}
	var cache schemaCache
	readTestJSON(t, e.configFile(schemaCacheFile), &cache)
	if len(cache.Servers["fs"]) != 9 || cache.Servers["fs"][0].Name != "fs__big" {
		t.Fatalf("cache %+v", cache)
	}
	data, _ := os.ReadFile(e.mark)
	var pid int
	fmt.Sscanf(string(data), "start %d", &pid)
	if _, err := os.Stat(fmt.Sprintf("/proc/%d/comm", pid)); err == nil && !procIsZombie(pid) {
		t.Fatalf("server %d still running", pid)
	}
}

func procIsZombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	return err != nil || strings.Contains(string(data), ") Z ")
}

func readTestJSON(t *testing.T, file string, into any) {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatal(err)
	}
}

func TestAddFilesArePrivate(t *testing.T) {
	e := newAddEnv(t, nil)
	if code := e.run(e.serverArgs("fs")...); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	for _, name := range []string{connectionsFile, schemaCacheFile} {
		info, err := os.Stat(e.configFile(name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %v", name, info.Mode())
		}
	}
	ents, _ := os.ReadDir(filepath.Dir(e.configFile(connectionsFile)))
	for _, ent := range ents {
		if strings.HasPrefix(ent.Name(), ".tmp") {
			t.Fatalf("temp file left: %s", ent.Name())
		}
	}
}

func TestFailedAddSavesNothing(t *testing.T) {
	e := newAddEnv(t, nil)
	args := e.serverArgs("fs", "--env", mcptest.EnvFail+"=boom on stderr")
	if code := e.run(args...); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(e.stderr.String(), "boom on stderr") {
		t.Fatalf("stderr %q", e.stderr)
	}
	for _, name := range []string{connectionsFile, schemaCacheFile} {
		if _, err := os.Stat(e.configFile(name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s written: %v", name, err)
		}
	}
}

func TestSecretsStoredOnlyAsReferences(t *testing.T) {
	e := newAddEnv(t, nil)
	e.vars["MY_TOKEN"] = "sk-very-secret"
	var got string
	srv := httptest.NewServer(mcpHandler(func(r *http.Request) { got = r.Header.Get("Authorization") }))
	defer srv.Close()
	if code := e.run("add", "web", srv.URL, "--header", "Authorization=Bearer $MY_TOKEN"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if got != "Bearer sk-very-secret" {
		t.Fatalf("header %q", got)
	}
	if code := e.run(e.serverArgs("fs", "--env", "API_KEY=$MY_TOKEN")...); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	var set connectionSet
	readTestJSON(t, e.configFile(connectionsFile), &set)
	if set.Servers["web"].Headers["Authorization"] != "Bearer $MY_TOKEN" || set.Servers["fs"].Env["API_KEY"] != "$MY_TOKEN" {
		t.Fatalf("set %+v", set)
	}
	for _, name := range []string{connectionsFile, schemaCacheFile} {
		data, _ := os.ReadFile(e.configFile(name))
		if strings.Contains(string(data), "sk-very-secret") {
			t.Fatalf("%s holds the secret", name)
		}
	}
}

func mcpHandler(seen func(*http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen(r)
		body, _ := io.ReadAll(r.Body)
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		json.Unmarshal(body, &m)
		if len(m.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mcptest.Response(m.ID, m.Method, m.Params, nil))
	})
}

func TestMissingReferenceFailsAdd(t *testing.T) {
	e := newAddEnv(t, nil)
	if code := e.run(e.serverArgs("fs", "--env", "API_KEY=$NOT_SET")...); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(e.stderr.String(), "$NOT_SET is not set") {
		t.Fatalf("stderr %q", e.stderr)
	}
}

func TestServerEnvIsNotInherited(t *testing.T) {
	e := newAddEnv(t, nil)
	cfg, err := clientConfig(e.Env, "fs", connection{Command: "x", Env: map[string]string{"A": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range cfg.Env {
		if strings.HasPrefix(kv, "XAI_API_KEY=") {
			t.Fatalf("provider key passed to server: %v", cfg.Env)
		}
	}
	if !strings.Contains(strings.Join(cfg.Env, "\n"), "A=1") {
		t.Fatalf("env %v", cfg.Env)
	}
}

func TestAddUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "nothing", args: []string{"add"}, want: "usage: hp add"},
		{name: "bad flag", args: []string{"add", "x", "--nope", "1"}, want: "unknown flag"},
		{name: "flag without value", args: []string{"add", "x", "--env"}, want: "needs a value"},
		{name: "not a pair", args: []string{"add", "x", "--env", "K", "--", "cmd"}, want: "is not K=V"},
		{name: "empty command", args: []string{"add", "x", "--"}, want: "usage: hp add"},
		{name: "no name", args: []string{"add", "--", "cmd"}, want: "usage: hp add"},
		{name: "env without command", args: []string{"add", "x", "--env", "K=V"}, want: "--env needs a command"},
		{name: "plain http", args: []string{"add", "x", "http://example.com/mcp"}, want: "must use https"},
		{name: "bad scheme", args: []string{"add", "x", "ftp://example.com"}, want: "must use https"},
		{name: "traversal", args: []string{"add", "../evil", "--", "cmd"}, want: "invalid name"},
		{name: "double underscore", args: []string{"add", "a__b", "--", "cmd"}, want: "invalid name"},
		{name: "trailing underscore", args: []string{"add", "a_", "--", "cmd"}, want: "invalid name"},
		{name: "long name", args: []string{"add", strings.Repeat("a", 33), "--", "cmd"}, want: "invalid name"},
		{name: "missing folder", args: []string{"add", "nope"}, want: "not a folder or git URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAddEnv(t, nil)
			if code := e.run(tt.args...); code != 2 {
				t.Fatalf("code %d %s", code, e.stderr)
			}
			if !strings.Contains(e.stderr.String(), tt.want) {
				t.Fatalf("stderr %q", e.stderr)
			}
		})
	}
}

func TestAddCommandKeepsItsFlags(t *testing.T) {
	a, err := parseAdd([]string{"fs", "--env", "A=1", "--", "node", "--inspect", "--", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(a.command, " "); got != "node --inspect -- x" {
		t.Fatalf("command %q", got)
	}
}

func TestAddDuplicateIsRefused(t *testing.T) {
	e := newAddEnv(t, nil)
	if code := e.run(e.serverArgs("fs")...); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if code := e.run(e.serverArgs("fs")...); code != 2 {
		t.Fatalf("code %d", code)
	}
	if e.starts(t) != 1 {
		t.Fatal("duplicate must not start the server")
	}
}

func TestListRemoveAndRefresh(t *testing.T) {
	e := newAddEnv(t, nil)
	if code := e.run(e.serverArgs("fs")...); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if code := e.run("list"); code != 0 || !strings.Contains(e.stdout.String(), "9 tools") {
		t.Fatalf("list %d %q", code, e.stdout)
	}
	if code := e.run("add", "--refresh", "fs"); code != 0 || e.stdout.String() != "refreshed fs: 9 tools\n" {
		t.Fatalf("refresh %d %q %s", code, e.stdout, e.stderr)
	}
	if code := e.run("add", "--refresh", "nope"); code != 1 {
		t.Fatalf("refresh unknown %d", code)
	}
	if code := e.run("remove", "fs"); code != 0 || e.stdout.String() != "removed fs\n" {
		t.Fatalf("remove %d %q", code, e.stdout)
	}
	if code := e.run("list"); code != 0 || e.stdout.String() != "" {
		t.Fatalf("list %d %q", code, e.stdout)
	}
	if code := e.run("remove", "fs"); code != 1 {
		t.Fatalf("second remove %d", code)
	}
	var cache schemaCache
	readTestJSON(t, e.configFile(schemaCacheFile), &cache)
	if _, ok := cache.Servers["fs"]; ok {
		t.Fatal("cache kept the removed server")
	}
}

func TestFailedRefreshKeepsTheCache(t *testing.T) {
	e := newAddEnv(t, nil)
	if code := e.run(e.serverArgs("fs")...); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	var set connectionSet
	readTestJSON(t, e.configFile(connectionsFile), &set)
	c := set.Servers["fs"]
	c.Env[mcptest.EnvFail] = "down"
	set.Servers["fs"] = c
	if err := saveConnections(filepath.Dir(e.configFile(connectionsFile)), set); err != nil {
		t.Fatal(err)
	}
	if code := e.run("add", "--refresh", "fs"); code != 1 {
		t.Fatalf("code %d", code)
	}
	var cache schemaCache
	readTestJSON(t, e.configFile(schemaCacheFile), &cache)
	if len(cache.Servers["fs"]) != 9 {
		t.Fatalf("cache %+v", cache)
	}
}

func TestMalformedConfigIsNotOverwritten(t *testing.T) {
	e := newAddEnv(t, nil)
	file := e.configFile(connectionsFile)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := e.run(e.serverArgs("fs")...); code != 1 {
		t.Fatalf("code %d", code)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "{broken" {
		t.Fatalf("config %q", data)
	}
}

func TestHomeMustBeAbsolute(t *testing.T) {
	for _, home := range []string{"", "relative"} {
		e := newAddEnv(t, nil)
		e.vars["HOME"] = home
		if code := e.run("list"); code != 1 {
			t.Fatalf("home %q code %d", home, code)
		}
	}
}

func writeSkill(t *testing.T, dir, frontmatter string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFile), []byte("---\n"+frontmatter+"\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAddFolderSkill(t *testing.T) {
	e := newAddEnv(t, nil)
	src := filepath.Join(e.home, "src", "my-skill")
	writeSkill(t, src, "description: does things")
	if err := os.MkdirAll(filepath.Join(src, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(e.home, "secret")
	if err := os.WriteFile(secret, []byte("hidden"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	if code := e.run("add", "src/my-skill"); code != 0 {
		t.Fatalf("code %d %s", code, e.stderr)
	}
	if e.stdout.String() != "added skill my-skill\n" {
		t.Fatalf("stdout %q", e.stdout)
	}
	dest := e.configFile(filepath.Join("skills", "my-skill"))
	info, err := os.Stat(filepath.Join(dest, "run.sh"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("run.sh %v %v", info, err)
	}
	for _, name := range []string{".git", "link"} {
		if _, err := os.Lstat(filepath.Join(dest, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s copied: %v", name, err)
		}
	}
	if code := e.run("list"); code != 0 || !strings.Contains(e.stdout.String(), "my-skill") {
		t.Fatalf("list %q", e.stdout)
	}
	if code := e.run("remove", "my-skill"); code != 0 {
		t.Fatalf("remove %d", code)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("still there: %v", err)
	}
}

func TestAddSkillRefusals(t *testing.T) {
	tests := []struct {
		name, frontmatter, want string
	}{
		{name: "no description", frontmatter: "name: x", want: "needs frontmatter"},
		{name: "traversal in name", frontmatter: "name: ../../evil\ndescription: d", want: "invalid name"},
		{name: "slash in name", frontmatter: "name: a/b\ndescription: d", want: "invalid name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAddEnv(t, nil)
			src := filepath.Join(e.home, "s")
			writeSkill(t, src, tt.frontmatter)
			if code := e.run("add", src); code == 0 {
				t.Fatal("want failure")
			}
			if !strings.Contains(e.stderr.String(), tt.want) {
				t.Fatalf("stderr %q", e.stderr)
			}
			if _, err := os.Stat(filepath.Join(e.home, "evil")); err == nil {
				t.Fatal("escaped the skills folder")
			}
		})
	}
}

func TestSymlinkedSkillFileIsRefused(t *testing.T) {
	e := newAddEnv(t, nil)
	src := filepath.Join(e.home, "s")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(src, skillFile)); err != nil {
		t.Fatal(err)
	}
	if code := e.run("add", src); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func gitRepo(t *testing.T, dir string) string {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "x"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return "file://" + dir
}

func TestAddGitSkill(t *testing.T) {
	tests := []struct {
		name   string
		layout []string
		want   string
		code   int
		stderr string
	}{
		{name: "root", layout: []string{"."}, want: "repo-skill"},
		{name: "one level down", layout: []string{"inner"}, want: "inner"},
		{name: "two skills", layout: []string{"a", "b"}, code: 1, stderr: "more than one skill"},
		{name: "too deep", layout: []string{"a/b"}, code: 1, stderr: "no SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAddEnv(t, nil)
			repo := filepath.Join(e.home, "repo-skill")
			for _, dir := range tt.layout {
				writeSkill(t, filepath.Join(repo, dir), "description: from git")
			}
			url := gitRepo(t, repo)
			if code := e.run("add", url); code != tt.code {
				t.Fatalf("code %d %s", code, e.stderr)
			}
			if tt.code != 0 {
				if !strings.Contains(e.stderr.String(), tt.stderr) {
					t.Fatalf("stderr %q", e.stderr)
				}
				return
			}
			dest := e.configFile(filepath.Join("skills", tt.want))
			if _, err := os.Stat(filepath.Join(dest, skillFile)); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dest, ".git")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf(".git copied: %v", err)
			}
		})
	}
}

func TestGitHelpersAreRefused(t *testing.T) {
	e := newAddEnv(t, nil)
	if err := cloneShallow(context.Background(), "ext::sh -c touch${IFS}"+filepath.Join(e.home, "pwned"), filepath.Join(e.home, "dest")); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(filepath.Join(e.home, "pwned")); err == nil {
		t.Fatal("remote helper ran")
	}
}

func TestExposeTools(t *testing.T) {
	long := strings.Repeat("a", 70)
	tests := []struct {
		name  string
		tools []string
		want  []string
	}{
		{name: "plain", tools: []string{"read"}, want: []string{"s__read"}},
		{name: "sanitised", tools: []string{"a.b/c d"}, want: []string{"s__a_b_c_d"}},
		{name: "collision", tools: []string{"a.b", "a_b", "a b"}, want: []string{"s__a_b", "s__a_b-2", "s__a_b-3"}},
		{name: "truncated", tools: []string{long}, want: []string{"s__" + long[:61]}},
		{name: "truncated collision", tools: []string{long, long + "b"}, want: []string{"s__" + long[:61], "s__" + long[:59] + "-2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := make([]mcp.Tool, len(tt.tools))
			for i, name := range tt.tools {
				in[i] = mcp.Tool{Name: name}
			}
			got := exposeTools("s", in)
			names := make([]string, len(got))
			for i, c := range got {
				names[i] = c.Name
				if len(c.Name) > maxToolName {
					t.Fatalf("too long: %s", c.Name)
				}
			}
			if strings.Join(names, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("names %v", names)
			}
		})
	}
}

func TestObjectSchema(t *testing.T) {
	tests := []struct {
		name, raw, want string
	}{
		{name: "missing", raw: "", want: `{"properties":{},"type":"object"}`},
		{name: "no properties", raw: `{"type":"object"}`, want: `{"properties":{},"type":"object"}`},
		{name: "not an object", raw: `true`, want: `{"properties":{},"type":"object"}`},
		{name: "kept", raw: `{"type":"object","properties":{"a":{"type":"string"}}}`, want: `{"properties":{"a":{"type":"string"}},"type":"object"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(objectSchema(json.RawMessage(tt.raw))); got != tt.want {
				t.Fatalf("got %s", got)
			}
		})
	}
}
