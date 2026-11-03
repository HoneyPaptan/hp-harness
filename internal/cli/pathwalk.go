package cli

import (
	"path"
	"path/filepath"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func pathHasSymlink(h host.Host, dir, rel string) bool {
	parts, ok := relPathParts(rel)
	if !ok {
		return false
	}
	cur := dir
	for _, p := range parts {
		cur = filepath.Join(cur, p)
		ent, err := h.Lstat(cur)
		if err != nil {
			return false
		}
		if ent.IsLink {
			return true
		}
	}
	return false
}

func relPathParts(rel string) ([]string, bool) {
	if rel == "" || filepath.IsAbs(rel) {
		return nil, false
	}
	clean := filepath.ToSlash(path.Clean(rel))
	if mentionBadPath(clean) {
		return nil, false
	}
	parts := mentionParts(clean)
	if len(parts) == 0 {
		return nil, false
	}
	return parts, true
}
