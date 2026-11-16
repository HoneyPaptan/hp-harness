package cli

import (
	"path"
	"strings"
)

const (
	lastModelFile   = "last-model"
	lastModelMaxLen = 256
)

func lastModelPath(getenv func(string) string) string {
	file := path.Join(path.Dir(runsDir(getenv)), lastModelFile)
	if !path.IsAbs(file) {
		return ""
	}
	return file
}

func loadLastModel(env Env) string {
	file := lastModelPath(env.Getenv)
	if file == "" || env.Browser {
		return ""
	}
	data, err := env.Host.ReadFile(file)
	if err != nil {
		return ""
	}
	spec, _, _ := strings.Cut(string(data), "\n")
	spec = strings.TrimSpace(spec)
	if len(spec) > lastModelMaxLen || !strings.Contains(spec, "/") {
		return ""
	}
	return spec
}

func saveLastModel(env Env, spec string) error {
	file := lastModelPath(env.Getenv)
	if file == "" || env.Browser {
		return nil
	}
	return env.Host.WriteFile(file, []byte(spec+"\n"))
}

func applyLastModel(env Env, o runOpts) (runOpts, string) {
	if o.model != "" || o.baseURL != "" || o.keyEnv != "" {
		return o, ""
	}
	saved := loadLastModel(env)
	if saved == "" {
		return o, ""
	}
	if _, err := resolveEndpoint(saved, "", "", env.Getenv); err != nil {
		return o, "last model " + saved + " not used: " + firstLine(err.Error())
	}
	o.model = saved
	return o, ""
}
