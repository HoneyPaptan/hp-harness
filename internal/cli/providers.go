package cli

import (
	"fmt"
	"sync"

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

func bindCurrent(env Env, jpath string, primary Endpoint, bySpec map[string]Endpoint) error {
	st, err := replayRun(env, jpath)
	if err != nil || st.Current == "" {
		return nil
	}
	if _, ok := bySpec[st.Current]; ok {
		return nil
	}
	ep, err := endpointFor(primary, st.Current, env.Getenv)
	if err != nil {
		return err
	}
	bySpec[st.Current] = ep
	return nil
}

func endpointFor(cur Endpoint, spec string, getenv func(string) string) (Endpoint, error) {
	baseURL, keyEnv := cur.BaseURL, cur.KeyEnv
	preset, _ := splitFirstSlash(spec)
	if _, known := presets[preset]; known && preset != cur.Preset {
		baseURL, keyEnv = "", ""
	}
	ep, err := resolveEndpoint(spec, baseURL, keyEnv, getenv)
	if err != nil {
		return Endpoint{}, err
	}
	if ep.Model == "" {
		return Endpoint{}, usagef("model must be preset/name")
	}
	return ep, nil
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

func SharedProviders(build func(Endpoint) provider.Provider) func(Endpoint) provider.Provider {
	var mu sync.Mutex
	made := make(map[Endpoint]provider.Provider)
	return func(ep Endpoint) provider.Provider {
		mu.Lock()
		defer mu.Unlock()
		p, ok := made[ep]
		if !ok {
			p = build(ep)
			made[ep] = p
		}
		return p
	}
}

func cmdProviders(env Env, args []string) int {
	if len(args) != 0 {
		return exitUsage(env, usagef("unexpected arguments"))
	}
	names := presetNames()
	nameWidth, urlWidth := presetWidths(names)
	for _, name := range names {
		def := presets[name]
		fmt.Fprintf(env.Stdout, "%-*s  %-*s  %s\n", nameWidth, name, urlWidth, def.baseURL, keyStatus(def.keyEnv, env.Getenv))
	}
	return 0
}

func presetWidths(names []string) (int, int) {
	var nameWidth, urlWidth int
	for _, name := range names {
		nameWidth = max(nameWidth, len(name))
		urlWidth = max(urlWidth, len(presets[name].baseURL))
	}
	return nameWidth, urlWidth
}

func keyStatus(keyEnv string, getenv func(string) string) string {
	switch {
	case keyEnv == "":
		return "no key needed"
	case getenv(keyEnv) == "":
		return keyEnv + " not set"
	default:
		return keyEnv + " set"
	}
}
