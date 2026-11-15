package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/permit"
)

const settingsFileName = "settings.json"

type settings struct {
	PermissionMode     string          `json:"permission_mode"`
	Permission         json.RawMessage `json:"permission"`
	Context            *bool           `json:"context"`
	Web                *bool           `json:"web"`
	AutoCompactPercent *int            `json:"auto_compact_percent"`
	ContextWindows     map[string]int  `json:"context_windows"`
}

func settingsPath(getenv func(string) string) (string, error) {
	dir, err := userConfigDir(getenv)
	if err != nil {
		return "", err
	}
	return path.Join(dir, settingsFileName), nil
}

func loadSettings(env Env) (settings, error) {
	file, err := settingsPath(rawGetenv(env))
	if err != nil {
		return settings{}, err
	}
	data, err := readConfigFile(env, file)
	if errors.Is(err, fs.ErrNotExist) {
		return settings{}, nil
	}
	if err != nil {
		return settings{}, err
	}
	var st settings
	if err := json.Unmarshal(data, &st); err != nil {
		return settings{}, fmt.Errorf("%s: %w", file, err)
	}
	if st.PermissionMode != "" {
		if _, err := permit.ParseMode(st.PermissionMode); err != nil {
			return settings{}, fmt.Errorf("%s: %w", file, err)
		}
	}
	if err := validateCompactionSettings(file, st); err != nil {
		return settings{}, err
	}
	return st, nil
}

func validateCompactionSettings(file string, st settings) error {
	if st.AutoCompactPercent != nil {
		p := *st.AutoCompactPercent
		if p < 10 || p > 80 {
			return fmt.Errorf("%s: auto_compact_percent", file)
		}
	}
	for spec, w := range st.ContextWindows {
		if w <= 0 {
			return fmt.Errorf("%s: context_windows[%q]", file, spec)
		}
	}
	return nil
}

const defaultContextWindow = 128000

var contextWindowPrefixes = []struct {
	prefix string
	size   int
}{
	{"gpt-oss", 131072},
	{"claude-", 200000},
	{"llama-3.3-70b", 131072},
	{"kimi-k2", 131072},
}

func contextWindow(st settings, runSpec, runModel string) func(string) int {
	afterPreset := modelAfterPreset(runSpec)
	return func(model string) int {
		if w, ok := st.ContextWindows[model]; ok {
			return w
		}
		if model == runModel {
			if w, ok := st.ContextWindows[runSpec]; ok {
				return w
			}
			if w, ok := st.ContextWindows[afterPreset]; ok {
				return w
			}
		}
		for _, e := range contextWindowPrefixes {
			if strings.HasPrefix(model, e.prefix) {
				return e.size
			}
		}
		return defaultContextWindow
	}
}

func modelAfterPreset(spec string) string {
	if i := strings.IndexByte(spec, '/'); i >= 0 {
		return spec[i+1:]
	}
	return spec
}

func compactPercent(st settings) int {
	if st.AutoCompactPercent == nil {
		return 0
	}
	return *st.AutoCompactPercent
}

func loadPermissionRules(st settings) ([]permit.Rule, error) {
	if len(st.Permission) == 0 {
		return nil, nil
	}
	return permit.ParseRules(st.Permission)
}

func loadSettingsAndRules(env Env) (settings, []permit.Rule, error) {
	st, err := loadSettings(env)
	if err != nil {
		return settings{}, nil, err
	}
	rules, err := loadPermissionRules(st)
	if err != nil {
		file, fErr := settingsPath(rawGetenv(env))
		if fErr != nil {
			return settings{}, nil, err
		}
		return settings{}, nil, fmt.Errorf("%s: %w", file, err)
	}
	return st, rules, nil
}

func readConfigFile(env Env, file string) ([]byte, error) {
	if env.Browser {
		return env.Host.ReadFile(file)
	}
	return os.ReadFile(file)
}
