package cli

import "strings"

type Endpoint struct {
	Preset, Model, BaseURL, KeyEnv, APIKey string
}

const (
	customPreset  = "custom"
	customBaseEnv = "HP_BASE_URL"
	customKeyEnv  = "HP_API_KEY"
	modelExample  = "hp -m groq/openai/gpt-oss-120b"
)

type presetDef struct {
	baseURL string
	keyEnv  string
}

var presets = map[string]presetDef{
	"opencode":   {baseURL: "https://opencode.ai/inference/openai/v1", keyEnv: "OPENCODE_API_KEY"},
	"openai":     {baseURL: "https://api.openai.com/v1", keyEnv: "OPENAI_API_KEY"},
	"anthropic":  {baseURL: "https://api.anthropic.com/v1", keyEnv: "ANTHROPIC_API_KEY"},
	"gemini":     {baseURL: "https://generativelanguage.googleapis.com/v1beta/openai", keyEnv: "GEMINI_API_KEY"},
	"xai":        {baseURL: "https://api.x.ai/v1", keyEnv: "XAI_API_KEY"},
	"deepseek":   {baseURL: "https://api.deepseek.com", keyEnv: "DEEPSEEK_API_KEY"},
	"mistral":    {baseURL: "https://api.mistral.ai/v1", keyEnv: "MISTRAL_API_KEY"},
	"cerebras":   {baseURL: "https://api.cerebras.ai/v1", keyEnv: "CEREBRAS_API_KEY"},
	"together":   {baseURL: "https://api.together.ai/v1", keyEnv: "TOGETHER_API_KEY"},
	"fireworks":  {baseURL: "https://api.fireworks.ai/inference/v1", keyEnv: "FIREWORKS_API_KEY"},
	"openrouter": {baseURL: "https://openrouter.ai/api/v1", keyEnv: "OPENROUTER_API_KEY"},
	"groq":       {baseURL: "https://api.groq.com/openai/v1", keyEnv: "GROQ_API_KEY"},
	"ollama":     {baseURL: "http://127.0.0.1:11434/v1", keyEnv: ""},
}

func resolveEndpoint(model, baseURL, keyEnv string, getenv func(string) string) (Endpoint, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = strings.TrimSpace(getenv("HP_MODEL"))
	}
	if model == "" {
		return Endpoint{}, modelExampleUsage("model required: use e.g. %s")
	}
	if baseURL == "" && strings.HasPrefix(model, customPreset+"/") {
		baseURL, keyEnv = customFromEnv(keyEnv, getenv)
	}
	ep, err := splitModel(model, baseURL, keyEnv)
	if err != nil {
		return Endpoint{}, err
	}
	return applyKey(ep, getenv)
}

func customFromEnv(keyEnv string, getenv func(string) string) (string, string) {
	if keyEnv == "" && getenv(customKeyEnv) != "" {
		keyEnv = customKeyEnv
	}
	return strings.TrimSpace(getenv(customBaseEnv)), keyEnv
}

func splitModel(model, baseURL, keyEnv string) (Endpoint, error) {
	preset, rest := splitFirstSlash(model)
	if def, ok := presets[preset]; ok {
		ep := mergePreset(preset, rest, def, baseURL, keyEnv)
		if err := rejectEmptyModel(preset, ep.Model); err != nil {
			return Endpoint{}, err
		}
		return ep, nil
	}
	if baseURL != "" {
		ep := Endpoint{Preset: customPreset, Model: strings.TrimPrefix(model, customPreset+"/"), BaseURL: baseURL, KeyEnv: keyEnv}
		if err := rejectEmptyModel(customPreset, ep.Model); err != nil {
			return Endpoint{}, err
		}
		return ep, nil
	}
	return Endpoint{}, unknownPreset(preset)
}

func rejectEmptyModel(preset, model string) error {
	if strings.TrimSpace(model) != "" {
		return nil
	}
	return usagef(`model name missing after %q: use e.g. %s`, preset+"/", modelExample)
}

func modelExampleUsage(format string) error {
	return usagef(format, modelExample)
}

func splitFirstSlash(s string) (string, string) {
	i := strings.Index(s, "/")
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

func mergePreset(preset, model string, def presetDef, baseURL, keyEnv string) Endpoint {
	ep := Endpoint{Preset: preset, Model: model, BaseURL: def.baseURL, KeyEnv: def.keyEnv}
	if baseURL != "" {
		ep.BaseURL = baseURL
	}
	if keyEnv != "" {
		ep.KeyEnv = keyEnv
	}
	return ep
}

func unknownPreset(name string) error {
	names := presetNames()
	return usagef("unknown preset %q (known: %s)", name, strings.Join(names, ", "))
}

func presetNames() []string {
	out := make([]string, 0, len(presets))
	for k := range presets {
		out = append(out, k)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func applyKey(ep Endpoint, getenv func(string) string) (Endpoint, error) {
	if ep.KeyEnv == "" {
		return ep, nil
	}
	v := getenv(ep.KeyEnv)
	if v == "" {
		return Endpoint{}, usagef("%s is not set", ep.KeyEnv)
	}
	ep.APIKey = v
	return ep, nil
}

func presetEndpoint(cur Endpoint, preset string, getenv func(string) string) (Endpoint, error) {
	if preset == customPreset && cur.Preset != customPreset {
		baseURL, keyEnv := customFromEnv("", getenv)
		if baseURL == "" {
			return Endpoint{}, usagef("%s is not set", customBaseEnv)
		}
		return applyKey(Endpoint{Preset: customPreset, BaseURL: baseURL, KeyEnv: keyEnv}, getenv)
	}
	if preset == customPreset {
		return applyKey(Endpoint{Preset: customPreset, BaseURL: cur.BaseURL, KeyEnv: cur.KeyEnv}, getenv)
	}
	def, ok := presets[preset]
	if !ok {
		return Endpoint{}, unknownPreset(preset)
	}
	ep := Endpoint{Preset: preset, BaseURL: def.baseURL, KeyEnv: def.keyEnv}
	if preset == cur.Preset {
		ep.BaseURL = cur.BaseURL
		ep.KeyEnv = cur.KeyEnv
	}
	return applyKey(ep, getenv)
}
