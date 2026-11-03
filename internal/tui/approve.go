package tui

const (
	approvePrompt = "allow? "
	approveYes    = "Yes"
	approveAlways = "Yes, and don't ask again for this"
	approveNo     = "No"
)

var approveItems = []string{approveYes, approveAlways, approveNo}

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
	_ = s.renderer.DrawMenu(approvePrompt, nil, 0, rows, sel, "", "", footerLine(s.cfg.Agent))
}

func (s *Session) approvalKey(k Key) {
	switch k.Type {
	case KeyUp:
		s.approval.menu.move(-1)
		s.drawApproval()
	case KeyDown:
		s.approval.menu.move(1)
		s.drawApproval()
	case KeyEnter:
		item, ok := s.approval.menu.chosen()
		if !ok {
			return
		}
		s.sendApproval(approveFromItem(item))
	case KeyEsc:
		s.sendApproval(ApproveNo)
	case KeyCtrlC:
		s.sendApproval(ApproveNo)
		s.interrupt()
	case KeyRune:
		switch k.Rune {
		case 'y', 'Y':
			s.sendApproval(ApproveYes)
		case 'a', 'A':
			s.sendApproval(ApproveAlways)
		case 'n', 'N':
			s.sendApproval(ApproveNo)
		}
	}
}

func approveFromItem(item string) ApproveAnswer {
	switch item {
	case approveYes:
		return ApproveYes
	case approveAlways:
		return ApproveAlways
	default:
		return ApproveNo
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
