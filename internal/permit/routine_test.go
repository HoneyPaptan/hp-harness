package permit

import (
	"strings"
	"testing"
)

func TestBashRoutine(t *testing.T) {
	dir := "/w"
	home := "/home/u"
	for _, cmd := range routineCommands {
		a := Action{Tool: "bash", Target: cmd, Kind: KindExec}
		if !Routine(a, dir, home) {
			t.Errorf("routine command %q", cmd)
		}
	}
	extra := []string{
		"go test ./...",
		"git status -sb",
		"make check",
		"make build",
		"npm run test",
		"npm run build",
		"pnpm run check",
		"find . -name '*.go'",
	}
	for _, cmd := range extra {
		a := Action{Tool: "bash", Target: cmd, Kind: KindExec}
		if !Routine(a, dir, home) {
			t.Errorf("expected routine %q", cmd)
		}
	}
	more := []string{"rg foo", "git log --oneline", "git branch", "go test ./..."}
	for _, cmd := range more {
		a := Action{Tool: "bash", Target: cmd, Kind: KindExec}
		if !Routine(a, dir, home) {
			t.Errorf("expected routine %q", cmd)
		}
	}
}

func TestBashNotRoutine(t *testing.T) {
	dir := "/w"
	home := "/home/u"
	cmds := []string{
		"go test ./... && rm -rf ~",
		"ls | sh",
		`git diff "--ext-diff"`,
		"git diff --ex''t-diff",
		`rg --p\re=sh x`,
		"cat x > y",
		"echo $(id)",
		"echo `id`",
		"find . -delete",
		"find . -exec rm {} \\;",
		"find . -fprint out",
		"find . -okdir rm",
		"git push",
		"make deploy",
		"make install",
		"make test x",
		"npm run release",
		"rg --pre=sh x",
		"grep --pre-glob='*.sh' x",
		"git diff --ext-diff",
		"git log --output=/tmp/x",
		"git -c core.pager=sh log",
		"git branch -D main",
		"go test -exec sh ./...",
		"go build -o /usr/bin/x",
		"find . -fprint0 x",
		"cat $HOME/x",
	}
	for _, cmd := range cmds {
		a := Action{Tool: "bash", Target: cmd, Kind: KindExec}
		if Routine(a, dir, home) {
			t.Errorf("expected not routine %q", cmd)
		}
	}
}

func TestWriteRoutine(t *testing.T) {
	dir := "/w"
	home := "/home/u"
	ok := []string{"internal/x.go"}
	for _, target := range ok {
		a := Action{Tool: "write", Target: target, Kind: KindWrite}
		if !Routine(a, dir, home) {
			t.Errorf("expected routine write %q", target)
		}
	}
	bad := []string{"../x", "/etc/passwd", ".env", ".git/config", "a/../../x"}
	for _, target := range bad {
		a := Action{Tool: "write", Target: target, Kind: KindWrite}
		if Routine(a, dir, home) {
			t.Errorf("expected not routine write %q", target)
		}
	}
}

func TestWriteLinkedNotRoutine(t *testing.T) {
	a := Action{Tool: "write", Target: "x.go", Kind: KindWrite, Linked: true}
	if Routine(a, "/w", "/home/u") {
		t.Fatal("linked write should not be routine")
	}
}

func TestReadAlwaysRoutine(t *testing.T) {
	dir := "/w"
	home := "/home/u"
	a := Action{Tool: "read", Target: "/etc/passwd", Kind: KindRead}
	if !Routine(a, dir, home) {
		t.Fatal("read should be routine")
	}
	a = Action{Tool: "read", Target: ".env", Kind: KindRead}
	if !Routine(a, dir, home) {
		t.Fatal("read .env should still be routine")
	}
}

func TestMCPNotRoutine(t *testing.T) {
	a := Action{Tool: "mcp", Target: "x__y", Kind: KindExternal}
	if Routine(a, "/w", "/home/u") {
		t.Fatal("mcp should not be routine")
	}
}

func TestShellWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
		ok   bool
	}{
		{"git log --oneline", []string{"git", "log", "--oneline"}, true},
		{`find . -name '*.go'`, []string{"find", ".", "-name", "*.go"}, true},
		{`git diff "--ext-diff"`, []string{"git", "diff", "--ext-diff"}, true},
		{"git diff --ex''t-diff", []string{"git", "diff", "--ext-diff"}, true},
		{`a "b c"`, []string{"a", "b c"}, true},
		{`a "b`, nil, false},
		{"git diff --{ext-diff,x}", nil, false},
		{"rg --pr* x", nil, false},
		{"ls [a]", nil, false},
		{"ls a?", nil, false},
		{`find . -name "*.go"`, []string{"find", ".", "-name", "*.go"}, true},
		{"", nil, true},
	}
	for _, tc := range tests {
		got, ok := shellWords(tc.in)
		if ok != tc.ok || strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Fatalf("%q: got %q %v", tc.in, got, ok)
		}
	}
}
