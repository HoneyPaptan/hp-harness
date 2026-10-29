package tool

import (
	"path/filepath"
)

func Resolve(dir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(dir, p))
}
