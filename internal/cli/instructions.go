package cli

import (
	"path/filepath"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/prompt"
)

const (
	agentsFileName             = "AGENTS.md"
	instructionFileMaxRunes    = 32 * 1024
	instructionTotalMaxBytes   = 64 * 1024
	instructionSkipMarker      = "[skipped: instruction limit]"
	instructionTruncatedSuffix = "\n[truncated at 32 KB]"
)

func contextEnabled(st settings) bool {
	if st.Context != nil && !*st.Context {
		return false
	}
	return true
}

func loadInstructions(h host.Host, dir, home string) []prompt.Instruction {
	var out []prompt.Instruction
	total := 0
	try := func(abs string) {
		if total >= instructionTotalMaxBytes {
			out = append(out, prompt.Instruction{
				Path: instructionDisplayPath(abs, home),
				Text: instructionSkipMarker,
			})
			return
		}
		data, ok := readAgentsFile(h, abs)
		if !ok {
			return
		}
		text := truncateInstructionRunes(string(data), instructionFileMaxRunes)
		size := len(text)
		if total+size > instructionTotalMaxBytes {
			out = append(out, prompt.Instruction{
				Path: instructionDisplayPath(abs, home),
				Text: instructionSkipMarker,
			})
			total = instructionTotalMaxBytes
			return
		}
		total += size
		out = append(out, prompt.Instruction{
			Path: instructionDisplayPath(abs, home),
			Text: text,
		})
	}
	if p, err := personalAgentsPath(home); err == nil {
		try(p)
	}
	for _, d := range agentsWalkDirs(dir, home) {
		try(filepath.Join(d, agentsFileName))
	}
	return out
}

func personalAgentsPath(home string) (string, error) {
	dir, err := userConfigDir(func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, agentsFileName), nil
}

func agentsWalkDirs(dir, home string) []string {
	dir = filepath.Clean(dir)
	home = filepath.Clean(home)
	if !dirStrictlyUnderHome(dir, home) {
		if dir == home {
			return nil
		}
		return []string{dir}
	}
	rel, err := filepath.Rel(home, dir)
	if err != nil {
		return []string{dir}
	}
	parts := pathRelParts(rel)
	if len(parts) == 0 {
		return nil
	}
	var dirs []string
	cur := home
	for _, p := range parts {
		cur = filepath.Join(cur, p)
		dirs = append(dirs, cur)
	}
	return dirs
}

func dirStrictlyUnderHome(dir, home string) bool {
	dir = filepath.Clean(dir)
	home = filepath.Clean(home)
	if dir == home {
		return false
	}
	rel, err := filepath.Rel(home, dir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func pathRelParts(rel string) []string {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." {
		return nil
	}
	var out []string
	for _, p := range strings.Split(rel, "/") {
		if p != "" && p != "." {
			out = append(out, p)
		}
	}
	return out
}

func instructionDisplayPath(abs, home string) string {
	home = filepath.Clean(home)
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(home, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(abs)
	}
	return "~/" + filepath.ToSlash(rel)
}

func readAgentsFile(h host.Host, path string) ([]byte, bool) {
	ent, err := h.Lstat(path)
	if err != nil {
		return nil, false
	}
	if ent.IsLink || ent.IsDir {
		return nil, false
	}
	data, err := h.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func truncateInstructionRunes(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + instructionTruncatedSuffix
}
