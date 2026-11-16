package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

const (
	crewViewHint   = "↑ ↓ select · Enter follows a task · Esc closes"
	crewLiveHint   = "Esc goes back to the tasks"
	crewWaitNote   = "waiting for the task to start"
	crewBackNote   = "Esc goes back to the tasks"
	titleCells     = 24
	statusCells    = 14
	runCells       = 22
	crewHeaderRule = "── "
)

type crewView struct {
	snap crewview.Snapshot
	menu *menu
	live *crewLive
}

type crewLive struct {
	index  int
	offset int
	done   bool
	waited bool
}

func (s *Session) crewOpen(agent CrewAgent, id string) {
	snap, found, err := agent.CrewView(id)
	if err != nil {
		s.fail(err)
		return
	}
	if !found {
		s.note(crewNoCrews)
		return
	}
	s.openCrewView(snap)
}

func (s *Session) openCrewView(snap crewview.Snapshot) {
	s.view = &crewView{snap: snap, menu: newMenu(crewRows(snap))}
	s.armViewTick()
	s.dirty = true
}

func crewRows(snap crewview.Snapshot) []string {
	rows := make([]string, len(snap.Rows))
	for i, r := range snap.Rows {
		rows[i] = padCut(r.Title, titleCells) + " " + padCut(r.Status, statusCells) + " " + padCut(r.RunID, runCells) + " " + r.Changed
	}
	return rows
}

func padCut(text string, cells int) string {
	runes := []rune(text)
	if len(runes) > cells {
		return string(runes[:cells-1]) + "…"
	}
	return text + strings.Repeat(" ", cells-len(runes))
}

func (s *Session) viewKey(k Key) {
	if s.view.live != nil {
		s.liveKey(k)
		return
	}
	switch k.Type {
	case KeyUp:
		s.view.menu.move(-1)
		s.dirty = true
	case KeyDown:
		s.view.menu.move(1)
		s.dirty = true
	case KeyEnter:
		s.followTask()
	case KeyEsc, KeyCtrlC:
		s.closeCrewView()
	}
}

func (s *Session) liveKey(k Key) {
	if k.Type == KeyEsc || k.Type == KeyCtrlC {
		s.backToTasks()
	}
}

func (s *Session) closeCrewView() {
	s.view = nil
	s.viewTick = nil
	s.clearDrawn()
	s.dirty = true
}

func (s *Session) followTask() {
	if len(s.view.snap.Rows) == 0 {
		return
	}
	index := s.view.menu.selected
	row := s.view.snap.Rows[index]
	s.clearDrawn()
	s.view.live = &crewLive{index: index}
	s.note(crewHeaderRule + row.Title + " · " + row.Status + " · run " + row.RunID)
	s.pollLive()
	s.armViewTick()
	s.dirty = true
}

func (s *Session) backToTasks() {
	s.clearDrawn()
	s.out.endLine()
	s.view.live = nil
	s.refreshTasks()
	s.armViewTick()
	s.dirty = true
}

func (s *Session) pollView() {
	s.viewTick = nil
	if s.view == nil {
		return
	}
	if s.view.live != nil {
		s.pollLive()
	} else {
		s.refreshTasks()
	}
	s.armViewTick()
}

func (s *Session) armViewTick() {
	s.viewTick = nil
	switch {
	case s.view == nil:
	case s.view.live != nil && !s.view.live.done:
		s.viewTick = time.After(s.cfg.CrewFollow)
	case s.view.live == nil && !s.view.snap.Done:
		s.viewTick = time.After(s.cfg.CrewPoll)
	}
}

func (s *Session) refreshTasks() {
	snap, found, err := s.cfg.Agent.(CrewAgent).CrewView(s.view.snap.ID)
	if err != nil {
		s.closeCrewView()
		s.fail(err)
		return
	}
	if !found {
		s.closeCrewView()
		return
	}
	rows := crewRows(snap)
	if strings.Join(rows, "\n") != strings.Join(s.view.menu.items, "\n") {
		s.dirty = true
	}
	s.view.snap = snap
	s.view.menu.items = rows
	s.view.menu.selected = min(s.view.menu.selected, max(len(rows)-1, 0))
	s.view.menu.keepVisible()
}

func (s *Session) pollLive() {
	live := s.view.live
	chunk, err := s.cfg.Agent.(CrewAgent).CrewFollow(s.view.snap.ID, live.index, live.offset)
	if err != nil {
		s.backToTasks()
		s.fail(err)
		return
	}
	if chunk.Waiting && !live.waited {
		live.waited = true
		s.clearDrawn()
		s.note(crewWaitNote)
	}
	if chunk.Text != "" {
		s.clearDrawn()
		s.out.text(chunk.Text)
		live.offset = chunk.Offset
	}
	if chunk.Ended {
		s.clearDrawn()
		s.out.endLine()
		for _, line := range chunk.Trailer {
			s.note(line)
		}
		s.note(crewBackNote)
		live.done = true
	}
	s.dirty = true
}

func (s *Session) drawView() {
	if s.view.live != nil {
		prompt := "following " + s.view.snap.Rows[s.view.live.index].Title + " · "
		hint := "Esc goes back"
		_ = s.renderer.Draw(prompt+hint, nil, 0, s.footer())
		return
	}
	rows, sel := s.view.menu.window()
	more := crewViewHint
	if n := s.view.menu.moreHidden(); n > 0 {
		more = "+" + strconv.Itoa(n) + " more · " + more
	}
	prompt := fmt.Sprintf("crew %s · %s", s.view.snap.ID, s.view.snap.Label)
	_ = s.renderer.DrawMenu(prompt, nil, 0, rows, sel, "", more, s.footer())
}
