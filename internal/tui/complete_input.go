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
				s.fileCache = []string{}
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
	if s.complete.menu.step(k) {
		s.dirty = true
		return true
	}
	switch k.Type {
	case KeyTab:
		s.applyCompletion()
		return true
	case KeyEnter:
		if s.exactCompletion() {
			return false
		}
		s.applyCompletion()
		return true
	case KeyEsc:
		s.closeCompletion()
		s.dirty = true
		return true
	}
	return false
}

func (s *Session) exactCompletion() bool {
	chosen, ok := s.complete.menu.chosen()
	if !ok {
		return false
	}
	word := string(s.editor.Runes()[s.complete.start:s.complete.end])
	if s.complete.kind == completeSlash {
		return word == "/"+chosen
	}
	return word == "@"+chosen
}
