package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
)

type autostartPaths struct {
	unit  string
	label string
}

func hpAutostartPath(env Env) autostartPaths {
	if env.Os == "darwin" {
		return autostartPaths{
			unit:  path.Join(env.Getenv("HOME"), "Library", "LaunchAgents", "com.honeypaptan.hp-recover.plist"),
			label: "com.honeypaptan.hp-recover",
		}
	}
	return autostartPaths{
		unit:  path.Join(env.Getenv("HOME"), ".config", "systemd", "user", "hp-recover.service"),
		label: "hp-recover.service",
	}
}

func cmdAutostart(env Env, args []string) int {
	if len(args) != 1 {
		return exitUsage(env, usagef("usage: hp autostart on|off|status"))
	}
	p := hpAutostartPath(env)
	switch args[0] {
	case "on":
		return autostartOn(env, p)
	case "off":
		return autostartOff(env, p)
	case "status":
		return autostartStatus(env, p)
	default:
		return exitUsage(env, usagef("unknown action %q, use on|off|status", args[0]))
	}
}

func autostartOn(env Env, p autostartPaths) int {
	if env.Exe == "" {
		return exitUsage(env, usagef("cannot resolve the hp binary path"))
	}
	if err := env.Host.WriteFile(p.unit, []byte(unitText(env.Os, env.Exe, p.label))); err != nil {
		return exitErr(env, "", "", err)
	}
	fmt.Fprintf(env.Stdout, "wrote %s\n", p.unit)
	if err := enableAutostart(env, p); err != nil {
		return exitErr(env, "", "", err)
	}
	printKeyNote(env, p)
	return 0
}

func enableAutostart(env Env, p autostartPaths) error {
	var argv []string
	if env.Os == "darwin" {
		argv = []string{"load", "-w", p.unit}
	} else {
		argv = []string{"--user", "enable", p.label}
	}
	if env.RunCtl == nil {
		return errors.New("no systemctl runner configured")
	}
	if err := env.RunCtl(argv); err != nil {
		return err
	}
	if env.Os == "darwin" {
		fmt.Fprintln(env.Stdout, "loaded with launchctl")
		return nil
	}
	fmt.Fprintln(env.Stdout, "enabled with systemctl --user enable")
	fmt.Fprintln(env.Stdout, "for recovery at boot without login, run: loginctl enable-linger $USER")
	return nil
}

func printKeyNote(env Env, p autostartPaths) {
	fmt.Fprintln(env.Stdout, "the unit holds no API keys")
	if env.Os == "darwin" {
		fmt.Fprintln(env.Stdout, "recovery needs the keys in the launchd environment, see README")
		return
	}
	fmt.Fprintln(env.Stdout, "recovery reads keys from %h/.config/hp/env via EnvironmentFile")
	fmt.Fprintln(env.Stdout, "create that file yourself, mode 0600; hp never writes keys into it")
}

func autostartOff(env Env, p autostartPaths) int {
	var argv []string
	if env.Os == "darwin" {
		argv = []string{"unload", p.unit}
	} else {
		argv = []string{"--user", "disable", p.label}
	}
	if env.RunCtl != nil {
		_ = env.RunCtl(argv)
	}
	if err := env.Host.Remove(p.unit); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return exitErr(env, "", "", err)
	}
	fmt.Fprintf(env.Stdout, "removed %s\n", p.unit)
	return 0
}

func autostartStatus(env Env, p autostartPaths) int {
	_, err := env.Host.Stat(p.unit)
	switch {
	case err == nil:
		fmt.Fprintf(env.Stdout, "installed: %s\n", p.unit)
	case errors.Is(err, fs.ErrNotExist):
		fmt.Fprintln(env.Stdout, "not installed")
	default:
		return exitErr(env, "", "", err)
	}
	return 0
}

func unitText(osName, exe, label string) string {
	if osName == "darwin" {
		return launchdPlist(exe, label)
	}
	return systemdUnit(exe)
}

func systemdUnit(exe string) string {
	return "[Unit]\n" +
		"Description=hp recover\n" +
		"\n" +
		"[Service]\n" +
		"Type=oneshot\n" +
		"ExecStart=" + exe + " recover\n" +
		"EnvironmentFile=-%h/.config/hp/env\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}

func launchdPlist(exe, label string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + label + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + exe + `</string>
    <string>recover</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`
}
