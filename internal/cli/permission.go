package cli

import (
	"fmt"

	"github.com/HoneyPaptan/hp-harness/internal/permit"
)

func resolvePermissionMode(fullAccess bool, getenv func(string) string, journalMode string, st settings) (permit.Mode, string, error) {
	if fullAccess {
		return permit.ModeFull, "flag", nil
	}
	if raw := getenv("HP_PERMISSION_MODE"); raw != "" {
		m, err := permit.ParseMode(raw)
		if err != nil {
			return "", "", fmt.Errorf("HP_PERMISSION_MODE: %w", err)
		}
		return m, "env", nil
	}
	if journalMode != "" {
		m, err := permit.ParseMode(journalMode)
		if err != nil {
			return "", "", err
		}
		return m, "journal", nil
	}
	if st.PermissionMode != "" {
		m, err := permit.ParseMode(st.PermissionMode)
		if err != nil {
			return "", "", err
		}
		return m, "settings", nil
	}
	return permit.ModeAuto, "default", nil
}

func effectivePermission(env Env, fullAccess bool, journalMode string) (permit.Mode, []permit.Rule, *permit.Grants, string, settings, error) {
	st, rules, err := loadSettingsAndRules(env)
	if err != nil {
		return "", nil, nil, "", settings{}, err
	}
	mode, source, err := resolvePermissionMode(fullAccess, rawGetenv(env), journalMode, st)
	if err != nil {
		return "", nil, nil, "", settings{}, err
	}
	return mode, rules, &permit.Grants{}, source, st, nil
}

func modeString(m permit.Mode) string {
	return string(m)
}
