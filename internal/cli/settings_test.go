package cli

import (
	"os"
	"path"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/permit"
)

func settingsTestEnv(t *testing.T, content string) (Env, string) {
	t.Helper()
	home := t.TempDir()
	env, _, _ := testEnv(t, host.NewMemory(), nil, map[string]string{"HOME": home})
	if content != "" {
		writeSettingsFile(t, home, content)
	}
	file, err := settingsPath(env.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	return env, file
}

func writeSettingsFile(t *testing.T, home, content string) {
	t.Helper()
	file := path.Join(home, ".config", "hp", "settings.json")
	if err := os.MkdirAll(path.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSettingsMissing(t *testing.T) {
	env, _ := settingsTestEnv(t, "")
	st, err := loadSettings(env)
	if err != nil {
		t.Fatal(err)
	}
	if st.PermissionMode != "" || len(st.Permission) != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestLoadSettingsValid(t *testing.T) {
	env, file := settingsTestEnv(t, `{"permission_mode":"ask","permission":{"bash":{"git push *":"deny"}}}`)
	st, err := loadSettings(env)
	if err != nil {
		t.Fatal(err)
	}
	if st.PermissionMode != "ask" {
		t.Fatal(st.PermissionMode)
	}
	rules, err := loadPermissionRules(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Tool != "bash" || rules[0].Pattern != "git push *" {
		t.Fatalf("%+v", rules)
	}
	_ = file
}

func TestLoadSettingsBadJSON(t *testing.T) {
	env, file := settingsTestEnv(t, `{`)
	_, err := loadSettings(env)
	if err == nil || !strings.Contains(err.Error(), file) {
		t.Fatalf("err %v", err)
	}
}

func TestLoadSettingsBadMode(t *testing.T) {
	env, file := settingsTestEnv(t, `{"permission_mode":"bogus"}`)
	_, err := loadSettings(env)
	if err == nil || !strings.Contains(err.Error(), file) {
		t.Fatalf("err %v", err)
	}
}

func TestLoadSettingsUnknownKeysIgnored(t *testing.T) {
	env, _ := settingsTestEnv(t, `{"permission_mode":"auto","extra":1}`)
	st, err := loadSettings(env)
	if err != nil {
		t.Fatal(err)
	}
	if st.PermissionMode != string(permit.ModeAuto) {
		t.Fatal(st.PermissionMode)
	}
}

func TestLoadSettingsRuleOrder(t *testing.T) {
	env, _ := settingsTestEnv(t, `{"permission":{"bash":{"a":"deny","b":"ask"},"read":"allow"}}`)
	st, err := loadSettings(env)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := loadPermissionRules(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
		t.Fatalf("len %d", len(rules))
	}
	if rules[0].Pattern != "a" || rules[1].Pattern != "b" || rules[2].Tool != "read" {
		t.Fatalf("%+v", rules)
	}
}

func TestLoadSettingsAutoCompactDefault(t *testing.T) {
	env, _ := settingsTestEnv(t, "")
	st, err := loadSettings(env)
	if err != nil {
		t.Fatal(err)
	}
	if st.AutoCompactPercent != nil {
		t.Fatal(st.AutoCompactPercent)
	}
	if got := compactPercent(st); got != 0 {
		t.Fatalf("percent %d", got)
	}
}

func TestLoadSettingsAutoCompactBounds(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		ok   bool
		want int
	}{
		{`{"auto_compact_percent":10}`, true, 10},
		{`{"auto_compact_percent":80}`, true, 80},
		{`{"auto_compact_percent":9}`, false, 0},
		{`{"auto_compact_percent":81}`, false, 0},
	} {
		env, file := settingsTestEnv(t, tc.raw)
		st, err := loadSettings(env)
		if tc.ok {
			if err != nil {
				t.Fatalf("%s: %v", tc.raw, err)
			}
			if st.AutoCompactPercent == nil || *st.AutoCompactPercent != tc.want {
				t.Fatalf("%s: %+v", tc.raw, st.AutoCompactPercent)
			}
		} else if err == nil || !strings.Contains(err.Error(), file) {
			t.Fatalf("%s: err %v", tc.raw, err)
		}
	}
}

func TestLoadSettingsAutoCompactNonInteger(t *testing.T) {
	env, file := settingsTestEnv(t, `{"auto_compact_percent":"80"}`)
	_, err := loadSettings(env)
	if err == nil || !strings.Contains(err.Error(), file) {
		t.Fatalf("err %v", err)
	}
}

func TestLoadSettingsContextWindows(t *testing.T) {
	env, file := settingsTestEnv(t, `{"context_windows":{"xai/grok":200000,"gpt-4":8192}}`)
	st, err := loadSettings(env)
	if err != nil {
		t.Fatal(err)
	}
	if st.ContextWindows["xai/grok"] != 200000 || st.ContextWindows["gpt-4"] != 8192 {
		t.Fatalf("%+v", st.ContextWindows)
	}
	_ = file
}

func TestLoadSettingsContextWindowsZero(t *testing.T) {
	env, file := settingsTestEnv(t, `{"context_windows":{"xai/grok":0}}`)
	_, err := loadSettings(env)
	if err == nil || !strings.Contains(err.Error(), file) {
		t.Fatalf("err %v", err)
	}
}
