package tui

import "strings"

const (
	approvePrompt = "allow? "
	approveHint   = "1 2 3 or ↑ ↓ Enter · Tab adds a note to Yes or No\nCtrl-C says no · Esc says no and stops the turn"
	noteHint      = "Enter sends it · Esc goes back to the choices · Ctrl-C says no"
	noteYesPrompt = "yes, and after this turn: "
	noteNoPrompt  = "no, because: "
)

var (
	approveItems   = []string{"1 Yes", "2 Yes, and don't ask again for this", "3 No"}
	approveAnswers = []ApproveAnswer{ApproveYes, ApproveAlways, ApproveNo}
	approveRunes   = map[rune]ApproveAnswer{
		'1': ApproveYes, 'y': ApproveYes, 'Y': ApproveYes,
		'2': ApproveAlways, 'a': ApproveAlways, 'A': ApproveAlways,
		'3': ApproveNo, 'n': ApproveNo, 'N': ApproveNo,
	}
)

type approval struct {
	reply    chan<- Approval
	menu     *menu
	answered bool
	noting   bool
	note     Editor
}

func (a *approval) answer() ApproveAnswer {
	return approveAnswers[a.menu.selected]
}

func (s *Session) openApproval(ev Event) {
	s.dropApproval()
	s.out.line(sgrDim, ev.Text)
	s.approval = &approval{reply: ev.Reply, menu: newMenu(approveItems)}
	s.drawApproval()
}

func (s *Session) dropApproval() {
	if s.approval == nil {
		return
	}
	s.approval = nil
	s.clearDrawn()
}

func (s *Session) drawApproval() {
	a := s.approval
	if a == nil {
		return
	}
	rows, sel := a.menu.window()
	s.drawn = true
	if !a.noting {
		s.renderer.Select(0, 0)
		_ = s.renderer.DrawMenu(approvePrompt, nil, 0, rows, sel, "", approveHint, footerLine(s.cfg.Agent))
		return
	}
	from, to, _ := a.note.Selection()
	s.renderer.Select(from, to)
	_ = s.renderer.DrawMenu(notePrompt(a.answer()), a.note.Runes(), a.note.Cursor(), rows, sel, "", noteHint, footerLine(s.cfg.Agent))
}

func notePrompt(ans ApproveAnswer) string {
	if ans == ApproveNo {
		return noteNoPrompt
	}
	return noteYesPrompt
}

func (s *Session) approvalKey(k Key) {
	if s.approval.noting {
		s.noteKey(k)
		return
	}
	if s.approval.menu.step(k) {
		s.drawApproval()
		return
	}
	switch k.Type {
	case KeyLeft:
		s.approval.menu.move(-1)
		s.drawApproval()
	case KeyRight:
		s.approval.menu.move(1)
		s.drawApproval()
	case KeyTab:
		s.startNote()
	case KeyEnter:
		s.sendApproval(s.approval.answer(), "")
	case KeyCtrlC:
		s.sendApproval(ApproveNo, "")
	case KeyEsc:
		s.sendApproval(ApproveNo, "")
		s.interrupt()
	case KeyRune:
		if ans, ok := approveRunes[k.Rune]; ok {
			s.sendApproval(ans, "")
		}
	}
}

func (s *Session) startNote() {
	if s.approval.answer() == ApproveAlways {
		s.approval.menu.move(1)
	} else {
		s.approval.noting = true
	}
	s.drawApproval()
}

func (s *Session) noteKey(k Key) {
	a := s.approval
	switch k.Type {
	case KeyEnter:
		s.sendApproval(a.answer(), strings.TrimSpace(a.note.Text()))
	case KeyCtrlC:
		s.sendApproval(ApproveNo, "")
	case KeyEsc:
		a.noting = false
		a.note.Clear()
		s.drawApproval()
	case KeyUp, KeyDown, KeyTab, KeyCtrlD, KeyCtrlL, KeyCtrlN:
	default:
		a.note.Handle(k)
		s.drawApproval()
	}
}

func (s *Session) sendApproval(ans ApproveAnswer, note string) {
	if s.approval == nil || s.approval.answered {
		return
	}
	s.approval.answered = true
	ch := s.approval.reply
	s.approval = nil
	s.clearDrawn()
	s.out.line(sgrDim, approveResultLine(ans, note))
	s.out.activity()
	if ch != nil {
		ch <- Approval{Answer: ans, Note: declineNote(ans, note)}
	}
	if ans != ApproveNo && note != "" {
		s.enqueue(note)
	}
}

func declineNote(ans ApproveAnswer, note string) string {
	if ans != ApproveNo {
		return ""
	}
	return note
}

func approveResultLine(ans ApproveAnswer, note string) string {
	switch ans {
	case ApproveYes:
		return "allowed"
	case ApproveAlways:
		return "allowed for this session"
	}
	if note != "" {
		return "declined: " + note
	}
	return "declined"
}
