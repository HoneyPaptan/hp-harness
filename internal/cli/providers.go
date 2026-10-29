package cli

import (
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func endpointSpec(ep Endpoint) string {
	return ep.Preset + "/" + ep.Model
}

func resolveRunSpecs(o runOpts, getenv func(string) string) (Endpoint, map[string]Endpoint, error) {
	primary, err := resolveEndpoint(o.model, o.baseURL, o.keyEnv, getenv)
	if err != nil {
		return Endpoint{}, nil, err
	}
	return finishSpecMap(primary, o.fallbacks, getenv)
}

func resolveResumeSpecs(st journal.RunStart, getenv func(string) string) (Endpoint, map[string]Endpoint, error) {
	primary, err := endpointFromStart(st, getenv)
	if err != nil {
		return Endpoint{}, nil, err
	}
	return finishSpecMap(primary, st.Fallbacks, getenv)
}

func finishSpecMap(primary Endpoint, fallbacks []string, getenv func(string) string) (Endpoint, map[string]Endpoint, error) {
	bySpec := map[string]Endpoint{endpointSpec(primary): primary}
	for _, spec := range fallbacks {
		if _, ok := bySpec[spec]; ok {
			continue
		}
		ep, err := resolveEndpoint(spec, "", "", getenv)
		if err != nil {
			return Endpoint{}, nil, err
		}
		bySpec[spec] = ep
	}
	return primary, bySpec, nil
}

func providerFactory(env Env, bySpec map[string]Endpoint) func(string) (provider.Provider, string, error) {
	return func(spec string) (provider.Provider, string, error) {
		ep, ok := bySpec[spec]
		if !ok {
			return nil, "", usagef("unknown model %q", spec)
		}
		return env.NewProvider(ep), ep.Model, nil
	}
}

func limitProviders(env Env, bySpec map[string]Endpoint) func(string) (provider.Provider, string, error) {
	if len(bySpec) <= 1 {
		return nil
	}
	return providerFactory(env, bySpec)
}
