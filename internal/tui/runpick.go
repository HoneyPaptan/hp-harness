package tui

import (
	"context"
	"strconv"
)

const (
	runPrompt = "run: "
	runFooter = "hp -r · runs in this folder, newest first"
	runHint   = "↑ ↓ Enter resumes, type to filter, Esc cancels"
)

type RunChoice struct {
	ID    string
	Label string
}

func (s *Session) startRunPick() {
	labels := make([]string, len(s.cfg.RunChoices))
	s.runIDs = make(map[string]string, len(labels))
	for i, c := range s.cfg.RunChoices {
		labels[i] = c.Label
		s.runIDs[c.Label] = c.ID
	}
	s.runPick = newMenu(labels)
}

func runPickMore(m *menu) string {
	if n := m.moreHidden(); n > 0 {
		return "+" + strconv.Itoa(n) + " more · " + runHint
	}
	return runHint
}

func (s *Session) runPickKey(ctx context.Context, k Key) {
	switch k.Type {
	case KeyUp:
		s.runPick.move(-1)
	case KeyDown:
		s.runPick.move(1)
	case KeyEnter:
		s.chooseRun(ctx)
	case KeyEsc, KeyCtrlC, KeyCtrlD:
		s.quit = true
	default:
		s.editor.Handle(k)
		s.runPick.setFilter(s.editor.Text())
	}
	s.dirty = true
}

func (s *Session) chooseRun(ctx context.Context) {
	label, ok := s.runPick.chosen()
	if !ok {
		return
	}
	agent, resume, intro, err := s.cfg.OpenRun(s.runIDs[label])
	s.clearDrawn()
	s.runPick = nil
	s.editor.Clear()
	if err != nil {
		s.fail(err)
		s.quit = true
		return
	}
	s.cfg.Agent = agent
	s.preconnect(ctx)
	if intro != "" {
		s.note(intro)
	}
	s.out.gap()
	if resume {
		s.startTurn(ctx, s.cfg.Agent.Resume)
	}
}
