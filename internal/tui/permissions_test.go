package tui

import (
	"strings"
	"testing"
)

func TestSlashPermissions(t *testing.T) {
	a := newFakeAgent()
	a.permInfo = "rules in /home/t/.config/hp/settings.json"
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "/permissions\r"}, {wait: "permissions auto"}, {wait: "rules in /home/t/.config/hp/settings.json"}, {keys: "\x04"}})
	h.waitExit()
}

func TestSlashPermissionsSetMode(t *testing.T) {
	tests := []struct {
		keys string
		want string
	}{
		{"/permissions ask\r", "permissions ask"},
		{"/permissions auto\r", "permissions auto"},
		{"/permissions full-access\r", "permissions full-access"},
		{"/permissions full-access\r", "permissions full-access"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			a := newFakeAgent()
			h := startSession(t, a, false, false)
			h.play([]step{{keys: tc.keys}, {wait: tc.want}, {keys: "\x04"}})
			h.waitExit()
			if a.PermissionMode() != strings.TrimPrefix(tc.want, "permissions ") {
				t.Fatalf("mode %q", a.PermissionMode())
			}
		})
	}
}

func TestSlashPermissionsFullAccessWarning(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "/permissions full-access\r"}, {wait: fullAccessNote}, {keys: "\x04"}})
	h.waitExit()
}

func TestSlashPermissionsReset(t *testing.T) {
	a := newFakeAgent()
	a.permMode = "full-access"
	h := startSession(t, a, false, false)
	h.play([]step{{keys: "/permissions reset\r"}, {wait: "permissions ask"}, {keys: "\x04"}})
	h.waitExit()
	if a.PermissionMode() != "ask" {
		t.Fatalf("mode %q", a.PermissionMode())
	}
}

func TestSlashPermissionsBadArg(t *testing.T) {
	h := startSession(t, newFakeAgent(), false, false)
	h.play([]step{{keys: "/permissions nope\r"}, {wait: permUsage}, {keys: "\x04"}})
	h.waitExit()
}

func TestSessionShiftTabCyclesPermissions(t *testing.T) {
	a := newFakeAgent()
	h := startSession(t, a, false, false)
	h.play([]step{
		{keys: "\x1b[Z"},
		{wait: "permissions ask"},
		{keys: "\x1b[Z"},
		{wait: "permissions full-access"},
		{wait: fullAccessNote},
		{keys: "\x1b[Z"},
		{wait: "permissions auto"},
		{keys: "\x04"},
	})
	h.waitExit()
	if a.PermissionMode() != "auto" {
		t.Fatalf("mode %q", a.PermissionMode())
	}
}
