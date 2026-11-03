package permit

import (
	"path/filepath"
	"strings"
)

func Sensitive(path, dir, home string) bool {
	path = filepath.Clean(path)
	dir = filepath.Clean(dir)
	home = filepath.Clean(home)
	if !insideDir(path, dir) {
		return true
	}
	base := filepath.Base(path)
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	if underGit(dir, path) {
		return true
	}
	if underHomeDir(home, path, ".ssh") {
		return true
	}
	if underHomeDir(home, path, ".config", "hp") {
		return true
	}
	if underHomeDir(home, path, ".gnupg") {
		return true
	}
	return shellRC(home, path)
}

func underGit(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil || pathEscapes(rel) {
		return false
	}
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if seg == ".git" {
			return true
		}
	}
	return false
}

func underHomeDir(home, path string, elems ...string) bool {
	root := filepath.Join(append([]string{home}, elems...)...)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	prefix := root + string(filepath.Separator)
	return strings.HasPrefix(path, prefix)
}

func shellRC(home, path string) bool {
	candidates := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".config", "fish", "config.fish"),
	}
	for _, c := range candidates {
		if path == filepath.Clean(c) {
			return true
		}
	}
	return false
}
