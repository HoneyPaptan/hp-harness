package cli

import (
	"path"
	"path/filepath"

	"github.com/HoneyPaptan/hp-harness/internal/prompt"
	"github.com/HoneyPaptan/hp-harness/internal/skill"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func skillDirs(dir, home string) []string {
	return []string{
		path.Join(dir, ".hp", "skills"),
		path.Join(dir, ".claude", "skills"),
		path.Join(home, ".config", "hp", "skills"),
	}
}

func discoverSkills(env Env, dir string) ([]skill.Skill, error) {
	return skill.Discover(env.Host, skillDirs(dir, env.Getenv("HOME")))
}

func buildRegistry(skills []skill.Skill) *tool.Registry {
	b := tool.Builtins()
	if len(skills) == 0 {
		return tool.NewRegistry(b...)
	}
	all := append(append([]tool.Tool(nil), b...), skill.NewTool(skills))
	return tool.NewRegistry(all...)
}

func buildSystem(env Env, dir string, skills []skill.Skill) string {
	return prompt.Build(prompt.Info{Dir: dir, Date: env.Host.Now(), Skills: skills})
}

func resolveWorkDir(env Env, flagDir string) (string, error) {
	if flagDir == "" {
		return env.Cwd, nil
	}
	if filepath.IsAbs(flagDir) {
		return flagDir, nil
	}
	return filepath.Join(env.Cwd, flagDir), nil
}
