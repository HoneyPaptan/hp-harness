package cli

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/skill"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

type sessionAgent struct {
	env      Env
	opts     runOpts
	client   *jev.Client
	dir      string
	skills   []skill.Skill
	ep       Endpoint
	bySpec   map[string]Endpoint
	id       string
	jpath    string
	started  bool
	open     bool
	settling bool
	clock    *printer
}

func newSessionAgent(env Env, o runOpts, ep Endpoint, bySpec map[string]Endpoint, client *jev.Client, dir string) (*sessionAgent, error) {
	skills, err := discoverSkills(env, dir)
	if err != nil {
		return nil, err
	}
	return &sessionAgent{env: env, opts: o, client: client, dir: dir, skills: skills, ep: ep, bySpec: bySpec}, nil
}

func restoredAgent(env Env, st journal.State, bySpec map[string]Endpoint, ep Endpoint, client *jev.Client, id, jpath string) (*sessionAgent, error) {
	a, err := newSessionAgent(env, optsFromStart(st.Start), ep, bySpec, client, st.Start.Dir)
	if err != nil {
		return nil, err
	}
	if cur, ok := bySpec[st.Current]; ok {
		a.ep = cur
	}
	a.id, a.jpath, a.started = id, jpath, true
	return a, nil
}

func optsFromStart(st journal.RunStart) runOpts {
	return runOpts{
		steps: st.StepLimit, maxTokens: st.MaxTokens, fallbacks: st.Fallbacks,
		switchAfter: st.SwitchAfter, maxWait: st.MaxWait, jev: st.Jev,
	}
}

func (a *sessionAgent) Send(ctx context.Context, prompt string, emit func(tui.Event)) (tui.Outcome, error) {
	deps := a.begin(emit)
	return a.finish(a.dispatch(ctx, deps, prompt))
}

func (a *sessionAgent) Resume(ctx context.Context, emit func(tui.Event)) (tui.Outcome, error) {
	deps := a.begin(emit)
	return a.finish(core.Resume(ctx, deps, a.jpath))
}

func (a *sessionAgent) dispatch(ctx context.Context, deps core.Deps, prompt string) (core.Outcome, error) {
	if !a.started {
		return a.first(ctx, deps, prompt)
	}
	if a.open {
		a.settling = true
		return core.ContinueInterrupted(ctx, deps, a.jpath, prompt)
	}
	return core.Continue(ctx, deps, a.jpath, prompt)
}

func (a *sessionAgent) first(ctx context.Context, deps core.Deps, prompt string) (core.Outcome, error) {
	if err := a.register(); err != nil {
		return core.Outcome{}, err
	}
	cfg := runConfig(a.id, a.ep, a.dir, a.jpath, a.opts, prompt, a.skills, a.env)
	out, err := core.Run(ctx, deps, cfg)
	a.started = journalExists(a.env, a.jpath)
	return out, err
}

func (a *sessionAgent) register() error {
	if a.id != "" {
		return nil
	}
	id, err := newRunID(a.env.Host.Now(), a.env.Rand)
	if err != nil {
		return err
	}
	a.id = id
	a.jpath = filepath.Join(runsDir(a.env.Getenv), id+".jsonl")
	return writeRunPid(a.env, id)
}

func (a *sessionAgent) begin(emit func(tui.Event)) core.Deps {
	a.clock = newPrinter(a.env, a.id, a.client)
	d := runDeps(a.env, a.ep, a.bySpec, a.skills, a.observer(emit), a.client)
	d.Providers = providerFactory(a.env, a.bySpec)
	return d
}

func (a *sessionAgent) finish(out core.Outcome, err error) (tui.Outcome, error) {
	a.open = err != nil
	if err != nil {
		return tui.Outcome{}, err
	}
	return tui.Outcome{
		Model: a.Model(), Steps: out.Steps,
		InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens,
		TTFT: a.clock.ttft, Jev: strings.TrimSpace(a.clock.jevSummary()), Note: reasonNote(out.Reason),
	}, nil
}

func reasonNote(reason string) string {
	if reason == core.ReasonStepLimit {
		return "step limit"
	}
	return ""
}

func (a *sessionAgent) observer(emit func(tui.Event)) core.Observer {
	note := func(text string) { emit(tui.Event{Kind: tui.EventNote, Text: text}) }
	return core.Observer{
		OnTurnStart: func() {
			a.settling = false
			a.clock.onTurnStart()
			emit(tui.Event{Kind: tui.EventStep})
		},
		OnEvent: func(ev provider.Event) {
			a.clock.markTTFT()
			emit(tui.Event{Kind: tui.EventText, Text: ev.Text})
		},
		OnToolStart: func(c provider.ToolCall) {
			a.clock.markTTFT()
			emit(tui.Event{Kind: tui.EventTool, Text: strings.TrimSpace(c.Name + " " + toolMainArg(c.Args))})
		},
		OnToolResult: func(r provider.ToolResult) { a.toolResult(r, emit) },
		OnWait: func(spec string, d time.Duration, until time.Time) {
			note(waitLine(spec, d, until))
		},
		OnSwitch: func(from, to string) {
			a.ep = a.bySpec[to]
			note(switchLine(from, to))
		},
		OnRetry: func(d time.Duration, attempt, max int) {
			note(retryLine(d, attempt, max))
		},
	}
}

func (a *sessionAgent) toolResult(r provider.ToolResult, emit func(tui.Event)) {
	if !r.IsError || a.settling {
		return
	}
	emit(tui.Event{Kind: tui.EventToolError, Text: cutRunes(firstLine(r.Text), 120)})
}

func (a *sessionAgent) Model() string {
	return endpointSpec(a.ep)
}

func (a *sessionAgent) SetModel(spec string) error {
	ep, ok := a.bySpec[spec]
	if !ok {
		found, err := endpointFor(a.ep, spec, a.env.Getenv)
		if err != nil {
			return err
		}
		ep = found
	}
	if err := a.journalSwitch(endpointSpec(ep)); err != nil {
		return err
	}
	a.bySpec[endpointSpec(ep)] = ep
	a.ep = ep
	return nil
}

func (a *sessionAgent) journalSwitch(to string) error {
	if !a.started || to == a.Model() {
		return nil
	}
	w, _, err := journal.Open(a.env.Host, a.jpath)
	if err != nil {
		return err
	}
	return w.ModelSwitch(a.Model(), to)
}

func (a *sessionAgent) Reset() error {
	a.release()
	a.id, a.jpath = "", ""
	a.started, a.open = false, false
	return nil
}

func (a *sessionAgent) Runs() ([]string, error) {
	return runLines(a.env)
}

func (a *sessionAgent) release() {
	if a.id != "" {
		removeRunPid(a.env, a.id)
	}
}
