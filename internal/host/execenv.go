package host

import (
	"os"
	"strings"
)

func ToolExecEnv(from []string) []string {
	base := from
	if base == nil {
		base = os.Environ()
	}
	return toolExecEnv(base)
}

func toolExecEnv(base []string) []string {
	out := make([]string, 0, len(base)+1)
	for _, e := range base {
		key, _, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if key == "HP_DETACHED" || key == "AI_AGENT" {
			continue
		}
		out = append(out, e)
	}
	out = append(out, "AI_AGENT=hp")
	return out
}
