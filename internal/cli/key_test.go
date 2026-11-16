package cli

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func keyTestEnv(t *testing.T, vars map[string]string, term Terminal) (Env, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	home := t.TempDir()
	merged := map[string]string{"HOME": home}
	for k, v := range vars {
		merged[k] = v
	}
	env, stdout, stderr := testEnv(t, host.NewMemory(), nil, merged)
	env.Term = term
	return env, stdout, stderr, home
}

func keyTestFile(t *testing.T, home, content string, mode fs.FileMode) string {
	t.Helper()
	file := path.Join(home, ".config", "hp", "env")
	if err := os.MkdirAll(path.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, mode); err != nil {
		t.Fatal(err)
	}
	return file
}

func readKeyTestFile(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestKeySet(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		content    string
		term       *fakeTerm
		wantCode   int
		wantFile   string
		wantStdout string
		wantStderr string
	}{
		{
			name: "new key", args: []string{"key", "set", "XAI_API_KEY"},
			term: &fakeTerm{secret: "sekrit"}, wantCode: 0,
			wantFile: "XAI_API_KEY=sekrit\n", wantStdout: "set XAI_API_KEY\n",
			wantStderr: "value for XAI_API_KEY:",
		},
		{
			name: "overwrite key", args: []string{"key", "set", "XAI_API_KEY"},
			content: "OTHER=one\nXAI_API_KEY=old\n", term: &fakeTerm{secret: "new"}, wantCode: 0,
			wantFile: "OTHER=one\nXAI_API_KEY=new\n", wantStdout: "set XAI_API_KEY\n",
		},
		{
			name: "piped value", args: []string{"key", "set", "XAI_API_KEY"},
			term: &fakeTerm{piped: true, input: strings.NewReader("piped-secret\n")}, wantCode: 0,
			wantFile: "XAI_API_KEY=piped-secret\n", wantStdout: "set XAI_API_KEY\n",
		},
		{
			name: "bad name", args: []string{"key", "set", "xai_api_key"},
			term: &fakeTerm{secret: "sekrit"}, wantCode: 2, wantFile: "",
			wantStderr: "invalid key name",
		},
		{
			name: "empty piped value", args: []string{"key", "set", "XAI_API_KEY"},
			term: &fakeTerm{piped: true, input: strings.NewReader("")}, wantCode: 2, wantFile: "",
			wantStderr: "must not be empty",
		},
		{
			name: "newline value", args: []string{"key", "set", "XAI_API_KEY"},
			term: &fakeTerm{secret: "a\nb"}, wantCode: 2, wantFile: "",
			wantStderr: "newline",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, stdout, stderr, home := keyTestEnv(t, nil, tc.term)
			file := path.Join(home, ".config", "hp", "env")
			if tc.content != "" {
				keyTestFile(t, home, tc.content, 0o600)
			}
			env.Args = tc.args
			if code := Main(context.Background(), env); code != tc.wantCode {
				t.Fatalf("code %d want %d, stderr %q", code, tc.wantCode, stderr.String())
			}
			if tc.wantFile == "" {
				if _, err := os.Stat(file); err == nil {
					t.Fatalf("file was written")
				}
			} else {
				if got := readKeyTestFile(t, file); got != tc.wantFile {
					t.Fatalf("file %q want %q", got, tc.wantFile)
				}
				info, err := os.Stat(file)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0o600 {
					t.Fatalf("mode %v", info.Mode().Perm())
				}
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Fatalf("stdout %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Fatalf("stderr %q", stderr.String())
			}
		})
	}
}

func TestKeyRemove(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		content    string
		wantCode   int
		wantFile   string
		wantStdout string
		wantStderr string
	}{
		{
			name: "removes one line", args: []string{"key", "rm", "A_KEY"},
			content: "A_KEY=1\nB_KEY=2\n", wantCode: 0,
			wantFile: "B_KEY=2\n", wantStdout: "removed A_KEY\n",
		},
		{
			name: "missing key", args: []string{"key", "rm", "C_KEY"},
			content: "A_KEY=1\n", wantCode: 1,
			wantFile: "A_KEY=1\n", wantStderr: "C_KEY is not in",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, stdout, stderr, home := keyTestEnv(t, nil, nil)
			file := keyTestFile(t, home, tc.content, 0o600)
			env.Args = tc.args
			if code := Main(context.Background(), env); code != tc.wantCode {
				t.Fatalf("code %d want %d, stderr %q", code, tc.wantCode, stderr.String())
			}
			if got := readKeyTestFile(t, file); got != tc.wantFile {
				t.Fatalf("file %q want %q", got, tc.wantFile)
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Fatalf("stdout %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Fatalf("stderr %q", stderr.String())
			}
		})
	}
}

func TestKeyList(t *testing.T) {
	cases := []struct {
		name    string
		content string
		mode    fs.FileMode
		vars    map[string]string
		wantOut []string
		notOut  []string
		wantErr []string
		notErr  []string
	}{
		{
			name: "names and sources", content: "XAI_API_KEY=file-secret\nCUSTOM_KEY=custom-secret\n",
			mode: 0o600, vars: map[string]string{"XAI_API_KEY": "", "OPENAI_API_KEY": "env-secret"},
			wantOut: []string{"XAI_API_KEY", "file", "CUSTOM_KEY", "file", "OPENAI_API_KEY", "env"},
			notOut:  []string{"file-secret", "custom-secret", "env-secret"},
			notErr:  []string{"note:"},
		},
		{
			name: "env wins", content: "XAI_API_KEY=file-secret\n",
			mode: 0o600, vars: map[string]string{"XAI_API_KEY": "env-secret"},
			wantOut: []string{"XAI_API_KEY", "env"}, notOut: []string{"file-secret", "env-secret"},
		},
		{
			name: "wide mode note", content: "XAI_API_KEY=file-secret\n",
			mode: 0o644, vars: map[string]string{"XAI_API_KEY": ""},
			wantOut: []string{"XAI_API_KEY", "file"}, notOut: []string{"file-secret"},
			wantErr: []string{"note:", "wider than 0600"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, stdout, stderr, home := keyTestEnv(t, tc.vars, nil)
			keyTestFile(t, home, tc.content, tc.mode)
			env.Args = []string{"key", "list"}
			if code := Main(context.Background(), env); code != 0 {
				t.Fatalf("code %d, stderr %q", code, stderr.String())
			}
			for _, want := range tc.wantOut {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout %q missing %q", stdout.String(), want)
				}
			}
			for _, bad := range tc.notOut {
				if strings.Contains(stdout.String(), bad) {
					t.Fatalf("stdout %q has %q", stdout.String(), bad)
				}
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr %q missing %q", stderr.String(), want)
				}
			}
			for _, bad := range tc.notErr {
				if strings.Contains(stderr.String(), bad) {
					t.Fatalf("stderr %q has %q", stderr.String(), bad)
				}
			}
		})
	}
}

func TestKeyFileReachesCommands(t *testing.T) {
	vars := map[string]string{"XAI_API_KEY": ""}
	env, stdout, stderr, home := keyTestEnv(t, vars, nil)
	keyTestFile(t, home, "XAI_API_KEY=from-file\n", 0o600)
	env.Args = []string{"providers"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "XAI_API_KEY set") {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestKeyFileEnvWins(t *testing.T) {
	env, _, _, home := keyTestEnv(t, map[string]string{"XAI_API_KEY": "env-secret"}, nil)
	keyTestFile(t, home, "XAI_API_KEY=file-secret\n", 0o600)
	applied := applyKeyFile(env)
	if got := applied.Getenv("XAI_API_KEY"); got != "env-secret" {
		t.Fatalf("value %q", got)
	}
	if got := applied.Getenv("MISSING_KEY"); got != "" {
		t.Fatalf("missing value %q", got)
	}
}

func TestParseKeyFile(t *testing.T) {
	cases := []struct {
		name   string
		data   string
		names  []string
		values map[string]string
	}{
		{
			name: "skips blanks and comments", data: "# comment\n\nXAI_API_KEY=one\n",
			names: []string{"XAI_API_KEY"}, values: map[string]string{"XAI_API_KEY": "one"},
		},
		{
			name: "last value wins", data: "A_KEY=one\nA_KEY=two\n",
			names: []string{"A_KEY"}, values: map[string]string{"A_KEY": "two"},
		},
		{
			name: "drops invalid lines", data: "lower=one\nNO_VALUE=\nB_KEY=two\n",
			names: []string{"B_KEY"}, values: map[string]string{"B_KEY": "two"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kf := parseKeyFile([]byte(tc.data))
			if strings.Join(kf.names, ",") != strings.Join(tc.names, ",") {
				t.Fatalf("names %v want %v", kf.names, tc.names)
			}
			for name, want := range tc.values {
				if got := kf.values[name]; got != want {
					t.Fatalf("%s %q want %q", name, got, want)
				}
			}
		})
	}
}

func TestCheckKeyValue(t *testing.T) {
	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		{name: "plain", value: "sk-abc", ok: true},
		{name: "equals", value: "a=b", ok: true},
		{name: "empty", value: ""},
		{name: "newline", value: "a\nb"},
		{name: "carriage return", value: "a\rb"},
		{name: "nul", value: "a\x00b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkKeyValue(tc.value); (err == nil) != tc.ok {
				t.Fatalf("err %v want ok %v", err, tc.ok)
			}
		})
	}
}

func TestValidKeyName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"GROQ_API_KEY", true},
		{"_X", true},
		{"A1", true},
		{"", false},
		{"1A", false},
		{"lower", false},
		{"A-B", false},
		{"A B", false},
	}
	for _, tc := range cases {
		if got := validKeyName(tc.name); got != tc.want {
			t.Errorf("validKeyName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
