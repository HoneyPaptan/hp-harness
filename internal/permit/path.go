package permit

import (
	"path/filepath"
	"strings"
)

func resolvePath(dir, p string) string {
	if p == "" {
		p = "."
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(dir, p))
}

func displayPath(dir, p string) string {
	abs := resolvePath(dir, p)
	rel, err := filepath.Rel(dir, abs)
	if err != nil || pathEscapes(rel) {
		return abs
	}
	return rel
}

func pathEscapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func insideDir(path, dir string) bool {
	dir = filepath.Clean(dir)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return !pathEscapes(rel)
}
