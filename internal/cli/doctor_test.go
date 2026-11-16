package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

type doctorStub struct {
	fake.Provider
	ids []string
	err error
}

func (s *doctorStub) ListModels(ctx context.Context) ([]string, error) {
	if s.err == context.DeadlineExceeded {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.ids, s.err
}

func runDoctor(t *testing.T, extra map[string]string, stubs map[string]*doctorStub) (int, string) {
	t.Helper()
	h := host.NewMemory()
	h.ExecFunc = func(context.Context, host.ExecRequest) (host.ExecResult, error) {
		return host.ExecResult{Output: []byte("git version 2.40.0\n")}, nil
	}
	env, stdout, stderr := testEnv(t, h, &fake.Provider{}, extra)
	env.NewProvider = func(ep Endpoint) provider.Provider {
		if s, ok := stubs[ep.Preset]; ok {
			return s
		}
		return &doctorStub{err: errors.New("dial tcp 127.0.0.1:11434: connect: connection refused")}
	}
	env.Args = []string{"doctor"}
	code := Main(context.Background(), env)
	return code, stdout.String() + stderr.String()
}

func TestDoctorReportsAWorkingSetup(t *testing.T) {
	stubs := map[string]*doctorStub{"xai": {ids: []string{"grok-a", "grok-b"}}}
	code, out := runDoctor(t, map[string]string{"XAI_API_KEY": "sk-secret-value"}, stubs)
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	for _, want := range []string{
		"[ok] version: hp dev",
		"[ok] settings:",
		"[ok] config folder:",
		"[ok] git: git version 2.40.0",
		"[ok] keys: XAI_API_KEY (env)",
		"[ok] xai: https://api.x.ai/v1 answered, 2 models",
		"[warn] ollama:",
		"fix: start ollama",
		"hp doctor: ok=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "sk-secret-value") {
		t.Fatalf("key value printed:\n%s", out)
	}
}

func TestDoctorFailsWithNothingUsable(t *testing.T) {
	code, out := runDoctor(t, map[string]string{"XAI_API_KEY": ""}, nil)
	if code != 1 {
		t.Fatalf("code %d\n%s", code, out)
	}
	for _, want := range []string{
		"[warn] keys: no provider key is set",
		"fix: hp key set GROQ_API_KEY",
		"[fail] providers: no provider answered",
		"fail=1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestDoctorFailsOnAModelWithoutItsKey(t *testing.T) {
	code, out := runDoctor(t, map[string]string{"HP_MODEL": "groq/openai/gpt-oss-120b"}, map[string]*doctorStub{"xai": {ids: []string{"a"}}})
	if code != 1 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !strings.Contains(out, "[fail] HP_MODEL: groq/openai/gpt-oss-120b:") || !strings.Contains(out, "fix: hp key set GROQ_API_KEY") {
		t.Fatalf("output\n%s", out)
	}
}

func TestDoctorSaysWhenAKeyIsRefused(t *testing.T) {
	stubs := map[string]*doctorStub{"xai": {err: &provider.HTTPError{Status: 401, Body: "Incorrect API key sk-leaked"}}}
	code, out := runDoctor(t, nil, stubs)
	if code != 1 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !strings.Contains(out, "[fail] xai: https://api.x.ai/v1 refused the key (HTTP 401)") || !strings.Contains(out, "fix: hp key set XAI_API_KEY with a valid key") {
		t.Fatalf("output\n%s", out)
	}
	if strings.Contains(out, "sk-leaked") {
		t.Fatalf("response body printed:\n%s", out)
	}
}

func TestDoctorStopsWaitingAtTheNetworkBudget(t *testing.T) {
	old := doctorNetworkBudget
	doctorNetworkBudget = 30 * time.Millisecond
	t.Cleanup(func() { doctorNetworkBudget = old })
	code, out := runDoctor(t, nil, map[string]*doctorStub{"xai": {err: context.DeadlineExceeded}})
	if code != 1 || !strings.Contains(out, "did not answer in 30ms") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestDoctorFailsOnBrokenSettings(t *testing.T) {
	home := t.TempDir()
	env, stdout, _ := testEnv(t, host.NewMemory(), &fake.Provider{}, map[string]string{"HOME": home})
	file, err := settingsPath(rawGetenv(env))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	env.NewProvider = func(Endpoint) provider.Provider { return &doctorStub{ids: []string{"a"}} }
	env.Args = []string{"doctor"}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "[fail] settings:") || !strings.Contains(stdout.String(), "fix: edit or remove "+file) {
		t.Fatalf("output\n%s", stdout.String())
	}
}

func TestDoctorKeySourceNamesTheFile(t *testing.T) {
	env, _, _ := testEnv(t, host.NewMemory(), &fake.Provider{}, nil)
	real := getenvMap(map[string]string{"HOME": "/home/t"})
	env.realGetenv = real
	env.Getenv = getenvMap(map[string]string{"HOME": "/home/t", "GROQ_API_KEY": "from-file", "XAI_API_KEY": "from-env"})
	check := doctorKeys(env)
	if check.status != doctorOK || !strings.Contains(check.msg, "GROQ_API_KEY (/home/t/.config/hp/env)") {
		t.Fatalf("check %+v", check)
	}
}

func TestDoctorTakesNoArguments(t *testing.T) {
	env, _, stderr := testEnv(t, host.NewMemory(), nil, nil)
	env.Args = []string{"doctor", "now"}
	if code := Main(context.Background(), env); code != 2 {
		t.Fatalf("code %d %s", code, stderr.String())
	}
}
