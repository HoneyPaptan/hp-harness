package permit

import "testing"

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
