package tui

import (
	"errors"
	"fmt"
	"strings"
)

const (
	maxErrorRunes = 200
	unknownHint   = "unknown command %s, /help lists them, // sends a literal /"
	quitHint      = "press Ctrl-C again or Ctrl-D to quit"
	literalPrefix = "//"
	jevUsage      = "usage: /jev on|off"
	keyUsage      = "usage: /key NAME"
	keyNotSaved   = "key not saved"
	keyAddHint    = "add it with /key TYPESAFE_API_KEY"
)

var helpLines = []string{
	"/help          this list",
	"/model         pick a provider and model",
	"/model p/name  switch the model for following turns",
	"/jev           show the jev state",
	"/jev on|off    switch jev for following turns",
	"/key NAME      store an API key",
	"/new           start a fresh run",
	"/runs          list recent runs",
	"/quit          leave, also Ctrl-D",
	"//text         send /text to the model",
	"Ctrl-C or Esc stops a turn",
}

func splitSlash(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "/") || strings.HasPrefix(text, literalPrefix) {
		return "", "", false
	}
	fields := strings.Fields(text)
	return fields[0], strings.Join(fields[1:], " "), true
}

func promptText(text string) string {
	if strings.HasPrefix(text, literalPrefix) {
		return text[1:]
	}
	return text
}

func (s *Session) runSlash(name, arg string) {
	switch name {
	case "/help":
		s.showHelp()
	case "/model":
		if arg == "" {
			s.startPicker()
			return
		}
		s.switchModel(arg)
	case "/jev":
		s.switchJev(arg)
	case "/key":
		s.startKey(arg)
	case "/new":
		s.newRun()
	case "/runs":
		s.showRuns()
	case "/quit":
		s.quit = true
	default:
		s.note(fmt.Sprintf(unknownHint, name))
	}
}

func (s *Session) showHelp() {
	for _, line := range helpLines {
		s.note(line)
	}
}

func (s *Session) startPicker() {
	items := s.cfg.Agent.Providers()
	if len(items) == 0 {
		s.note(pickerEmpty)
		return
	}
	s.picker = newPicker(items)
	s.dirty = true
}

func (s *Session) switchModel(spec string) {
	if spec != "" {
		if err := s.cfg.Agent.SetModel(spec); err != nil {
			s.fail(err)
			return
		}
	}
	s.note("model " + s.cfg.Agent.Model())
}

func (s *Session) switchJev(arg string) {
	switch arg {
	case "":
		s.note(jevLine(s.cfg.Agent.Jev()))
	case "on", "off":
		s.setJev(arg == "on")
	default:
		s.note(jevUsage)
	}
}

func (s *Session) setJev(on bool) {
	if err := s.cfg.Agent.SetJev(on); err != nil {
		s.fail(err)
		if isKeyMissing(err) {
			s.note(keyAddHint)
		}
		return
	}
	s.note(jevLine(on))
}

func isKeyMissing(err error) bool {
	var missing KeyMissingError
	return errors.As(err, &missing)
}

func (s *Session) startKey(name string) {
	if name == "" {
		s.note(keyUsage)
		return
	}
	s.keyName = name
	s.masking = true
	s.editor.Clear()
	s.dirty = true
}

func jevLine(on bool) string {
	if on {
		return "jev on"
	}
	return "jev off"
}

func (s *Session) newRun() {
	if err := s.cfg.Agent.Reset(); err != nil {
		s.fail(err)
		return
	}
	s.note("new run")
}

func (s *Session) showRuns() {
	lines, err := s.cfg.Agent.Runs()
	if err != nil {
		s.fail(err)
		return
	}
	for _, line := range lines {
		s.note(line)
	}
}
