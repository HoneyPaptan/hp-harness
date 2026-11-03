package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

type nestedContext struct {
	enabled  bool
	h        host.Host
	workDir  string
	attached map[string]struct{}
}

func newNestedContext(enabled bool, h host.Host, workDir string, attached map[string]struct{}) *nestedContext {
	if attached == nil {
		attached = make(map[string]struct{})
	}
	return &nestedContext{enabled: enabled, h: h, workDir: workDir, attached: attached}
}

func attachedInstructionPaths(msgs []provider.Message) map[string]struct{} {
	attached := make(map[string]struct{})
	for _, m := range msgs {
		for _, tr := range m.ToolResults {
			collectAttachedPaths(tr.Text, attached)
		}
	}
	return attached
}

func collectAttachedPaths(text string, attached map[string]struct{}) {
	rest := text
	for {
		const prefix = "<instructions path=\""
		i := strings.Index(rest, prefix)
		if i < 0 {
			return
		}
		rest = rest[i+len(prefix):]
		j := strings.Index(rest, "\">")
		if j < 0 {
			return
		}
		attached[rest[:j]] = struct{}{}
		rest = rest[j:]
	}
}

func wrapNestedTools(tools []tool.Tool, n *nestedContext) []tool.Tool {
	if n == nil || !n.enabled {
		return tools
	}
	out := make([]tool.Tool, len(tools))
	for i, t := range tools {
		out[i] = wrapNestedTool(t, n)
	}
	return out
}

func wrapNestedTool(t tool.Tool, n *nestedContext) tool.Tool {
	switch t.Name {
	case "read", "edit", "write":
	default:
		return t
	}
	inner := t.Run
	t.Run = func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
		res, err := inner(ctx, env, raw)
		if err != nil || res.IsError {
			return res, err
		}
		rel, ok := nestedToolPath(raw)
		if !ok {
			return res, err
		}
		if extra := n.attachFor(env.Dir, rel); extra != "" {
			res.Text += extra
		}
		return res, err
	}
	return t
}

func nestedToolPath(raw json.RawMessage) (string, bool) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || args.Path == "" {
		return "", false
	}
	return args.Path, true
}

func (n *nestedContext) attachFor(workDir, rel string) string {
	abs, ok := nestedResolvePath(n.h, workDir, rel)
	if !ok {
		return ""
	}
	for _, dir := range nestedSearchDirs(abs, workDir) {
		agentsPath := filepath.Join(dir, agentsFileName)
		display, err := filepath.Rel(workDir, agentsPath)
		if err != nil {
			continue
		}
		display = filepath.ToSlash(display)
		if _, seen := n.attached[display]; seen {
			continue
		}
		data, ok := readAgentsFile(n.h, agentsPath)
		if !ok {
			continue
		}
		n.attached[display] = struct{}{}
		body := truncateInstructionRunes(string(data), instructionFileMaxRunes)
		return formatNestedInstruction(display, body)
	}
	return ""
}

func formatNestedInstruction(displayPath, body string) string {
	return "\n\n<instructions path=\"" + displayPath + "\">\n" + body + "\n</instructions>"
}

func nestedResolvePath(h host.Host, workDir, rel string) (string, bool) {
	parts, ok := relPathParts(rel)
	if !ok {
		if filepath.IsAbs(rel) {
			abs := filepath.Clean(rel)
			if pathWithinWork(abs, workDir) {
				return abs, true
			}
		}
		return "", false
	}
	cur := workDir
	for i, p := range parts {
		cur = filepath.Join(cur, p)
		ent, err := h.Lstat(cur)
		if err != nil {
			return "", false
		}
		if ent.IsLink {
			return "", false
		}
		if ent.IsDir {
			if i == len(parts)-1 {
				return "", false
			}
			continue
		}
		if i != len(parts)-1 {
			return "", false
		}
		return cur, true
	}
	return "", false
}

func pathWithinWork(abs, workDir string) bool {
	abs = filepath.Clean(abs)
	workDir = filepath.Clean(workDir)
	rel, err := filepath.Rel(workDir, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func nestedSearchDirs(fileAbs, workDir string) []string {
	fileAbs = filepath.Clean(fileAbs)
	workDir = filepath.Clean(workDir)
	if !pathWithinWork(fileAbs, workDir) {
		return nil
	}
	dir := filepath.Dir(fileAbs)
	var dirs []string
	for {
		if dir == workDir {
			break
		}
		if !pathWithinWork(dir, workDir) {
			break
		}
		dirs = append(dirs, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dirs
}
