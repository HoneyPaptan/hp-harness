package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

const (
	crewUsage         = "usage: /crew [ID | plan.md | goal | list | status ID | report ID]"
	crewNoCrews       = "no crews yet, /crew plan.md or /crew GOAL starts one"
	crewUnavailable   = "/crew is not available here"
	crewBusyNote      = "finish or stop the turn before /crew"
	crewDraftNote     = "drafting a plan with one model request, Esc cancels"
	crewAskNote       = "Enter starts the crew, e edits the plan, Esc cancels"
	crewBadPlanNote   = "the plan has no tasks yet, e edits it, Esc cancels"
	crewCancelNote    = "crew not started"
	crewCountHint     = "enter a number from 1 to %d"
	crewDefaultAt     = 2
	crewPollDefault   = 2 * time.Second
	crewFollowDefault = 300 * time.Millisecond
)

type CrewPlan struct {
	Path   string
	Text   string
	Goal   string
	Titles []string
	Dirty  int
}

type CrewAgent interface {
	CrewPlan(arg string) (CrewPlan, bool, error)
	CrewDraft(ctx context.Context, goal string) (CrewPlan, error)
	CrewEdit(plan CrewPlan) (CrewPlan, error)
	CrewStart(plan CrewPlan, n int) (string, error)
	CrewPreflight() (int, error)
	CrewView(id string) (crewview.Snapshot, bool, error)
	CrewFollow(id string, index, offset int) (crewview.Chunk, error)
	CrewStop(id, task string) error
	CrewRetry(id, task string) error
	CrewPause(id string) error
	CrewUnpause(id string) error
	CrewList() ([]string, error)
	CrewStatus(id string) ([]string, error)
	CrewReport(id string) ([]string, error)
	CrewBrief(id string) (string, bool, error)
}

type crewStage int

const (
	crewDrafting crewStage = iota
	crewCount
)

type crewFlow struct {
	stage  crewStage
	plan   CrewPlan
	dirty  int
	cancel context.CancelFunc
}

type draftResult struct {
	flow *crewFlow
	plan CrewPlan
	err  error
}

func (s *Session) runCrew(ctx context.Context, arg string) {
	agent, ok := s.cfg.Agent.(CrewAgent)
	if !ok {
		s.note(crewUnavailable)
		return
	}
	if s.busy() {
		s.note(crewBusyNote)
		return
	}
	fields := strings.Fields(arg)
	switch {
	case len(fields) == 0:
		s.crewOpen(agent, "")
	case fields[0] == "list" && len(fields) == 1:
		s.showCrewLines(agent.CrewList())
	case fields[0] == "status" && len(fields) <= 2:
		s.crewDetail(fields, agent.CrewStatus)
	case fields[0] == "report" && len(fields) <= 2:
		s.crewDetail(fields, agent.CrewReport)
	default:
		s.crewPlanOrGoal(ctx, agent, arg)
	}
}

func (s *Session) crewDetail(fields []string, show func(string) ([]string, error)) {
	if len(fields) != 2 {
		s.note(crewUsage)
		return
	}
	s.showCrewLines(show(fields[1]))
}

func (s *Session) showCrewLines(lines []string, err error) {
	if err != nil {
		s.fail(err)
		return
	}
	for _, line := range lines {
		s.note(line)
	}
}

func (s *Session) crewPlanOrGoal(ctx context.Context, agent CrewAgent, arg string) {
	if !strings.ContainsAny(arg, " \t") {
		snap, found, err := agent.CrewView(arg)
		if err != nil {
			s.fail(err)
			return
		}
		if found {
			s.openCrewView(snap)
			return
		}
	}
	dirty, err := agent.CrewPreflight()
	if err != nil {
		s.fail(err)
		return
	}
	plan, found, err := agent.CrewPlan(arg)
	if err != nil {
		s.fail(err)
		return
	}
	plan.Dirty = dirty
	if s.cfg.Agent.Model() == "" {
		s.pickModelFirst("/crew " + arg)
		return
	}
	if found {
		s.askCrewCount(plan)
		return
	}
	s.startCrewDraft(ctx, agent, arg, dirty)
}

func (s *Session) startCrewDraft(ctx context.Context, agent CrewAgent, goal string, dirty int) {
	draftCtx, cancel := context.WithCancel(ctx)
	flow := &crewFlow{stage: crewDrafting, dirty: dirty, cancel: cancel}
	s.crew = flow
	s.note(crewDraftNote)
	go func() {
		plan, err := agent.CrewDraft(draftCtx, goal)
		select {
		case s.drafts <- draftResult{flow: flow, plan: plan, err: err}:
		case <-s.stopped:
		}
	}()
}

func (s *Session) handleDraft(res draftResult) {
	if res.flow != s.crew {
		return
	}
	s.crew.cancel()
	s.crew = nil
	s.clearDrawn()
	s.dirty = true
	if res.err != nil {
		s.fail(res.err)
		return
	}
	res.plan.Dirty = res.flow.dirty
	for _, line := range strings.Split(res.plan.Text, "\n") {
		s.note(line)
	}
	s.askCrewCount(res.plan)
}

func (s *Session) askCrewCount(plan CrewPlan) {
	if plan.Text == "" {
		s.noteTitles(plan.Titles)
	}
	if plan.Dirty > 0 {
		s.note(dirtyNote(plan.Dirty))
	}
	s.note(crewAskNote)
	s.crew = &crewFlow{stage: crewCount, plan: plan}
	s.editor.SetText(strconv.Itoa(min(len(plan.Titles), crewDefaultAt)))
	s.dirty = true
}

func (s *Session) noteTitles(titles []string) {
	for i, title := range titles {
		s.note(fmt.Sprintf("%d. %s", i+1, title))
	}
}

func (s *Session) editCrewPlan() {
	typed := strings.TrimSpace(s.editor.Text())
	s.clearDrawn()
	edited, err := s.cfg.Agent.(CrewAgent).CrewEdit(s.crew.plan)
	s.flush = nil
	s.dirty = true
	edited.Dirty = s.crew.plan.Dirty
	edited.Text = ""
	s.crew.plan = edited
	if err != nil {
		s.fail(err)
	}
	s.noteTitles(edited.Titles)
	if len(edited.Titles) == 0 {
		s.note(crewBadPlanNote)
		s.editor.SetText("")
		return
	}
	s.note(crewAskNote)
	s.editor.SetText(strconv.Itoa(min(len(edited.Titles), crewDefaultAt)))
	if n, err := strconv.Atoi(typed); err == nil && n >= 1 && n <= len(edited.Titles) {
		s.editor.SetText(typed)
	}
}

func dirtyNote(n int) string {
	if n == 1 {
		return "note: 1 uncommitted change is not in the crew's worktrees"
	}
	return fmt.Sprintf("note: %d uncommitted changes are not in the crew's worktrees", n)
}

func (s *Session) crewKey(k Key) {
	switch k.Type {
	case KeyEsc, KeyCtrlC:
		s.cancelCrew()
	case KeyUp, KeyDown, KeyCtrlD:
	case KeyEnter:
		if s.crew.stage == crewCount {
			s.confirmCrew()
		}
	default:
		if s.crew.stage != crewCount {
			return
		}
		if k.Type == KeyRune && k.Rune == 'e' {
			s.editCrewPlan()
			return
		}
		s.editor.Handle(k)
		s.dirty = true
	}
}

func (s *Session) cancelCrew() {
	if s.crew.cancel != nil {
		s.crew.cancel()
	}
	s.crew = nil
	s.editor.Clear()
	s.clearDrawn()
	s.note(crewCancelNote)
	s.dirty = true
}

func (s *Session) confirmCrew() {
	plan := s.crew.plan
	max := len(plan.Titles)
	if max == 0 {
		s.clearDrawn()
		s.note(crewBadPlanNote)
		s.dirty = true
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(s.editor.Text()))
	if err != nil || n < 1 || n > max {
		s.clearDrawn()
		s.note(fmt.Sprintf(crewCountHint, max))
		s.dirty = true
		return
	}
	s.crew = nil
	s.editor.Clear()
	s.clearDrawn()
	s.dirty = true
	id, err := s.cfg.Agent.(CrewAgent).CrewStart(plan, n)
	if err != nil {
		s.fail(err)
		return
	}
	s.crewID, s.crewBrief = id, ""
	s.note(fmt.Sprintf("crew %s started, %d at once, it keeps running if you quit", id, n))
	s.note("/crew status " + id + " · /crew report " + id)
	s.pollCrew()
}

func (s *Session) pollCrew() {
	s.crewTick = nil
	if s.crewID == "" {
		return
	}
	brief, done, err := s.cfg.Agent.(CrewAgent).CrewBrief(s.crewID)
	if err != nil {
		s.endCrewWatch(func() { s.fail(err) })
		return
	}
	if done {
		id := s.crewID
		s.endCrewWatch(func() { s.note("crew " + id + " done: " + brief + " · /crew report " + id) })
		return
	}
	if brief != s.crewBrief {
		s.crewBrief = brief
		s.dirty = true
	}
	s.crewTick = time.After(s.cfg.CrewPoll)
}

func (s *Session) endCrewWatch(show func()) {
	s.clearDrawn()
	show()
	s.crewID, s.crewBrief = "", ""
	s.dirty = true
}

func (s *Session) crewPrompt() string {
	if s.crew.stage == crewDrafting {
		return "drafting plan · Esc cancels "
	}
	n := len(s.crew.plan.Titles)
	if n == 0 {
		return "plan has no tasks · e edits · Esc cancels "
	}
	noun := "tasks"
	if n == 1 {
		noun = "task"
	}
	return fmt.Sprintf("%d %s · agents at once (1-%d): ", n, noun, n)
}

func (s *Session) footer() string {
	line := footerLine(s.cfg.Agent)
	switch {
	case s.crewBrief != "":
		line += " · crew " + s.crewBrief
	case s.view != nil && s.view.snap.Paused:
		line += " · crew paused"
	}
	return line
}
