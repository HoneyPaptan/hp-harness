package cli

import (
	"context"
	"errors"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

const (
	crewDraftTimeout   = 2 * time.Minute
	crewDraftMaxTokens = 4096
)

type CrewService interface {
	PlanTitles(plan string) ([]string, bool, error)
	DraftSystem() string
	SavePlan(reply string) (path, text string, err error)
	EditPlan(plan string) ([]string, error)
	Stop(id, task string) error
	Retry(id, task string) error
	Pause(id string) error
	Unpause(id string) error
	LastVerify() string
	Start(req crewview.Start) (string, error)
	Preflight() (int, error)
	View(id string) (crewview.Snapshot, bool, error)
	Follow(id string, index, offset int) (crewview.Chunk, error)
	List() ([]string, error)
	Status(id string) ([]string, error)
	Report(id string) ([]string, error)
	Brief(id string) (string, bool, error)
}

func (a *sessionAgent) crewService() (CrewService, error) {
	if a.env.Crew != nil {
		return a.env.Crew(a.dir), nil
	}
	if a.env.Browser {
		return nil, errors.New("/crew is not available in the browser")
	}
	return nil, errors.New("/crew is not available here")
}

func (a *sessionAgent) CrewPlan(arg string) (tui.CrewPlan, bool, error) {
	svc, err := a.crewService()
	if err != nil {
		return tui.CrewPlan{}, false, err
	}
	titles, found, err := svc.PlanTitles(arg)
	return tui.CrewPlan{Path: arg, Titles: titles, Verify: svc.LastVerify()}, found, err
}

func (a *sessionAgent) CrewDraft(ctx context.Context, goal string) (tui.CrewPlan, error) {
	svc, err := a.crewService()
	if err != nil {
		return tui.CrewPlan{}, err
	}
	if a.ep.Preset == "" {
		return tui.CrewPlan{}, tui.NoModelError{}
	}
	session, err := newRunID(a.env.Host.Now(), a.env.Rand)
	if err != nil {
		return tui.CrewPlan{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, crewDraftTimeout)
	defer cancel()
	msg, _, err := provider.Collect(a.env.NewProvider(a.ep).Stream(ctx, provider.Request{
		Model: a.ep.Model, System: svc.DraftSystem(), MaxTokens: crewDraftMaxTokens, SessionID: session,
		Messages: []provider.Message{{Role: provider.RoleUser, Text: goal}},
	}))
	if err != nil {
		return tui.CrewPlan{}, err
	}
	path, text, err := svc.SavePlan(msg.Text)
	if err != nil {
		return tui.CrewPlan{}, err
	}
	titles, _, err := svc.PlanTitles(path)
	return tui.CrewPlan{Path: path, Text: text, Titles: titles, Goal: goal, Verify: svc.LastVerify()}, err
}

func (a *sessionAgent) CrewEdit(plan tui.CrewPlan) (tui.CrewPlan, error) {
	svc, err := a.crewService()
	if err != nil {
		return tui.CrewPlan{}, err
	}
	titles, err := svc.EditPlan(plan.Path)
	plan.Titles = titles
	return plan, err
}

func (a *sessionAgent) CrewStart(plan tui.CrewPlan, n int) (string, error) {
	svc, err := a.crewService()
	if err != nil {
		return "", err
	}
	if a.ep.Preset == "" {
		return "", tui.NoModelError{}
	}
	return svc.Start(crewview.Start{
		Plan: plan.Path, Label: plan.Goal, N: n, Verify: plan.Verify,
		Model: endpointSpec(a.ep), BaseURL: a.ep.BaseURL, KeyEnv: a.ep.KeyEnv,
	})
}

func (a *sessionAgent) CrewPreflight() (int, error) {
	svc, err := a.crewService()
	if err != nil {
		return 0, err
	}
	return svc.Preflight()
}

func (a *sessionAgent) CrewView(id string) (crewview.Snapshot, bool, error) {
	svc, err := a.crewService()
	if err != nil {
		return crewview.Snapshot{}, false, err
	}
	return svc.View(id)
}

func (a *sessionAgent) CrewFollow(id string, index, offset int) (crewview.Chunk, error) {
	svc, err := a.crewService()
	if err != nil {
		return crewview.Chunk{}, err
	}
	return svc.Follow(id, index, offset)
}

func (a *sessionAgent) CrewStop(id, task string) error {
	return a.crewDo(func(svc CrewService) error { return svc.Stop(id, task) })
}

func (a *sessionAgent) CrewRetry(id, task string) error {
	return a.crewDo(func(svc CrewService) error { return svc.Retry(id, task) })
}

func (a *sessionAgent) CrewPause(id string) error {
	return a.crewDo(func(svc CrewService) error { return svc.Pause(id) })
}

func (a *sessionAgent) CrewUnpause(id string) error {
	return a.crewDo(func(svc CrewService) error { return svc.Unpause(id) })
}

func (a *sessionAgent) crewDo(do func(CrewService) error) error {
	svc, err := a.crewService()
	if err != nil {
		return err
	}
	return do(svc)
}

func (a *sessionAgent) CrewList() ([]string, error) {
	svc, err := a.crewService()
	if err != nil {
		return nil, err
	}
	return svc.List()
}

func (a *sessionAgent) CrewStatus(id string) ([]string, error) {
	svc, err := a.crewService()
	if err != nil {
		return nil, err
	}
	return svc.Status(id)
}

func (a *sessionAgent) CrewReport(id string) ([]string, error) {
	svc, err := a.crewService()
	if err != nil {
		return nil, err
	}
	return svc.Report(id)
}

func (a *sessionAgent) CrewBrief(id string) (string, bool, error) {
	svc, err := a.crewService()
	if err != nil {
		return "", false, err
	}
	return svc.Brief(id)
}
