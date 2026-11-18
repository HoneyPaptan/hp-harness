package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

const (
	crewViewHint   = "↑ ↓ or 1-9 select · Enter follow"
	crewBackHint   = "Esc goes back"
	crewWaitNote   = "waiting for the task to start"
	crewHeaderRule = "── "
	rowMarkCells   = 2
	rowNumberCells = 3
	statusCells    = 13
	changedCells   = 8
	titleMinCells  = 16
	noRunID        = "-"
	hintJoin       = " · "
)

type crewView struct {
	snap crewview.Snapshot
	menu *menu
	live *crewLive
	ask  *viewAsk
	diff *pager
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
	s.view = &crewView{snap: snap, menu: newMenu(crewRows(snap, s.cfg.Width()))}
	s.armViewTick()
	s.dirty = true
}

func crewRows(snap crewview.Snapshot, width int) []string {
	title := titleCells(snap, width)
	rows := make([]string, len(snap.Rows))
	for i, r := range snap.Rows {
		rows[i] = fmt.Sprintf("%2d ", i+1) + padCut(r.Title, title) + " " + padCut(r.Status, statusCells) + " " + r.Changed
	}
	return rows
}

func titleCells(snap crewview.Snapshot, width int) int {
	longest := 0
	for _, r := range snap.Rows {
		longest = max(longest, rowWidth(r.Title))
	}
	free := width - rowMarkCells - rowNumberCells - statusCells - changedCells - 2
	return min(longest, max(free, titleMinCells))
}

func padCut(text string, cells int) string {
	if rowWidth(text) > cells {
		text = fitRow(text, cells)
	}
	return text + strings.Repeat(" ", max(cells-rowWidth(text), 0))
}

func rowDetail(r crewview.Row) string {
	var parts []string
	if r.Detail != "" {
		parts = append(parts, r.Status+": "+r.Detail)
	}
	if r.RunID != "" && r.RunID != noRunID {
		parts = append(parts, "run "+r.RunID)
	}
	return strings.Join(parts, " · ")
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
	if s.view.menu.step(k) {
		s.dirty = true
		return
	}
	switch k.Type {
	case KeyEnter:
		s.followTask()
	case KeyHome:
		s.selectTask(0)
	case KeyEnd:
		s.selectTask(len(s.view.snap.Rows) - 1)
	case KeyPgUp:
		s.selectTask(s.view.menu.selected - menuRows)
	case KeyPgDn:
		s.selectTask(s.view.menu.selected + menuRows)
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

func (s *Session) selectTask(index int) {
	s.view.menu.jump(index)
	s.dirty = true
}

func (s *Session) viewRune(r rune) {
	agent := s.cfg.Agent.(CrewAgent)
	id := s.view.snap.ID
	switch {
	case r >= '1' && r <= '9':
		if index := int(r - '1'); index < len(s.view.snap.Rows) {
			s.selectTask(index)
		}
		return
	case r == 'g':
		s.selectTask(0)
		return
	case r == 'G':
		s.selectTask(len(s.view.snap.Rows) - 1)
		return
	}
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
	var parts []string
	if total := len(s.view.snap.Rows); total > menuRows {
		parts = append(parts, fmt.Sprintf("%d of %d", s.view.menu.selected+1, total))
	}
	parts = append(parts, crewViewHint)
	if row, ok := s.selectedRow(); ok {
		for _, r := range row.Keys {
			parts = append(parts, string(r)+" "+viewKeyVerbs[r])
		}
	}
	switch {
	case s.view.snap.Paused:
		parts = append(parts, "p resume")
	case !s.view.snap.Done:
		parts = append(parts, "p pause")
	}
	return wrapHint(append(parts, "Esc closes"), s.cfg.Width())
}

func wrapHint(parts []string, width int) string {
	var lines []string
	line := ""
	for _, part := range parts {
		switch {
		case line == "":
			line = part
		case rowWidth(line)+rowWidth(hintJoin)+rowWidth(part) > width:
			lines = append(lines, line)
			line = part
		default:
			line += hintJoin + part
		}
	}
	return strings.Join(append(lines, line), "\n")
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
	rows := crewRows(snap, s.cfg.Width())
	if strings.Join(rows, "\n") != strings.Join(s.view.menu.items, "\n") || viewSignature(snap) != viewSignature(s.view.snap) {
		s.dirty = true
	}
	s.view.snap = snap
	s.view.menu.items = rows
	s.view.menu.selected = min(s.view.menu.selected, max(len(rows)-1, 0))
	s.view.menu.keepVisible()
}

func viewSignature(snap crewview.Snapshot) string {
	parts := []string{strconv.FormatBool(snap.Paused), strconv.FormatBool(snap.Done)}
	for _, r := range snap.Rows {
		parts = append(parts, r.Keys, r.Detail, r.RunID)
	}
	return strings.Join(parts, "\x00")
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
		live.done = true
	}
	s.dirty = true
}

func (s *Session) drawView() {
	if s.view.diff != nil {
		s.drawPager(s.view.diff)
		return
	}
	if s.view.live != nil {
		verb := "following "
		if s.view.live.done {
			verb = "end of "
		}
		_ = s.renderer.Draw(verb+s.view.snap.Rows[s.view.live.index].Title+" · "+crewBackHint, nil, 0, s.footer())
		return
	}
	rows, sel := s.view.menu.window()
	more := s.viewHint()
	if s.view.ask != nil {
		more = s.view.ask.text
	}
	prompt := fmt.Sprintf("crew %s · %s", s.view.snap.ID, s.view.snap.Label)
	if s.view.snap.Paused {
		prompt += " · paused"
	}
	note := ""
	if row, ok := s.selectedRow(); ok && s.view.ask == nil {
		note = rowDetail(row)
	}
	_ = s.renderer.DrawMenu(prompt, nil, 0, rows, sel, note, more, s.footer())
}
