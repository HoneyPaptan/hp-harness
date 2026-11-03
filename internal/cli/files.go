package cli

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

const maxListedFiles = 200

func listFiles(h host.Host, root string) ([]string, error) {
	patterns, _ := readGitignore(h, filepath.Join(root, ".gitignore"))
	type node struct {
		rel string
		abs string
	}
	queue := []node{{"", root}}
	var out []string
	for len(queue) > 0 && len(out) < maxListedFiles {
		cur := queue[0]
		queue = queue[1:]
		ents, err := h.ReadDir(cur.abs)
		if err != nil {
			return nil, err
		}
		for _, e := range ents {
			if strings.HasPrefix(e.Name, ".") {
				continue
			}
			if e.Name == "node_modules" || e.Name == "vendor" {
				continue
			}
			if e.IsLink {
				continue
			}
			rel := e.Name
			if cur.rel != "" {
				rel = cur.rel + "/" + e.Name
			}
			if gitignoreSkip(patterns, e.Name, e.IsDir, cur.rel == "") {
				continue
			}
			if e.IsDir {
				queue = append(queue, node{rel, filepath.Join(cur.abs, e.Name)})
				continue
			}
			out = append(out, filepath.ToSlash(rel))
			if len(out) >= maxListedFiles {
				break
			}
		}
	}
	slices.Sort(out)
	return out, nil
}

func readGitignore(h host.Host, path string) ([]string, error) {
	data, err := h.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var patterns []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, nil
}

func gitignoreSkip(patterns []string, base string, isDir, atRoot bool) bool {
	for _, pat := range patterns {
		rootOnly := strings.HasPrefix(pat, "/")
		if rootOnly {
			if !atRoot {
				continue
			}
			pat = pat[1:]
		}
		dirOnly := strings.HasSuffix(pat, "/")
		if dirOnly {
			if !isDir {
				continue
			}
			pat = strings.TrimSuffix(pat, "/")
		}
		if pat == base {
			return true
		}
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
	}
	return false
}
