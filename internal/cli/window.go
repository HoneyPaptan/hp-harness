package cli

import "strings"

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

func contextWindow(st settings, runSpec string) func(string) int {
	afterPreset := modelAfterPreset(runSpec)
	return func(model string) int {
		if w, ok := st.ContextWindows[runSpec]; ok {
			return w
		}
		if w, ok := st.ContextWindows[afterPreset]; ok {
			return w
		}
		if w, ok := st.ContextWindows[model]; ok {
			return w
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
