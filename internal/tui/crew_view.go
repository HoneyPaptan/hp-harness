package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

const (
	crewViewHint   = "↑ ↓ select · Enter follow"
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
	ask  *viewAsk
	diff *diffView
}

type viewAsk struct {
	text string
	yes  func()
}

var viewKeyVerbs = map[rune]string{'s': "stop", 'r': "retry", 'd': "diff", 'a': "land", 'x': "discard"}

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
	if s.view.diff != nil {
		s.diffKey(k)
		return
	}
	if s.view.ask != nil {
		s.askKey(k)
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
	case KeyRune:
		s.viewRune(k.Rune)
	case KeyEsc, KeyCtrlC:
		s.closeCrewView()
	}
}

func (s *Session) askKey(k Key) {
	switch {
	case k.Type == KeyRune && (k.Rune == 'y' || k.Rune == 'Y'):
		yes := s.view.ask.yes
		s.view.ask = nil
		yes()
	case k.Type == KeyRune && (k.Rune == 'n' || k.Rune == 'N'), k.Type == KeyEsc, k.Type == KeyCtrlC:
		s.view.ask = nil
	default:
		return
	}
	s.dirty = true
}

func (s *Session) selectedRow() (crewview.Row, bool) {
	rows := s.view.snap.Rows
	if len(rows) == 0 {
		return crewview.Row{}, false
	}
	return rows[min(s.view.menu.selected, len(rows)-1)], true
}

func (s *Session) viewRune(r rune) {
	agent := s.cfg.Agent.(CrewAgent)
	id := s.view.snap.ID
	if r == 'p' {
		if s.view.snap.Paused {
			s.crewAction(func() error { return agent.CrewUnpause(id) })
			return
		}
		s.crewAction(func() error { return agent.CrewPause(id) })
		return
	}
	verb, known := viewKeyVerbs[r]
	row, ok := s.selectedRow()
	if !known || !ok {
		return
	}
	if !strings.ContainsRune(row.Keys, r) {
		s.clearDrawn()
		s.note("nothing to " + verb + ": " + row.Title + " is " + row.Status)
		s.dirty = true
		return
	}
	switch r {
	case 's':
		s.view.ask = &viewAsk{text: "stop " + row.Title + "? y/n", yes: func() {
			s.crewAction(func() error { return agent.CrewStop(id, row.Slug) })
		}}
	case 'r':
		s.crewAction(func() error { return agent.CrewRetry(id, row.Slug) })
	case 'd':
		s.showDiff(agent, row)
	case 'a':
		s.askLand(agent, row)
	case 'x':
		s.view.ask = &viewAsk{text: "discard " + row.Title + " and delete its worktree and branch? y/n", yes: func() {
			s.crewAction(func() error { return agent.CrewDiscard(id, row.Slug) })
		}}
	}
	s.dirty = true
}

func (s *Session) showDiff(agent CrewAgent, row crewview.Row) {
	lines, err := agent.CrewDiff(s.view.snap.ID, row.Slug)
	if err != nil {
		s.clearDrawn()
		s.fail(err)
		return
	}
	s.openDiff(row.Title, lines)
}

func (s *Session) askLand(agent CrewAgent, row crewview.Row) {
	id := s.view.snap.ID
	question, err := agent.CrewLandCheck(id, row.Slug)
	if err != nil {
		s.clearDrawn()
		s.fail(err)
		return
	}
	s.view.ask = &viewAsk{text: question + " y/n", yes: func() {
		msg, err := agent.CrewLand(id, row.Slug)
		s.crewAction(func() error { return err })
		if err == nil {
			s.clearDrawn()
			s.note(msg)
		}
	}}
}

func (s *Session) crewAction(do func() error) {
	if err := do(); err != nil {
		s.clearDrawn()
		s.fail(err)
	}
	s.refreshTasks()
	s.armViewTick()
	s.dirty = true
}

func (s *Session) viewHint() string {
	parts := []string{crewViewHint}
	if row, ok := s.selectedRow(); ok {
		for _, r := range row.Keys {
			parts = append(parts, string(r)+" "+viewKeyVerbs[r])
		}
	}
	pause := "p pause"
	if s.view.snap.Paused {
		pause = "p resume"
	}
	return strings.Join(append(parts, pause, "Esc closes"), " · ")
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
	if strings.Join(rows, "\n") != strings.Join(s.view.menu.items, "\n") || snap.Paused != s.view.snap.Paused || rowKeys(snap) != rowKeys(s.view.snap) {
		s.dirty = true
	}
	s.view.snap = snap
	s.view.menu.items = rows
	s.view.menu.selected = min(s.view.menu.selected, max(len(rows)-1, 0))
	s.view.menu.keepVisible()
}

func rowKeys(snap crewview.Snapshot) string {
	keys := make([]string, len(snap.Rows))
	for i, r := range snap.Rows {
		keys[i] = r.Keys
	}
	return strings.Join(keys, ",")
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
	if s.view.diff != nil {
		s.drawDiff()
		return
	}
	if s.view.live != nil {
		prompt := "following " + s.view.snap.Rows[s.view.live.index].Title + " · "
		hint := "Esc goes back"
		_ = s.renderer.Draw(prompt+hint, nil, 0, s.footer())
		return
	}
	rows, sel := s.view.menu.window()
	more := s.viewHint()
	if s.view.ask != nil {
		more = s.view.ask.text
	}
	if n := s.view.menu.moreHidden(); n > 0 {
		more = "+" + strconv.Itoa(n) + " more · " + more
	}
	prompt := fmt.Sprintf("crew %s · %s", s.view.snap.ID, s.view.snap.Label)
	if s.view.snap.Paused {
		prompt += " · paused"
	}
	_ = s.renderer.DrawMenu(prompt, nil, 0, rows, sel, "", more, s.footer())
}
