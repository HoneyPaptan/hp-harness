package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

type startCall struct {
	plan                   string
	n                      int
	model, baseURL, keyEnv string
}

type fakeCrew struct {
	titles   map[string][]string
	saveErr  error
	started  []startCall
	startErr error
	briefs   []string
	done     bool
}

func (f *fakeCrew) PlanTitles(plan string) ([]string, bool, error) {
	titles, ok := f.titles[plan]
	return titles, ok, nil
}

func (f *fakeCrew) DraftSystem() string { return "draft system" }

func (f *fakeCrew) SavePlan(reply string) (string, string, error) {
	if f.saveErr != nil {
		return "", "", f.saveErr
	}
	f.titles["/plans/p.md"] = []string{"a", "b"}
	return "/plans/p.md", strings.TrimSpace(reply), nil
}

func (f *fakeCrew) Start(plan string, n int, model, baseURL, keyEnv string) (string, error) {
	if f.startErr != nil {
		return "", f.startErr
	}
	f.started = append(f.started, startCall{plan, n, model, baseURL, keyEnv})
	return "c1", nil
}

func (f *fakeCrew) List() ([]string, error)            { return []string{"listed"}, nil }
func (f *fakeCrew) Status(id string) ([]string, error) { return []string{"status " + id}, nil }
func (f *fakeCrew) Report(id string) ([]string, error) { return []string{"report " + id}, nil }

func (f *fakeCrew) Brief(string) (string, bool, error) { return "1 running", f.done, nil }

func crewAgent(t *testing.T, ep Endpoint, prov provider.Provider, svc *fakeCrew, dirs *[]string) *sessionAgent {
	t.Helper()
	env, _, _ := testEnv(t, host.NewMemory(), &fake.Provider{}, nil)
	env.NewProvider = func(Endpoint) provider.Provider { return prov }
	if svc != nil {
		env.Crew = func(dir string) CrewService {
			if dirs != nil {
				*dirs = append(*dirs, dir)
			}
			return svc
		}
	}
	return &sessionAgent{env: env, ep: ep, dir: "/work", bySpec: make(map[string]Endpoint)}
}

var customEP = Endpoint{Preset: "custom", Model: "m", BaseURL: "http://x/v1", KeyEnv: "K"}

func TestCrewUnavailable(t *testing.T) {
	cases := []struct {
		name    string
		browser bool
		want    string
	}{
		{"terminal without crew", false, "/crew is not available here"},
		{"browser", true, "/crew is not available in the browser"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := crewAgent(t, customEP, &fake.Provider{}, nil, nil)
			a.env.Browser = tc.browser
			calls := map[string]error{}
			_, _, calls["plan"] = a.CrewPlan("x")
			_, calls["draft"] = a.CrewDraft(context.Background(), "x")
			_, calls["start"] = a.CrewStart(tui.CrewPlan{}, 1)
			_, calls["list"] = a.CrewList()
			_, calls["status"] = a.CrewStatus("c")
			_, calls["report"] = a.CrewReport("c")
			_, _, calls["brief"] = a.CrewBrief("c")
			for name, err := range calls {
				if err == nil || err.Error() != tc.want {
					t.Fatalf("%s: err %v", name, err)
				}
			}
		})
	}
}

func TestCrewDraftSendsOneRequest(t *testing.T) {
	prov := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("## a\nx\n## b\ny\n")}}
	svc := &fakeCrew{titles: map[string][]string{}}
	var dirs []string
	a := crewAgent(t, customEP, prov, svc, &dirs)
	plan, err := a.CrewDraft(context.Background(), "build a thing")
	if err != nil {
		t.Fatal(err)
	}
	if len(prov.Requests) != 1 {
		t.Fatalf("requests %d", len(prov.Requests))
	}
	req := prov.Requests[0]
	if req.Model != "m" || req.System != "draft system" || len(req.Messages) != 1 || req.Messages[0].Text != "build a thing" || len(req.Tools) != 0 || req.SessionID == "" {
		t.Fatalf("request %+v", req)
	}
	if plan.Path != "/plans/p.md" || plan.Text != "## a\nx\n## b\ny" || len(plan.Titles) != 2 {
		t.Fatalf("plan %+v", plan)
	}
	if len(dirs) == 0 || dirs[0] != "/work" {
		t.Fatalf("dirs %v", dirs)
	}
}

func TestCrewDraftFailures(t *testing.T) {
	cases := []struct {
		name   string
		ep     Endpoint
		prov   *fake.Provider
		save   error
		want   string
		wantRq int
	}{
		{"no model", Endpoint{}, &fake.Provider{}, nil, "pick a model first", 0},
		{"provider error", customEP, &fake.Provider{Turns: []fake.Turn{{Err: errors.New("rate limited")}}}, nil, "rate limited", 1},
		{"not a plan", customEP, &fake.Provider{Turns: []fake.Turn{fake.TextTurn("sorry")}}, errors.New("not a plan"), "not a plan", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeCrew{titles: map[string][]string{}, saveErr: tc.save}
			a := crewAgent(t, tc.ep, tc.prov, svc, nil)
			_, err := a.CrewDraft(context.Background(), "goal")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v", err)
			}
			if len(tc.prov.Requests) != tc.wantRq {
				t.Fatalf("requests %d want %d", len(tc.prov.Requests), tc.wantRq)
			}
		})
	}
}

func TestCrewStartUsesSessionModel(t *testing.T) {
	svc := &fakeCrew{titles: map[string][]string{}}
	a := crewAgent(t, customEP, &fake.Provider{}, svc, nil)
	id, err := a.CrewStart(tui.CrewPlan{Path: "plan.md"}, 3)
	if err != nil || id != "c1" {
		t.Fatalf("%q %v", id, err)
	}
	want := startCall{"plan.md", 3, "custom/m", "http://x/v1", "K"}
	if len(svc.started) != 1 || svc.started[0] != want {
		t.Fatalf("started %+v", svc.started)
	}
	empty := crewAgent(t, Endpoint{}, &fake.Provider{}, svc, nil)
	if _, err := empty.CrewStart(tui.CrewPlan{Path: "plan.md"}, 1); !tui.IsNoModel(err) {
		t.Fatalf("err %v", err)
	}
	if len(svc.started) != 1 {
		t.Fatalf("started without a model: %+v", svc.started)
	}
}

func TestCrewPlanAndShowPassThrough(t *testing.T) {
	svc := &fakeCrew{titles: map[string][]string{"plan.md": {"one", "two"}}}
	a := crewAgent(t, customEP, &fake.Provider{}, svc, nil)
	plan, found, err := a.CrewPlan("plan.md")
	if err != nil || !found || plan.Path != "plan.md" || len(plan.Titles) != 2 {
		t.Fatalf("%+v %v %v", plan, found, err)
	}
	if _, found, _ := a.CrewPlan("a goal"); found {
		t.Fatal("goal reported as a file")
	}
	if lines, _ := a.CrewList(); lines[0] != "listed" {
		t.Fatalf("list %v", lines)
	}
	if lines, _ := a.CrewStatus("c9"); lines[0] != "status c9" {
		t.Fatalf("status %v", lines)
	}
	if lines, _ := a.CrewReport("c9"); lines[0] != "report c9" {
		t.Fatalf("report %v", lines)
	}
	if text, done, _ := a.CrewBrief("c9"); text != "1 running" || done {
		t.Fatalf("brief %q %v", text, done)
	}
}
