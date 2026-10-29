package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestRunsEmpty(t *testing.T) {
	h := host.NewMemory()
	env, stdout, _ := testEnv(t, h, nil, nil)
	env.Args = []string{"runs"}
	if Main(nilCtx(), env) != 0 {
		t.Fatal()
	}
	if stdout.String() != "no runs yet\n" {
		t.Fatalf("%q", stdout.String())
	}
}

func nilCtx() context.Context {
	return context.Background()
}

func TestRunsList(t *testing.T) {
	h := host.NewMemory()
	env, stdout, _ := testEnv(t, h, nil, nil)
	seedRun(t, h, env.Getenv, "20260101-100000-aaaa", "short prompt here", "final", true)
	seedRun(t, h, env.Getenv, "20260102-100000-bbbb", "second run prompt text", "", false)
	env.Args = []string{"runs"}
	if Main(nilCtx(), env) != 0 {
		t.Fatal()
	}
	out := stdout.String()
	if !strings.HasPrefix(out, "20260102-100000-bbbb") {
		t.Fatalf("order:\n%s", out)
	}
	if !strings.Contains(out, "resumable") || !strings.Contains(out, "final") {
		t.Fatalf("%s", out)
	}
	if !strings.Contains(out, "xai/m") {
		t.Fatalf("%s", out)
	}
	long := strings.Repeat("x", 60)
	seedRun(t, h, env.Getenv, "20260103-100000-cccc", long, "final", true)
	stdout = &bytes.Buffer{}
	env.Stdout = stdout
	Main(nilCtx(), env)
	if strings.Contains(stdout.String(), long) {
		t.Fatal("prompt not cut")
	}
}

func TestVersion(t *testing.T) {
	h := host.NewMemory()
	env, stdout, _ := testEnv(t, h, nil, nil)
	env.Version = "dev"
	env.Args = []string{"version"}
	if Main(nilCtx(), env) != 0 {
		t.Fatal()
	}
	if stdout.String() != "hp dev\n" {
		t.Fatalf("%q", stdout.String())
	}
	stdout.Reset()
	env.Args = []string{"--version"}
	if Main(nilCtx(), env) != 0 {
		t.Fatal()
	}
	if stdout.String() != "hp dev\n" {
		t.Fatalf("%q", stdout.String())
	}
}
