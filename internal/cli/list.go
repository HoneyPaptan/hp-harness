package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/HoneyPaptan/hp-harness/internal/skill"
)

func cmdList(env Env, args []string) int {
	if len(args) != 0 {
		return exitUsage(env, usagef("list takes no arguments"))
	}
	if err := printConnections(env); err != nil {
		return exitErr(env, "", "", err)
	}
	return 0
}

func printConnections(env Env) error {
	dir, err := userConfigDir(env.Getenv)
	if err != nil {
		return err
	}
	set, err := loadConnections(env, dir)
	if err != nil {
		return err
	}
	cache, err := loadSchemaCache(env, dir)
	if err != nil {
		return err
	}
	skills, err := userSkills(env, dir)
	if err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(set.Servers)) {
		c := set.Servers[name]
		fmt.Fprintf(env.Stdout, "%-20s %-30s %s\n", name, c.where(), toolCount(len(cache.Servers[name])))
	}
	for _, sk := range skills {
		fmt.Fprintf(env.Stdout, "%-20s skill\n", sk.Name)
	}
	return nil
}

func (c connection) where() string {
	if c.URL == "" {
		return "stdio " + path.Base(c.Command)
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return "http"
	}
	return "http " + u.Host + u.Path
}

func userSkills(env Env, dir string) ([]skill.Skill, error) {
	return skill.Discover(env.Host, []string{userSkillsDir(dir)})
}

func nameTaken(env Env, dir string, set connectionSet, name string) (bool, error) {
	if _, ok := set.Servers[name]; ok {
		return true, nil
	}
	skills, err := userSkills(env, dir)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(skills, func(s skill.Skill) bool { return s.Name == name }) {
		return true, nil
	}
	_, err = os.Lstat(filepath.Join(userSkillsDir(dir), name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func cmdRemove(env Env, args []string) int {
	if len(args) != 1 {
		return exitUsage(env, usagef("usage: hp remove <name>"))
	}
	if err := removeNamed(env, args[0]); err != nil {
		return exitErr(env, "", "", err)
	}
	fmt.Fprintf(env.Stdout, "removed %s\n", args[0])
	return 0
}

func removeNamed(env Env, name string) error {
	dir, err := userConfigDir(env.Getenv)
	if err != nil {
		return err
	}
	set, err := loadConnections(env, dir)
	if err != nil {
		return err
	}
	if _, ok := set.Servers[name]; ok {
		return removeServer(env, dir, set, name)
	}
	return removeSkill(env, dir, name)
}

func removeServer(env Env, dir string, set connectionSet, name string) error {
	delete(set.Servers, name)
	if err := saveConnections(dir, set); err != nil {
		return err
	}
	cache, err := loadSchemaCache(env, dir)
	if err != nil {
		return err
	}
	delete(cache.Servers, name)
	return saveSchemaCache(dir, cache)
}

func removeSkill(env Env, dir, name string) error {
	skills, err := userSkills(env, dir)
	if err != nil {
		return err
	}
	for _, sk := range skills {
		if sk.Name == name {
			return os.RemoveAll(sk.Dir)
		}
	}
	return fmt.Errorf("nothing named %s", name)
}
