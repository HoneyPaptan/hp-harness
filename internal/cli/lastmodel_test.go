package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func quitSession(t *testing.T, term *fakeTerm, code <-chan int) {
	t.Helper()
	term.send(t, "\x04")
	if c := awaitCode(t, code); c != 0 {
		t.Fatalf("code %d", c)
	}
}

func TestLastModelSavesOnlyTheSpecInTheStateDir(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, &fake.Provider{}, nil)
	if err := saveLastModel(env, "opencode-go/hy4-flash-free"); err != nil {
		t.Fatal(err)
	}
	data, err := h.ReadFile("/state/hp/last-model")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "opencode-go/hy4-flash-free\n" {
		t.Fatalf("file = %q", data)
	}
	if got := loadLastModel(env); got != "opencode-go/hy4-flash-free" {
		t.Fatalf("loaded %q", got)
	}
	if _, err := h.ReadFile("/home/t/.config/hp/last-model"); err == nil {
		t.Fatal("saved next to the keys")
	}
}

func TestLastModelPathNeedsAnAbsoluteBase(t *testing.T) {
	if got := lastModelPath(getenvMap(nil)); got != "" {
		t.Fatalf("path %q", got)
	}
	if got := lastModelPath(getenvMap(map[string]string{"HOME": "/home/t"})); got != "/home/t/.local/state/hp/last-model" {
		t.Fatalf("path %q", got)
	}
}

func TestLoadLastModelIgnoresJunk(t *testing.T) {
	cases := []struct{ name, data, want string }{
		{"missing", "", ""},
		{"no slash", "grok\n", ""},
		{"first line only", "groq/fast\nextra\n", "groq/fast"},
		{"padded", "  groq/fast  \n", "groq/fast"},
		{"too long", "groq/" + strings.Repeat("a", lastModelMaxLen) + "\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			env, _, _ := testEnv(t, h, &fake.Provider{}, nil)
			if tc.data != "" {
				if err := h.WriteFile("/state/hp/last-model", []byte(tc.data)); err != nil {
					t.Fatal(err)
				}
			}
			if got := loadLastModel(env); got != tc.want {
				t.Fatalf("loaded %q want %q", got, tc.want)
			}
		})
	}
}

func TestApplyLastModelPrecedence(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, &fake.Provider{}, map[string]string{"GROQ_API_KEY": "g"})
	if err := saveLastModel(env, "groq/fast"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   runOpts
		want string
	}{
		{"nothing given", runOpts{}, "groq/fast"},
		{"model flag wins", runOpts{model: "xai/grok"}, "xai/grok"},
		{"base url flag keeps old behaviour", runOpts{baseURL: "http://x/v1"}, ""},
		{"key env flag keeps old behaviour", runOpts{keyEnv: "XAI_API_KEY"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, note := applyLastModel(env, tc.in)
			if got.model != tc.want || note != "" {
				t.Fatalf("model %q note %q", got.model, note)
			}
		})
	}
}

func TestApplyLastModelSkipsWhenTheKeyIsGone(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, &fake.Provider{}, nil)
	if err := saveLastModel(env, "groq/fast"); err != nil {
		t.Fatal(err)
	}
	got, note := applyLastModel(env, runOpts{})
	if got.model != "" || !strings.Contains(note, "groq/fast") || !strings.Contains(note, "GROQ_API_KEY is not set") || strings.Contains(note, "\n") {
		t.Fatalf("model %q note %q", got.model, note)
	}
}

func TestSessionPickIsUsedByTheNextBareSession(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("one"), fake.TextTurn("two")}}
	env, _, _ := testEnv(t, h, fp, map[string]string{"GROQ_API_KEY": "g", "HP_MODEL": "xai/grok"})
	term := newFakeTerm(t)
	env.Term = term
	code := startHP(t, env)
	term.send(t, "/model groq/fast\r")
	term.await(t, "model groq/fast", 1)
	quitSession(t, term, code)

	term = newFakeTerm(t)
	env.Term = term
	code = startHP(t, env)
	term.await(t, "groq/fast", 1)
	term.send(t, "again\r")
	term.await(t, "0 steps", 1)
	quitSession(t, term, code)
	if len(fp.Requests) != 1 || fp.Requests[0].Model != "fast" {
		t.Fatalf("requests %+v", fp.Requests)
	}
}

func TestSessionModelFlagBeatsTheLastPick(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("one")}}
	env, _, _ := testEnv(t, h, fp, map[string]string{"GROQ_API_KEY": "g"})
	if err := saveLastModel(env, "groq/fast"); err != nil {
		t.Fatal(err)
	}
	term := newFakeTerm(t)
	env.Term = term
	env.Args = []string{"-m", "xai/grok"}
	code := startHP(t, env)
	term.send(t, "go\r")
	term.await(t, "0 steps", 1)
	quitSession(t, term, code)
	if fp.Requests[0].Model != "grok" {
		t.Fatalf("model %q", fp.Requests[0].Model)
	}
	if got := loadLastModel(env); got != "groq/fast" {
		t.Fatalf("flag changed the saved model to %q", got)
	}
}

func TestSessionLastPickBeatsTheEnvModel(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("one")}}
	env, _, _ := testEnv(t, h, fp, map[string]string{"GROQ_API_KEY": "g", "HP_MODEL": "xai/grok"})
	if err := saveLastModel(env, "groq/fast"); err != nil {
		t.Fatal(err)
	}
	term := newFakeTerm(t)
	env.Term = term
	code := startHP(t, env)
	term.send(t, "go\r")
	term.await(t, "0 steps", 1)
	quitSession(t, term, code)
	if fp.Requests[0].Model != "fast" {
		t.Fatalf("model %q", fp.Requests[0].Model)
	}
}

func TestRunIgnoresTheLastPick(t *testing.T) {
	h := host.NewMemory()
	fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
	env, _, _ := testEnv(t, h, fp, map[string]string{"GROQ_API_KEY": "g", "HP_MODEL": "xai/grok"})
	if err := saveLastModel(env, "groq/fast"); err != nil {
		t.Fatal(err)
	}
	env.Args = []string{"run", "say hi"}
	if c := Main(context.Background(), env); c != 0 {
		t.Fatalf("code %d", c)
	}
	if fp.Requests[0].Model != "grok" {
		t.Fatalf("model %q", fp.Requests[0].Model)
	}
}

func TestSessionFallsBackToThePickerWhenTheKeyIsGone(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, &fake.Provider{}, map[string]string{"HP_MODEL": "xai/grok"})
	if err := saveLastModel(env, "groq/fast"); err != nil {
		t.Fatal(err)
	}
	term := newFakeTerm(t)
	env.Term = term
	code := startHP(t, env)
	term.await(t, "GROQ_API_KEY is not set", 1)
	term.await(t, "provider: ", 1)
	term.send(t, "\x1b")
	quitSession(t, term, code)
}

func TestWebPickDoesNotChangeTheLastPick(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, &fake.Provider{}, map[string]string{"GROQ_API_KEY": "g"})
	a := &sessionAgent{env: env, ep: Endpoint{Preset: "xai", Model: "grok"}, bySpec: make(map[string]Endpoint)}
	if err := a.useModel("groq/fast"); err != nil {
		t.Fatal(err)
	}
	if got := loadLastModel(env); got != "" {
		t.Fatalf("saved %q", got)
	}
}
