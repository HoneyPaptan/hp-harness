package crew

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func git(env Env, dir string, args ...string) (string, error) {
	if env.Git != nil {
		return env.Git(dir, args...)
	}
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(out.String()))
	}
	return out.String(), nil
}

func gitRootHead(env Env, dir string) (string, string, error) {
	root, err := git(env, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", errNoRepo
	}
	head, err := git(env, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", "", errNoCommit
	}
	return strings.TrimSpace(root), strings.TrimSpace(head), nil
}

func ensureWorktree(env Env, st *State, t *Task) error {
	if _, err := os.Stat(filepath.Join(t.Worktree, ".git")); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(t.Worktree), 0o755); err != nil {
		return err
	}
	_, err := git(env, st.Repo, "worktree", "add", "-b", t.Branch, t.Worktree, st.Base)
	return err
}

func changedFiles(env Env, st *State, t *Task) string {
	if t.Status == StatusQueued {
		return "-"
	}
	out, err := git(env, t.Worktree, "status", "--porcelain")
	if err != nil {
		return "?"
	}
	n := countLines(out)
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

func diffStat(env Env, st *State, t *Task) string {
	var parts []string
	if out, err := git(env, t.Worktree, "diff", "--stat", st.Base); err == nil {
		if s := strings.TrimSpace(out); s != "" {
			parts = append(parts, s)
		}
	}
	if out, err := git(env, t.Worktree, "status", "--porcelain"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if strings.HasPrefix(line, "??") {
				parts = append(parts, strings.TrimSpace(line))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func countLines(s string) int {
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if line != "" {
			n++
		}
	}
	return n
}
