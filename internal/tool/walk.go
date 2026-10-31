package tool

import (
	"path/filepath"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func Walk(env Env, root string, visit func(path string, entry host.Entry)) error {
	ent, err := env.Host.Stat(root)
	if err != nil {
		return err
	}
	if !ent.IsDir {
		visit(root, ent)
		return nil
	}
	return walkDir(env, root, visit)
}

func walkDir(env Env, dir string, visit func(string, host.Entry)) error {
	ents, err := env.Host.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		p := filepath.Join(dir, e.Name)
		if e.IsDir {
			if e.Name == ".git" || e.IsLink {
				continue
			}
			if err := walkDir(env, p, visit); err != nil {
				return err
			}
			continue
		}
		visit(p, e)
	}
	return nil
}

func ReadText(env Env, path string) ([]byte, bool, error) {
	ent, err := env.Host.Stat(path)
	if err != nil {
		return nil, false, err
	}
	if ent.Size > 1<<20 {
		return nil, false, nil
	}
	data, err := env.Host.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	if isBinary(data) {
		return nil, false, nil
	}
	return data, true, nil
}

func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return strings.IndexByte(string(data[:n]), 0) >= 0
}

func DisplayPath(workDir, p string) string {
	rel, err := filepath.Rel(workDir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}
