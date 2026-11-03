package tui

func (s *Session) updateCompletion() {
	if s.picker != nil || s.masking || s.busy() {
		s.closeCompletion()
		return
	}
	kind, start, end, word, ok := tokenAt(s.editor.Runes(), s.editor.Cursor())
	if !ok {
		s.closeCompletion()
		return
	}
	switch kind {
	case completeSlash:
		if s.complete == nil || s.complete.kind != completeSlash {
			s.complete = newSlashCompletion()
		}
	case completeFile:
		if s.fileCache == nil {
			files, err := s.cfg.Agent.Files()
			if err != nil {
				s.fail(err)
				s.closeCompletion()
				return
			}
			s.fileCache = files
		}
		if s.complete == nil || s.complete.kind != completeFile {
			s.complete = &completion{menu: newMenu(s.fileCache), kind: completeFile}
		}
	}
	s.complete.start = start
	s.complete.end = end
	s.complete.menu.setFilter(word)
	if !s.complete.visible() {
		s.complete = nil
	}
}

func (s *Session) closeCompletion() {
	s.complete = nil
}

func (s *Session) clearPromptCompletion() {
	s.closeCompletion()
	s.clearFileCache()
}

func (s *Session) clearFileCache() {
	s.fileCache = nil
}

func (s *Session) applyCompletion() {
	if s.complete == nil || !s.complete.visible() {
		return
	}
	chosen, ok := s.complete.menu.chosen()
	if !ok {
		return
	}
	var insert string
	switch s.complete.kind {
	case completeSlash:
		insert = "/" + chosen + " "
	case completeFile:
		insert = "@" + chosen + " "
	default:
		return
	}
	s.editor.Replace(s.complete.start, s.complete.end, insert)
	s.closeCompletion()
	s.dirty = true
}

func (s *Session) completionKey(k Key) (handled bool) {
	if s.complete == nil || !s.complete.visible() {
		return false
	}
	switch k.Type {
	case KeyUp:
		s.complete.menu.move(-1)
		s.dirty = true
		return true
	case KeyDown:
		s.complete.menu.move(1)
		s.dirty = true
		return true
	case KeyTab:
		s.applyCompletion()
		return true
	case KeyEnter:
		s.applyCompletion()
		return true
	case KeyEsc:
		s.closeCompletion()
		s.dirty = true
		return true
	}
	return false
}
