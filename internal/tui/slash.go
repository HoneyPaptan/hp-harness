package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	maxErrorRunes   = 200
	unknownHint     = "unknown command %s, /help lists them, // sends a literal /"
	quitHint        = "press Ctrl-C again or Ctrl-D to quit"
	literalPrefix   = "//"
	jevUsage        = "usage: /jev on|off"
	keyUsage        = "usage: /key NAME"
	keyNotSaved     = "key not saved"
	keyAddHint      = "add it with /key TYPESAFE_API_KEY"
	queuedNote      = "queued %d: %s"
	tookBackNote    = "took back: %s"
	busySlashHint   = "slash commands wait for the turn: Esc stops it, then Enter runs this"
	queuedNoteRunes = 60
	permUsage       = "usage: /permissions [ask|auto|full-access|reset]"
	fullAccessNote  = "full access: no permission checks"
)

type slashCommand struct {
	name string
	args string
	help string
}

var slashCommands = []slashCommand{
	{"/help", "", "this list"},
	{"/model", "", "pick a provider and model"},
	{"/model", " p/name", "switch the model for following turns"},
	{"/jev", "", "show the jev state"},
	{"/jev", " on|off", "switch jev for following turns"},
	{"/key", " NAME", "store an API key"},
	{"/new", "", "start a fresh run"},
	{"/runs", "", "list recent runs"},
	{"/permissions", "", "show or set permission mode"},
	{"/permissions", " ask|auto|full-access|reset", "switch mode or reset session grants"},
	{"/compact", "", "compact the conversation now"},
	{"/crew", "", "open the newest crew"},
	{"/crew", " ID", "open an older crew"},
	{"/crew", " plan.md|goal", "run a plan file or a drafted goal as a crew"},
	{"/crew", " list", "list crews"},
	{"/crew", " status|report ID", "print one crew"},
	{"/quit", "", "leave, also Ctrl-D"},
}

var helpLines = append(commandHelpLines(),
	"//text         send /text to the model",
	"Ctrl-C or Esc stops a turn",
)

func commandHelpLines() []string {
	lines := make([]string, 0, len(slashCommands))
	for _, c := range slashCommands {
		lines = append(lines, slashHelpLine(c))
	}
	return lines
}

func slashHelpLine(c slashCommand) string {
	line := c.name
	if c.args != "" {
		line += c.args
	}
	if len(line) < 15 {
		line += strings.Repeat(" ", 15-len(line))
	} else {
		line += " "
	}
	return line + c.help
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

func (s *Session) runSlash(ctx context.Context, name, arg string) {
	switch name {
	case "/help":
		s.showHelp()
	case "/model":
		if arg == "" {
			s.startPicker()
			return
		}
		if name := providerOnlyArg(arg); name != "" {
			if slices.Contains(s.cfg.Agent.Providers(), name) {
				s.startPickerAtProvider(name)
				return
			}
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
	case "/permissions":
		s.switchPermissions(arg)
	case "/compact":
		s.runCompact(ctx)
	case "/crew":
		s.runCrew(ctx, arg)
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

func (s *Session) startPicker() bool {
	items := s.cfg.Agent.Providers()
	if len(items) == 0 {
		s.note(pickerEmpty)
		return false
	}
	s.picker = newPicker(items)
	s.dirty = true
	return true
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

func (s *Session) runCompact(ctx context.Context) {
	if s.busy() {
		return
	}
	ok, err := s.cfg.Agent.Compact(ctx)
	if err != nil {
		s.fail(err)
		return
	}
	if ok {
		s.note("compacted")
	} else {
		s.note("nothing to compact")
	}
}

func (s *Session) switchPermissions(arg string) {
	if arg == "" {
		s.note("permissions " + s.cfg.Agent.PermissionMode())
		if info := s.cfg.Agent.PermissionInfo(); info != "" {
			s.note(info)
		}
		return
	}
	prev := s.cfg.Agent.PermissionMode()
	if err := s.cfg.Agent.SetPermissionMode(arg); err != nil {
		s.note(permUsage)
		return
	}
	mode := s.cfg.Agent.PermissionMode()
	s.note("permissions " + mode)
	if mode == "full-access" && prev != "full-access" {
		s.note(fullAccessNote)
	}
	s.dirty = true
}

var permissionCycle = []string{"auto", "ask", "full-access"}

func (s *Session) cyclePermissions() {
	if s.complete != nil && s.complete.visible() {
		return
	}
	cur := s.cfg.Agent.PermissionMode()
	next := permissionCycle[0]
	for i, m := range permissionCycle {
		if m == cur {
			next = permissionCycle[(i+1)%len(permissionCycle)]
			break
		}
	}
	prev := cur
	s.clearDrawn()
	if err := s.cfg.Agent.SetPermissionMode(next); err != nil {
		s.fail(err)
		return
	}
	s.note("permissions " + next)
	if next == "full-access" && prev != "full-access" {
		s.note(fullAccessNote)
	}
	s.dirty = true
}
