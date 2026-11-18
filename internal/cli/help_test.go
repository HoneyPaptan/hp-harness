package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestCommandHelpExitsZero(t *testing.T) {
	for _, c := range commandHelps {
		if c.name == "crew" || c.name == "crews" {
			continue
		}
		for _, flagName := range []string{"-h", "--help"} {
			t.Run(c.name+" "+flagName, func(t *testing.T) {
				env, stdout, stderr := testEnv(t, host.NewMemory(), nil, nil)
				env.Args = []string{c.name, flagName}
				if code := Main(context.Background(), env); code != 0 {
					t.Fatalf("code %d, stderr %q", code, stderr.String())
				}
				out := stdout.String() + stderr.String()
				if !strings.Contains(out, "usage: hp "+c.name) {
					t.Fatalf("no usage line in %q", out)
				}
				if strings.Contains(out, "error") || strings.Contains(out, "help requested") {
					t.Fatalf("help reported an error: %q", out)
				}
			})
		}
	}
}

func TestRunHelpListsFlags(t *testing.T) {
	env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
	env.Args = []string{"run", "--help"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{"usage: hp run [flags] [PROMPT | -]", "-m string", "-fallback value", "-d"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("missing %q in %q", want, stderr.String())
		}
	}
}

func TestGlobalHelpDescribesEveryCommand(t *testing.T) {
	env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
	env.Args = []string{"--help"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d", code)
	}
	for _, c := range commandHelps {
		if !strings.Contains(stderr.String(), c.about) {
			t.Fatalf("help does not describe %s: %q", c.name, stderr.String())
		}
	}
}

func TestUnknownRunIDsReadTheSame(t *testing.T) {
	for _, args := range [][]string{
		{"attach", "ghost"},
		{"resume", "ghost"},
		{"resume", "-d", "ghost"},
		{"stop", "ghost"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
			env.Args = args
			if code := Main(context.Background(), env); code != 2 {
				t.Fatalf("code %d", code)
			}
			if got := stderr.String(); got != "error: run \"ghost\" not found\n" {
				t.Fatalf("stderr %q", got)
			}
		})
	}
}

func TestWebRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"99999", "-1"} {
		t.Run(port, func(t *testing.T) {
			env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
			env.Args = []string{"web", "-port=" + port}
			if code := Main(context.Background(), env); code != 2 {
				t.Fatalf("code %d", code)
			}
			if got := stderr.String(); !strings.Contains(got, "invalid port "+port) || strings.Contains(got, "taken") {
				t.Fatalf("stderr %q", got)
			}
		})
	}
}

func TestRetryShowsTheCauseOnce(t *testing.T) {
	refused := errors.New("dial tcp 127.0.0.1:11434: connect: connection refused")
	cut := func() fake.Turn { return fake.Turn{Err: &provider.TransientError{Err: refused}} }
	fp := &fake.Provider{Turns: []fake.Turn{cut(), cut(), fake.TextTurn("ok")}}
	env, _, stderr := testEnv(t, host.NewMemory(), fp, nil)
	env.Args = []string{"run", "-m", "xai/grok", "go"}
	if code := Main(context.Background(), env); code != 0 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
	got := stderr.String()
	want := "stream failed: dial tcp 127.0.0.1:11434: connect: connection refused\nstream failed, retrying in 2s (attempt 1/5)\n"
	if !strings.Contains(got, want) {
		t.Fatalf("stderr %q", got)
	}
	if n := strings.Count(got, "connection refused"); n != 1 {
		t.Fatalf("cause shown %d times: %q", n, got)
	}
}

func TestSessionWithoutTerminalSaysSo(t *testing.T) {
	env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
	env.Args = nil
	if code := Main(context.Background(), env); code != 2 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "needs a terminal") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestUnknownCommandLine(t *testing.T) {
	cases := map[string]string{
		"rnu":        `unknown command "rnu", did you mean "run"?`,
		"crewz":      `unknown command "crewz", did you mean "crew"?`,
		"frobnicate": `unknown command "frobnicate". hp help`,
	}
	for name, want := range cases {
		if got := unknownCommandLine(name); !strings.HasPrefix(got, want) {
			t.Errorf("%s: %q", name, got)
		}
	}
}
