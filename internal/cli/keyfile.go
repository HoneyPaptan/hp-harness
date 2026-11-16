package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
)

const keyFileName = "env"

func validKeyName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

type keyFile struct {
	values map[string]string
	names  []string
}

func keyFilePath(getenv func(string) string) (string, error) {
	dir, err := userConfigDir(getenv)
	if err != nil {
		return "", err
	}
	return path.Join(dir, keyFileName), nil
}

func readKeyFile(file string) (keyFile, fs.FileMode, error) {
	info, err := os.Stat(file)
	if errors.Is(err, fs.ErrNotExist) {
		return parseKeyFile(nil), 0, nil
	}
	if err != nil {
		return keyFile{}, 0, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return keyFile{}, 0, err
	}
	return parseKeyFile(data), info.Mode().Perm(), nil
}

func parseKeyFile(data []byte) keyFile {
	kf := keyFile{values: map[string]string{}}
	for _, line := range splitKeyLines(data) {
		name, value, ok := parseKeyLine(line)
		if !ok || value == "" {
			continue
		}
		if _, seen := kf.values[name]; !seen {
			kf.names = append(kf.names, name)
		}
		kf.values[name] = value
	}
	return kf
}

func parseKeyLine(line string) (string, string, bool) {
	name, value, ok := strings.Cut(line, "=")
	if !ok || !validKeyName(name) {
		return "", "", false
	}
	return name, value, true
}

func splitKeyLines(data []byte) []string {
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func keyLines(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return splitKeyLines(data), nil
}

func writeKeyLines(file string, lines []string) error {
	if len(lines) == 0 {
		return writePrivate(file, nil)
	}
	return writePrivate(file, []byte(strings.Join(lines, "\n")+"\n"))
}

func replaceKeyLine(lines []string, name, entry string) ([]string, bool) {
	out := make([]string, 0, len(lines)+1)
	found := false
	for _, line := range lines {
		lineName, _, ok := parseKeyLine(line)
		if ok && lineName == name {
			if entry != "" && !found {
				out = append(out, entry)
			}
			found = true
			continue
		}
		out = append(out, line)
	}
	if entry != "" && !found {
		out = append(out, entry)
	}
	return out, found
}

func keySet(file, name, value string) error {
	lines, err := keyLines(file)
	if err != nil {
		return err
	}
	out, _ := replaceKeyLine(lines, name, name+"="+value)
	return writeKeyLines(file, out)
}

func keyRemove(file, name string) (bool, error) {
	lines, err := keyLines(file)
	if err != nil {
		return false, err
	}
	out, found := replaceKeyLine(lines, name, "")
	if !found {
		return false, nil
	}
	return true, writeKeyLines(file, out)
}

func keyListNames(kf keyFile, getenv func(string) string) []string {
	seen := map[string]struct{}{}
	for _, name := range kf.names {
		seen[name] = struct{}{}
	}
	for _, def := range presets {
		if def.keyEnv != "" && getenv(def.keyEnv) != "" {
			seen[def.keyEnv] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func reloadKeyFile(env Env) Env {
	env.Getenv = rawGetenv(env)
	return applyKeyFile(env)
}

func applyKeyFile(env Env) Env {
	real := env.Getenv
	if real == nil {
		return env
	}
	file, err := keyFilePath(real)
	if err != nil {
		return env
	}
	kf, mode, err := readKeyFile(file)
	if err != nil {
		fmt.Fprintf(env.Stderr, "note: cannot read %s: %v\n", file, err)
		return env
	}
	if mode != 0 && mode&^fs.FileMode(0o600) != 0 {
		fmt.Fprintf(env.Stderr, "note: %s has mode %04o, wider than 0600; run chmod 600 %s\n", file, mode, file)
	}
	env.realGetenv = real
	env.Getenv = mergedGetenv(real, kf.values)
	return env
}

func mergedGetenv(real func(string) string, keys map[string]string) func(string) string {
	return func(name string) string {
		if value := real(name); value != "" {
			return value
		}
		return keys[name]
	}
}

func rawGetenv(env Env) func(string) string {
	if env.realGetenv != nil {
		return env.realGetenv
	}
	if env.Getenv != nil {
		return env.Getenv
	}
	return func(string) string { return "" }
}
