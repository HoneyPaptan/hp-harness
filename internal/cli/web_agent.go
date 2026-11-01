package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/web"
)

const webRunLimit = 50

type webBackend struct {
	env  Env
	opts runOpts
	dir  string
	pool *toolPool
}

type webAgent struct {
	*sessionAgent
	pool *toolPool
}

func (b *webBackend) State() web.State {
	st := web.State{Presets: configuredPresets(b.env.Getenv), Jev: b.opts.jev, Dir: b.dir}
	if ep, _, err := resolveRunSpecs(b.opts, b.env.Getenv); err == nil {
		st.Model = endpointSpec(ep)
	}
	return st
}

func configuredPresets(getenv func(string) string) []string {
	out := []string{}
	for _, name := range presetNames() {
		if key := presets[name].keyEnv; key == "" || getenv(key) != "" {
			out = append(out, name)
		}
	}
	return out
}

func (b *webBackend) Runs() ([]web.Run, error) {
	dir := runsDir(b.env.Getenv)
	ids, err := sortedRunIDs(b.env, dir)
	if err != nil {
		return nil, err
	}
	out := make([]web.Run, 0, min(len(ids), webRunLimit))
	for _, id := range ids {
		if len(out) == webRunLimit {
			break
		}
		if r, err := b.summary(id); err == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

func (b *webBackend) summary(id string) (web.Run, error) {
	recs, err := b.Records(id)
	if err != nil {
		return web.Run{}, err
	}
	st, err := journal.Replay(recs)
	if err != nil {
		return web.Run{}, err
	}
	return web.Run{
		ID: id, Title: cutRunes(oneLine(firstPrompt(st)), 80), Model: currentSpec(st),
		Status: webStatus(b.env, id, st), Updated: recs[len(recs)-1].TS,
	}, nil
}

func currentSpec(st journal.State) string {
	if st.Current != "" {
		return st.Current
	}
	return st.Start.Preset + "/" + st.Start.Model
}

func webStatus(env Env, id string, st journal.State) string {
	if runAlive(env, id) {
		return web.StatusRunning
	}
	if st.Finished {
		return web.StatusFinished
	}
	return web.StatusInterrupted
}

func (b *webBackend) Records(id string) ([]journal.Record, error) {
	return journal.Read(b.env.Host, b.journalPath(id))
}

func (b *webBackend) journalPath(id string) string {
	return filepath.Join(runsDir(b.env.Getenv), id+".jsonl")
}

func (b *webBackend) Busy(id string) bool {
	return liveElsewhere(b.env, id)
}

func (b *webBackend) Start(model, prompt string) (web.Agent, error) {
	o := b.opts
	if model != "" {
		o.model = model
	}
	ep, bySpec, err := resolveRunSpecs(o, b.env.Getenv)
	if err != nil {
		return nil, invalid(err)
	}
	client, err := openJev(b.env, o.jev)
	if err != nil {
		return nil, invalid(err)
	}
	a, err := newSessionAgent(b.env, o, ep, bySpec, client, b.dir)
	if err != nil {
		return nil, err
	}
	if err := a.seed(prompt); err != nil {
		a.release()
		return nil, err
	}
	return &webAgent{a, b.pool}, nil
}

func (a *sessionAgent) seed(prompt string) error {
	if err := a.register(); err != nil {
		return err
	}
	cfg := runConfig(a.id, a.ep, a.dir, a.jpath, a.opts, prompt, a.skills, a.env)
	if err := core.Seed(core.Deps{Host: a.env.Host}, cfg); err != nil {
		return err
	}
	a.started = true
	return nil
}

func invalid(err error) error {
	return fmt.Errorf("%w: %s", web.ErrInvalid, err)
}

func (b *webBackend) Open(id string) (web.Agent, error) {
	jpath := b.journalPath(id)
	st, err := replayRun(b.env, jpath)
	if err != nil {
		return nil, err
	}
	w, err := resolveStored(b.env, st, jpath)
	if err != nil {
		return nil, invalid(err)
	}
	a, err := restoredAgent(b.env, st, w.bySpec, w.ep, w.client, id, jpath)
	if err != nil {
		return nil, err
	}
	if b.Busy(id) {
		return nil, web.ErrBusy
	}
	if err := writeRunPid(b.env, id); err != nil {
		return nil, err
	}
	a.open = !st.Finished
	if parked := b.pool.take(id); parked != nil {
		a.tools = parked
	}
	return &webAgent{a, b.pool}, nil
}

func (a *webAgent) ID() string { return a.id }

func (a *webAgent) Release() {
	a.releaseRun()
	a.pool.park(a.id, a.tools)
}

func (a *webAgent) SetModel(spec string) error {
	err := a.sessionAgent.SetModel(spec)
	var usage usageError
	if errors.As(err, &usage) {
		return invalid(err)
	}
	return err
}

func (a *webAgent) Send(ctx context.Context, prompt string, obs core.Observer) (web.Stats, error) {
	deps := a.begin(obs)
	return a.stats(a.dispatch(ctx, deps, prompt))
}

func (a *webAgent) Resume(ctx context.Context, obs core.Observer) (web.Stats, error) {
	deps := a.begin(obs)
	return a.stats(core.Resume(ctx, deps, a.jpath))
}

func (a *webAgent) stats(out core.Outcome, err error) (web.Stats, error) {
	if err != nil {
		return web.Stats{}, err
	}
	return web.Stats{
		Reason: out.Reason, Steps: out.Steps,
		InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens,
		TTFT: a.clock.ttft, Jev: a.jevSpend(),
	}, nil
}

func (a *webAgent) jevSpend() *web.JevSpend {
	if a.client == nil {
		return nil
	}
	s := a.client.Spent()
	return &web.JevSpend{Requests: s.Requests, InputTokens: s.InputTokens, OutputTokens: s.OutputTokens}
}
