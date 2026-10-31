package jev

import (
	"path/filepath"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

const privateKeyMarker = "PRIVATE KEY-----"

var secretNames = []string{
	".env",
	".env.*",
	"*.pem",
	"*.key",
	"id_rsa*",
	"id_ed25519*",
	"id_ecdsa*",
	"*credentials*",
	".netrc",
	".npmrc",
}

func isSecretName(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	for _, pattern := range secretNames {
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
	}
	return false
}

func readShareable(env tool.Env, path string) (string, bool) {
	data, ok, err := tool.ReadText(env, path)
	if err != nil || !ok {
		return "", false
	}
	text := string(data)
	return text, !strings.Contains(text, privateKeyMarker)
}

func reachedThroughLink(env tool.Env, root string) bool {
	stop := filepath.Clean(env.Dir)
	for p := root; p != stop && p != filepath.Dir(p); p = filepath.Dir(p) {
		if entryIsLink(env, p) {
			return true
		}
	}
	return false
}

func entryIsLink(env tool.Env, path string) bool {
	ents, err := env.Host.ReadDir(filepath.Dir(path))
	if err != nil {
		return true
	}
	for _, e := range ents {
		if e.Name == filepath.Base(path) {
			return e.IsLink
		}
	}
	return true
}
