package crew

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunRefusals(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		git    func(string, ...string) (string, error)
		code   int
		stderr string
	}{
		{"zero agents", []string{"crew", "run", "-n", "0", "plan.md"}, nil, 2, "-n must be at least 1"},
		{"negative agents", []string{"crew", "run", "-n", "-3", "plan.md"}, nil, 2, "-n must be at least 1"},
		{"no commit yet", []string{"crew", "run", "plan.md"}, func(_ string, args ...string) (string, error) {
			if args[len(args)-1] == "HEAD" {
				return "", errors.New("fatal: ambiguous argument 'HEAD'")
			}
			return "/repo\n", nil
		}, 1, "needs a commit in this repository"},
		{"not a repository", []string{"crew", "run", "plan.md"}, func(string, ...string) (string, error) {
			return "", errors.New("fatal: not a git repository")
		}, 1, "needs a git repository here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, getenv, _ := serviceEnv(t)
			writePlan(t, env.Cwd, "plan.md", "## one\nfirst\n")
			if tc.git != nil {
				env.Git = tc.git
			}
			var out, errOut bytes.Buffer
			env.Stdout, env.Stderr = &out, &errOut
			if code := Main(context.Background(), env, tc.args); code != tc.code {
				t.Fatalf("code %d want %d, stderr %q", code, tc.code, errOut.String())
			}
			if !strings.Contains(errOut.String(), tc.stderr) || strings.Contains(errOut.String(), "fatal") {
				t.Fatalf("stderr %q", errOut.String())
			}
			if out.Len() != 0 || len(crewIDs(Env{Getenv: getenv})) != 0 {
				t.Fatalf("a crew was created: %q", out.String())
			}
		})
	}
}
