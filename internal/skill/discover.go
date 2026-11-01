package skill

import (
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func Discover(h host.Host, dirs []string) ([]Skill, error) {
	seen := make(map[string]struct{})
	var out []Skill
	for _, dir := range dirs {
		part, err := discoverDir(h, dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, sk := range part {
			if _, ok := seen[sk.Name]; ok {
				continue
			}
			seen[sk.Name] = struct{}{}
			out = append(out, sk)
		}
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func discoverDir(h host.Host, dir string) ([]Skill, error) {
	ents, err := h.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Skill
	for _, ent := range ents {
		if !ent.IsDir {
			continue
		}
		sk, ok, err := loadSkill(h, dir, ent.Name)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, sk)
		}
	}
	return out, nil
}

func loadSkill(h host.Host, dir, folder string) (Skill, bool, error) {
	skillPath := path.Join(dir, folder, "SKILL.md")
	data, err := h.ReadFile(skillPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Skill{}, false, nil
		}
		return Skill{}, false, err
	}
	name, desc, ok := identify(string(data), folder)
	if !ok {
		return Skill{}, false, nil
	}
	return Skill{
		Name: name, Description: desc,
		Dir: path.Join(dir, folder), Path: skillPath,
	}, true, nil
}

func Inspect(text, folder string) (string, bool) {
	name, _, ok := identify(text, folder)
	return name, ok
}

func identify(text, folder string) (string, string, bool) {
	fields, _, ok := parseFrontmatter(text)
	if !ok {
		return "", "", false
	}
	desc := normSpace(fields["description"])
	if desc == "" {
		return "", "", false
	}
	name := strings.TrimSpace(fields["name"])
	if name == "" {
		name = folder
	}
	return name, desc, true
}

func normSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
