package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed static
var staticFS embed.FS

type staticFile struct {
	data []byte
	etag string
}

func loadStatic() (map[string]staticFile, error) {
	root, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	files := make(map[string]staticFile)
	err = fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		files[p] = newStaticFile(data)
		return nil
	})
	return files, err
}

func newStaticFile(data []byte) staticFile {
	sum := sha256.Sum256(data)
	return staticFile{data: data, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	f, ok := s.static[name]
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("ETag", f.etag)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(f.data))
}
