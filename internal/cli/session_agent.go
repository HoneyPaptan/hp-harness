package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/permit"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/skill"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

var modelListTimeout = 5 * time.Second

type sessionAgent struct {
	env        Env
	opts       runOpts
	client     *jev.Client
	dir        string
	skills     []skill.Skill
	tools      *toolset
	ep         Endpoint
	bySpec     map[string]Endpoint
	id         string
	jpath      string
	started    bool
	open       bool
	settling   bool
	clock      *printer
	permMode   permit.Mode
	permRules  []permit.Rule
	permGrants *permit.Grants
}

func newSessionAgent(env Env, o runOpts, ep Endpoint, bySpec map[string]Endpoint, client *jev.Client, dir string) (*sessionAgent, error) {
	skills, err := discoverSkills(env, dir)
	if err != nil {
		return nil, err
	}
	tools := openToolset(env, skills, client)
	a := &sessionAgent{env: env, opts: o, client: client, dir: dir, skills: skills, tools: tools, ep: ep, bySpec: bySpec}
	if err := a.initPerm(false, ""); err != nil {
		tools.close()
		return nil, err
	}
	return a, nil
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
	if err := a.initPerm(false, st.Permission); err != nil {
		return nil, err
	}
	return a, nil
}

func optsFromStart(st journal.RunStart) runOpts {
	return runOpts{
		steps: st.StepLimit, maxTokens: st.MaxTokens, fallbacks: st.Fallbacks,
		switchAfter: st.SwitchAfter, maxWait: st.MaxWait, jev: st.Jev,
	}
}

func (a *sessionAgent) Send(ctx context.Context, prompt string, emit func(tui.Event)) (tui.Outcome, error) {
	if a.ep.Preset == "" {
		return tui.Outcome{}, tui.NoModelError{}
	}
	prompt = expandMentions(a.env.Host, a.dir, prompt)
	deps := a.begin(a.observer(emit))
	return a.finish(a.dispatch(ctx, deps, prompt))
}

func (a *sessionAgent) Resume(ctx context.Context, emit func(tui.Event)) (tui.Outcome, error) {
	deps := a.begin(a.observer(emit))
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
	cfg := runConfig(a.id, a.ep, a.dir, a.jpath, a.opts, prompt, a.skills, a.env, modeString(a.permMode))
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

func (a *sessionAgent) begin(obs core.Observer) core.Deps {
	a.clock = newPrinter(a.env, a.id, a.client)
	rp := runPerm{
		mode:  func() permit.Mode { return a.permMode },
		rules: a.permRules, grants: a.permGrants, dir: a.dir,
	}
	d := runDeps(a.env, a.ep, a.bySpec, a.tools.reg, a.tracked(obs), a.client, rp)
	d.Gate = newGate(a.env.Host, rp.mode, rp.rules, rp.grants, rp.dir, a.env.Getenv("HOME"), nil, newReviewer(a.env, func() Endpoint { return a.ep }))
	d.Providers = providerFactory(a.env, a.bySpec)
	return d
}

func (a *sessionAgent) initPerm(fullAccess bool, journalMode string) error {
	mode, rules, grants, _, err := effectivePermission(a.env, fullAccess, journalMode)
	if err != nil {
		return err
	}
	a.permMode = mode
	a.permRules = rules
	a.permGrants = grants
	return nil
}

func (a *sessionAgent) PermissionMode() string {
	return modeString(a.permMode)
}

func (a *sessionAgent) SetPermissionMode(raw string) error {
	mode, err := permit.ParseMode(raw)
	if err != nil {
		return err
	}
	if mode == a.permMode {
		return nil
	}
	if a.started {
		w, _, err := journal.Open(a.env.Host, a.jpath)
		if err != nil {
			return err
		}
		if err := w.PermissionSwitch(modeString(mode)); err != nil {
			return err
		}
	}
	a.permMode = mode
	return nil
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
			emit(tui.Event{Kind: tui.EventStep})
		},
		OnEvent: func(ev provider.Event) { emit(streamEvent(ev)) },
		OnToolStart: func(c provider.ToolCall) {
			emit(tui.Event{Kind: tui.EventTool, Text: strings.TrimSpace(c.Name + " " + toolMainArg(c.Args))})
		},
		OnToolResult: func(r provider.ToolResult) { a.toolResult(r, emit) },
		OnWait: func(spec string, d time.Duration, until time.Time) {
			note(waitLine(spec, d, until))
		},
		OnSwitch: func(from, to string) { note(switchLine(from, to)) },
		OnRetry: func(d time.Duration, attempt, max int) {
			note(retryLine(d, attempt, max))
		},
	}
}

func streamEvent(ev provider.Event) tui.Event {
	if ev.Kind == provider.EventReasoning {
		return tui.Event{Kind: tui.EventThinking}
	}
	return tui.Event{Kind: tui.EventText, Text: ev.Text}
}

func (a *sessionAgent) tracked(o core.Observer) core.Observer {
	turnStart, event, toolStart, onSwitch := o.OnTurnStart, o.OnEvent, o.OnToolStart, o.OnSwitch
	o.OnTurnStart = func() {
		a.clock.onTurnStart()
		if turnStart != nil {
			turnStart()
		}
	}
	o.OnEvent = func(ev provider.Event) {
		a.clock.markTTFT()
		event(ev)
	}
	o.OnToolStart = func(c provider.ToolCall) {
		a.clock.markTTFT()
		toolStart(c)
	}
	o.OnSwitch = func(from, to string) {
		a.ep = a.bySpec[to]
		onSwitch(from, to)
	}
	return o
}

func (a *sessionAgent) toolResult(r provider.ToolResult, emit func(tui.Event)) {
	if !r.IsError || a.settling {
		return
	}
	emit(tui.Event{Kind: tui.EventToolError, Text: cutRunes(firstLine(r.Text), 120)})
}

func (a *sessionAgent) Preconnect(ctx context.Context) {
	if a.ep.Preset == "" {
		return
	}
	p := a.env.NewProvider(a.ep)
	go provider.Preconnect(ctx, p)
}

func (a *sessionAgent) Model() string {
	if a.ep.Preset == "" {
		return ""
	}
	return endpointSpec(a.ep)
}

func (a *sessionAgent) Models(ctx context.Context, preset string) ([]string, error) {
	ep, err := presetEndpoint(a.ep, preset, a.env.Getenv)
	if err != nil {
		return nil, err
	}
	p := a.env.NewProvider(ep)
	listCtx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	return provider.ListModels(listCtx, p)
}

func (a *sessionAgent) Providers() []string {
	return currentFirst(configuredPresets(a.env.Getenv), a.ep.Preset)
}

func (a *sessionAgent) Jev() bool {
	return a.client != nil
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

func (a *sessionAgent) SetJev(on bool) error {
	if on == a.Jev() {
		return nil
	}
	if on {
		a.env = reloadKeyFile(a.env)
	}
	client, err := openJev(a.env, on)
	if err != nil {
		if errors.Is(err, jev.ErrNoKey) {
			return tui.KeyMissingError{Hint: jevKeyHint}
		}
		return err
	}
	if err := a.journalJev(on); err != nil {
		return err
	}
	a.applyJev(client, on)
	return nil
}

func (a *sessionAgent) SaveKey(name, value string) error {
	if err := checkKeyName(name); err != nil {
		return err
	}
	if err := checkKeyValue(value); err != nil {
		return err
	}
	file, err := keyFilePath(rawGetenv(a.env))
	if err != nil {
		return err
	}
	if err := keySet(file, name, value); err != nil {
		return err
	}
	a.env = reloadKeyFile(a.env)
	return nil
}

func (a *sessionAgent) journalJev(on bool) error {
	if !a.started {
		return nil
	}
	w, _, err := journal.Open(a.env.Host, a.jpath)
	if err != nil {
		return err
	}
	return w.JevSwitch(on)
}

func (a *sessionAgent) applyJev(client *jev.Client, on bool) {
	a.client = client
	a.tools.setJev(a.skills, client)
	a.opts.jev = on
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

func (a *sessionAgent) Files() ([]string, error) {
	return listFiles(a.env.Host, a.dir)
}

func (a *sessionAgent) release() {
	a.tools.close()
	a.releaseRun()
}

func (a *sessionAgent) releaseRun() {
	if a.id != "" {
		removeRunPid(a.env, a.id)
	}
}
