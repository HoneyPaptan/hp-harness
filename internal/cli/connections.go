package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	connectionsFile = "connections.json"
	schemaCacheFile = "connections-cache.json"
	maxNameLen      = 32
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]*[a-zA-Z0-9-])?$`)

type connection struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Dir     string            `json:"dir,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type connectionSet struct {
	Servers map[string]connection `json:"servers"`
}

type cachedTool struct {
	Name        string          `json:"name"`
	Remote      string          `json:"remote"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

type schemaCache struct {
	Servers map[string][]cachedTool `json:"servers"`
}

func checkName(name string) error {
	if len(name) > maxNameLen || strings.Contains(name, "__") || !namePattern.MatchString(name) {
		return usagef("invalid name %q: use letters, digits, - and _, at most %d characters", name, maxNameLen)
	}
	return nil
}

func userConfigDir(getenv func(string) string) (string, error) {
	home := getenv("HOME")
	if !path.IsAbs(home) {
		return "", errors.New("HOME must be an absolute path to use connections")
	}
	return path.Join(home, ".config", "hp"), nil
}

func userSkillsDir(dir string) string {
	return path.Join(dir, "skills")
}

func loadConnections(env Env, dir string) (connectionSet, error) {
	set := connectionSet{Servers: map[string]connection{}}
	if err := readJSON(env, path.Join(dir, connectionsFile), &set); err != nil {
		return connectionSet{}, err
	}
	if set.Servers == nil {
		set.Servers = map[string]connection{}
	}
	return set, nil
}

func loadSchemaCache(env Env, dir string) (schemaCache, error) {
	cache := schemaCache{Servers: map[string][]cachedTool{}}
	if err := readJSON(env, path.Join(dir, schemaCacheFile), &cache); err != nil {
		return schemaCache{}, err
	}
	if cache.Servers == nil {
		cache.Servers = map[string][]cachedTool{}
	}
	return cache, nil
}

func readJSON(env Env, file string, into any) error {
	data, err := env.Host.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return nil
}

func saveConnections(dir string, set connectionSet) error {
	return writePrivateJSON(filepath.Join(dir, connectionsFile), set)
}

func saveSchemaCache(dir string, cache schemaCache) error {
	return writePrivateJSON(filepath.Join(dir, schemaCacheFile), cache)
}

func writePrivateJSON(file string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(file, append(data, '\n'))
}

func writePrivate(file string, data []byte) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := fillPrivate(tmp, data); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

func fillPrivate(f *os.File, data []byte) error {
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
