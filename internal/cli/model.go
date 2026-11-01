package cli

import "strings"

type Endpoint struct {
	Preset, Model, BaseURL, KeyEnv, APIKey string
}

const customPreset = "custom"

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
		return Endpoint{}, usagef("model required: use -m preset/model")
	}
	ep, err := splitModel(model, baseURL, keyEnv)
	if err != nil {
		return Endpoint{}, err
	}
	return applyKey(ep, getenv)
}

func splitModel(model, baseURL, keyEnv string) (Endpoint, error) {
	preset, rest := splitFirstSlash(model)
	if def, ok := presets[preset]; ok {
		return mergePreset(preset, rest, def, baseURL, keyEnv), nil
	}
	if baseURL != "" {
		return Endpoint{Preset: customPreset, Model: strings.TrimPrefix(model, customPreset+"/"), BaseURL: baseURL, KeyEnv: keyEnv}, nil
	}
	return Endpoint{}, unknownPreset(preset)
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
