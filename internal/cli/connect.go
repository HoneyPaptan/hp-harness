package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/mcp"
)

const (
	maxToolName    = 64
	maxToolSummary = 1024
	minSecretLen   = 6
	toolSeparator  = "__"
)

var (
	refPattern    = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)
	unsafeToolRun = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	serverEnvKeys = []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TERM", "LANG", "LC_ALL", "TMPDIR"}
)

func expandRefs(value string, getenv func(string) string) (string, []string, error) {
	var secrets []string
	var missing error
	out := refPattern.ReplaceAllStringFunc(value, func(ref string) string {
		v := getenv(ref[1:])
		if v == "" && missing == nil {
			missing = fmt.Errorf("%s is not set", ref)
		}
		secrets = append(secrets, v)
		return v
	})
	return out, secrets, missing
}

func clientConfig(env Env, name string, c connection) (mcp.Config, error) {
	cfg := mcp.Config{
		Name: name, Command: c.Command, Args: c.Args, Dir: c.Dir, URL: c.URL, Version: env.Version,
	}
	if c.Timeout != "" {
		d, err := parseTimeout(c.Timeout)
		if err != nil {
			return mcp.Config{}, err
		}
		cfg.Timeout = d
	}
	if err := resolveHeaders(&cfg, c, env.Getenv); err != nil {
		return mcp.Config{}, err
	}
	if err := resolveEnv(&cfg, c, env.Getenv); err != nil {
		return mcp.Config{}, err
	}
	return cfg, nil
}

func parseTimeout(value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, usagef("timeout %q must be a positive duration like 90s or 5m", value)
	}
	return d, nil
}

func resolveHeaders(cfg *mcp.Config, c connection, getenv func(string) string) error {
	if len(c.Headers) == 0 {
		return nil
	}
	cfg.Headers = make(map[string]string, len(c.Headers))
	for k, v := range c.Headers {
		out, _, err := expandRefs(v, getenv)
		if err != nil {
			return fmt.Errorf("header %s: %w", k, err)
		}
		cfg.Headers[k] = out
		addSecrets(cfg, out)
	}
	return nil
}

func resolveEnv(cfg *mcp.Config, c connection, getenv func(string) string) error {
	if c.Command == "" {
		return nil
	}
	cfg.Env = serverBaseEnv(getenv)
	for _, k := range slices.Sorted(maps.Keys(c.Env)) {
		out, secrets, err := expandRefs(c.Env[k], getenv)
		if err != nil {
			return fmt.Errorf("env %s: %w", k, err)
		}
		cfg.Env = append(cfg.Env, k+"="+out)
		addSecrets(cfg, secrets...)
	}
	return nil
}

func addSecrets(cfg *mcp.Config, values ...string) {
	for _, v := range values {
		if len(v) >= minSecretLen {
			cfg.Secrets = append(cfg.Secrets, v)
		}
	}
}

func serverBaseEnv(getenv func(string) string) []string {
	out := make([]string, 0, len(serverEnvKeys))
	for _, k := range serverEnvKeys {
		if v := getenv(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func exposeTools(server string, tools []mcp.Tool) []cachedTool {
	sorted := slices.Clone(tools)
	slices.SortStableFunc(sorted, func(a, b mcp.Tool) int { return strings.Compare(a.Name, b.Name) })
	used := make(map[string]bool, len(sorted))
	out := make([]cachedTool, 0, len(sorted))
	for _, t := range sorted {
		name := uniqueToolName(server+toolSeparator+unsafeToolRun.ReplaceAllString(t.Name, "_"), used)
		used[name] = true
		out = append(out, cachedTool{
			Name: name, Remote: t.Name,
			Description: cutRunes(t.Description, maxToolSummary), Schema: objectSchema(t.Schema),
		})
	}
	return out
}

func uniqueToolName(base string, used map[string]bool) string {
	name := base[:min(len(base), maxToolName)]
	for n := 2; used[name]; n++ {
		suffix := "-" + strconv.Itoa(n)
		name = base[:min(len(base), maxToolName-len(suffix))] + suffix
	}
	return name
}

func objectSchema(raw json.RawMessage) json.RawMessage {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || m == nil {
		m = map[string]any{}
	}
	if _, ok := m["type"]; !ok {
		m["type"] = "object"
	}
	if _, ok := m["properties"]; !ok {
		m["properties"] = map[string]any{}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return out
}
