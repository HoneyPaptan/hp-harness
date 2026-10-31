package tui

import (
	"fmt"
	"strings"
)

const (
	maxErrorRunes = 200
	unknownHint   = "unknown command %s, /help lists them, // sends a literal /"
	quitHint      = "press Ctrl-C again or Ctrl-D to quit"
	literalPrefix = "//"
)

var helpLines = []string{
	"/help          this list",
	"/model         show the model",
	"/model p/name  switch the model for following turns",
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
		s.switchModel(arg)
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

func (s *Session) switchModel(spec string) {
	if spec != "" {
		if err := s.cfg.Agent.SetModel(spec); err != nil {
			s.fail(err)
			return
		}
	}
	s.note("model " + s.cfg.Agent.Model())
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
