package cli

import (
	"context"
	"path"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

const goldenSystemdUnit = `[Unit]
Description=hp recover

[Service]
Type=oneshot
RemainAfterExit=yes
KillMode=process
ExecStart="/usr/local/bin/hp" recover
EnvironmentFile=-%h/.config/hp/env

[Install]
WantedBy=default.target
`

const goldenLaunchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.honeypaptan.hp-recover</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/hp</string>
    <string>recover</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`

const testExe = "/usr/local/bin/hp"

type ctlCalls [][]string

func (c *ctlCalls) runner(args []string) error {
	*c = append(*c, append([]string(nil), args...))
	return nil
}

func TestAutostart(t *testing.T) {
	cases := []struct {
		name     string
		os       string
		args     []string
		unitPath string
	}{
		{name: "linux on", os: "linux", args: []string{"autostart", "on"},
			unitPath: path.Join("/home/t", ".config", "systemd", "user", "hp-recover.service")},
		{name: "linux off", os: "linux", args: []string{"autostart", "off"},
			unitPath: path.Join("/home/t", ".config", "systemd", "user", "hp-recover.service")},
		{name: "darwin on", os: "darwin", args: []string{"autostart", "on"},
			unitPath: path.Join("/home/t", "Library", "LaunchAgents", "com.honeypaptan.hp-recover.plist")},
		{name: "darwin off", os: "darwin", args: []string{"autostart", "off"},
			unitPath: path.Join("/home/t", "Library", "LaunchAgents", "com.honeypaptan.hp-recover.plist")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			env, stdout, stderr := testEnv(t, h, nil, nil)
			env.Exe = testExe
			env.Os = tc.os
			var ctl ctlCalls
			env.RunCtl = ctl.runner
			if tc.args[1] == "off" {
				if err := h.WriteFile(tc.unitPath, []byte("old")); err != nil {
					t.Fatal(err)
				}
			}
			env.Args = tc.args
			if code := Main(context.Background(), env); code != 0 {
				t.Fatalf("code %d %s", code, stderr.String())
			}
			mustNotBeGolden := tc.args[1] == "off"
			if _, err := env.Host.Stat(tc.unitPath); mustNotBeGolden && err == nil {
				t.Fatal("unit still there after off")
			}
			if !mustNotBeGolden {
				data, err := h.ReadFile(tc.unitPath)
				if err != nil {
					t.Fatal(err)
				}
				want := goldenSystemdUnit
				if tc.os == "darwin" {
					want = goldenLaunchdPlist
				}
				if string(data) != want {
					t.Fatalf("unit content mismatch:\n%s\ngot:\n%s", want, data)
				}
				if strings.Contains(string(data), "API_KEY") || strings.Contains(string(data), "sk-") {
					t.Fatal("unit holds a key")
				}
			}
			var wantCtl []string
			if tc.os == "darwin" && tc.args[1] == "on" {
				wantCtl = []string{"load", "-w", tc.unitPath}
			} else if tc.os == "darwin" {
				wantCtl = []string{"unload", tc.unitPath}
			} else if tc.args[1] == "on" {
				wantCtl = []string{"--user", "enable", "hp-recover.service"}
			} else {
				wantCtl = []string{"--user", "disable", "hp-recover.service"}
			}
			if len(ctl) != 1 {
				t.Fatalf("ctl calls %d", len(ctl))
			}
			if strings.Join(ctl[0], " ") != strings.Join(wantCtl, " ") {
				t.Fatalf("ctl %v", ctl[0])
			}
			wantOutput := "wrote "
			if mustNotBeGolden {
				wantOutput = "removed " + tc.unitPath
			}
			if !strings.Contains(stdout.String(), wantOutput) {
				t.Fatalf("stdout %q", stdout.String())
			}
			if tc.args[1] == "on" && tc.os == "linux" {
				for _, note := range []string{"no API keys", "0600", "loginctl enable-linger"} {
					if !strings.Contains(stdout.String(), note) {
						t.Fatalf("missing note %q in %q", note, stdout.String())
					}
				}
			}
		})
	}
}

func TestAutostartStatus(t *testing.T) {
	cases := []struct {
		name    string
		os      string
		written bool
		want    string
	}{
		{name: "linux installed", os: "linux", written: true, want: "installed:"},
		{name: "linux missing", os: "linux", want: "not installed"},
		{name: "darwin installed", os: "darwin", written: true, want: "installed:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			env, stdout, _ := testEnv(t, h, nil, nil)
			env.Os = tc.os
			p := hpAutostartPath(env)
			if tc.written {
				if err := env.Host.WriteFile(p.unit, []byte("unit")); err != nil {
					t.Fatal(err)
				}
			}
			env.Args = []string{"autostart", "status"}
			if code := Main(context.Background(), env); code != 0 {
				t.Fatal()
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout %q", stdout.String())
			}
		})
	}
}

func TestAutostartOnCtlFails(t *testing.T) {
	h := host.NewMemory()
	env, _, stderr := testEnv(t, h, nil, nil)
	env.Exe = testExe
	env.Os = "linux"
	env.RunCtl = func([]string) error { return context.DeadlineExceeded }
	env.Args = []string{"autostart", "on"}
	if code := Main(context.Background(), env); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Fatal(stderr.String())
	}
}

func TestAutostartBadArgs(t *testing.T) {
	cases := [][]string{
		{"autostart"},
		{"autostart", "yes", "no"},
		{"autostart", "restart"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := host.NewMemory()
			env, _, _ := testEnv(t, h, nil, nil)
			env.Args = args
			if code := Main(context.Background(), env); code != 2 {
				t.Fatalf("code %d", code)
			}
		})
	}
}

func TestSystemdUnitQuotesExe(t *testing.T) {
	got := systemdUnit(`/home/me/My 100% "tools"/hp`)
	want := `ExecStart="/home/me/My 100%% \"tools\"/hp" recover`
	if !strings.Contains(got, want+"\n") {
		t.Fatalf("unit %q missing %q", got, want)
	}
}

func TestLaunchdPlistEscapesExe(t *testing.T) {
	got := launchdPlist("/a&b/<hp>", "l")
	if !strings.Contains(got, "<string>/a&amp;b/&lt;hp&gt;</string>") {
		t.Fatal(got)
	}
}

func TestAutostartRejectsControlChars(t *testing.T) {
	h := host.NewMemory()
	env, _, _ := testEnv(t, h, nil, nil)
	env.Os = "linux"
	env.Exe = "/x\nExecStartPre=/bin/evil"
	env.RunCtl = func([]string) error { return nil }
	env.Args = []string{"autostart", "on"}
	if code := Main(context.Background(), env); code != 2 {
		t.Fatalf("code %d", code)
	}
}
