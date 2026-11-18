package tui

const (
	approvePrompt = "allow? "
	approveHint   = "1 2 3 or ↑ ↓ Enter · Ctrl-C says no · Esc says no and stops the turn"
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
	reply    chan<- ApproveAnswer
	menu     *menu
	answered bool
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
	if s.approval == nil {
		return
	}
	rows, sel := s.approval.menu.window()
	s.drawn = true
	_ = s.renderer.DrawMenu(approvePrompt, nil, 0, rows, sel, "", approveHint, footerLine(s.cfg.Agent))
}

func (s *Session) approvalKey(k Key) {
	if s.approval.menu.step(k) {
		s.drawApproval()
		return
	}
	switch k.Type {
	case KeyLeft:
		s.approval.menu.move(-1)
		s.drawApproval()
	case KeyRight, KeyTab:
		s.approval.menu.move(1)
		s.drawApproval()
	case KeyEnter:
		s.sendApproval(approveAnswers[s.approval.menu.selected])
	case KeyCtrlC:
		s.sendApproval(ApproveNo)
	case KeyEsc:
		s.sendApproval(ApproveNo)
		s.interrupt()
	case KeyRune:
		if ans, ok := approveRunes[k.Rune]; ok {
			s.sendApproval(ans)
		}
	}
}

func (s *Session) sendApproval(ans ApproveAnswer) {
	if s.approval == nil || s.approval.answered {
		return
	}
	s.approval.answered = true
	ch := s.approval.reply
	s.approval = nil
	s.clearDrawn()
	s.out.line(sgrDim, approveResultLine(ans))
	s.out.activity()
	if ch != nil {
		ch <- ans
	}
}

func approveResultLine(ans ApproveAnswer) string {
	switch ans {
	case ApproveYes:
		return "allowed"
	case ApproveAlways:
		return "allowed for this session"
	default:
		return "declined"
	}
}
