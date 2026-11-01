package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/skill"
)

const (
	skillFile      = "SKILL.md"
	allowedSchemes = "https:http:ssh:git:file"
	maxGitOutput   = 2 << 10
)

var scpLikeURL = regexp.MustCompile(`^[\w.-]+@[\w.-]+:`)

func isGitURL(s string) bool {
	for _, prefix := range []string{"https://", "http://", "ssh://", "git://", "file://"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return scpLikeURL.MatchString(s)
}

func addSkill(ctx context.Context, env Env, source string) error {
	if isGitURL(source) {
		return addSkillFromGit(ctx, env, source)
	}
	dir := source
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(env.Cwd, dir)
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return usagef("%q is not a folder or git URL", source)
	}
	if !isRegularFile(filepath.Join(root, skillFile)) {
		return fmt.Errorf("no %s in %s", skillFile, source)
	}
	return installSkill(env, root, filepath.Base(root))
}

func addSkillFromGit(ctx context.Context, env Env, source string) error {
	tmp, err := os.MkdirTemp("", "hp-add-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	repo := filepath.Join(tmp, "repo")
	if err := cloneShallow(ctx, source, repo); err != nil {
		return err
	}
	root, err := findSkillRoot(repo)
	if err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}
	if root == repo {
		return installSkill(env, root, repoName(source))
	}
	return installSkill(env, root, filepath.Base(root))
}

func cloneShallow(ctx context.Context, source, dest string) error {
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null",
		"clone", "--quiet", "--depth", "1", "--single-branch", "--no-tags", "--", source, dest)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL="+allowedSchemes, "GIT_LFS_SKIP_SMUDGE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone: %w: %s", err, gitTail(out))
	}
	return nil
}

func gitTail(out []byte) string {
	return strings.TrimSpace(string(out[max(0, len(out)-maxGitOutput):]))
}

func repoName(source string) string {
	name := strings.TrimRight(source, "/")
	name = name[strings.LastIndexAny(name, "/:")+1:]
	return strings.TrimSuffix(name, ".git")
}

func findSkillRoot(repo string) (string, error) {
	if isRegularFile(filepath.Join(repo, skillFile)) {
		return repo, nil
	}
	ents, err := os.ReadDir(repo)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range ents {
		if e.IsDir() && isRegularFile(filepath.Join(repo, e.Name(), skillFile)) {
			found = append(found, e.Name())
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no %s at the root or one level down", skillFile)
	case 1:
		return filepath.Join(repo, found[0]), nil
	}
	return "", fmt.Errorf("more than one skill: %s", strings.Join(found, ", "))
}

func isRegularFile(file string) bool {
	info, err := os.Lstat(file)
	return err == nil && info.Mode().IsRegular()
}

func installSkill(env Env, src, folder string) error {
	name, err := skillName(src, folder)
	if err != nil {
		return err
	}
	dir, err := userConfigDir(env.Getenv)
	if err != nil {
		return err
	}
	set, err := loadConnections(env, dir)
	if err != nil {
		return err
	}
	if taken, err := nameTaken(env, dir, set, name); err != nil || taken {
		return alreadyAdded(name, err)
	}
	if err := copySkill(src, dir, name); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "added skill %s\n", name)
	return nil
}

func skillName(src, folder string) (string, error) {
	data, err := os.ReadFile(filepath.Join(src, skillFile))
	if err != nil {
		return "", err
	}
	name, ok := skill.Inspect(string(data), folder)
	if !ok {
		return "", fmt.Errorf("%s needs frontmatter with a description", skillFile)
	}
	return name, checkName(name)
}

func copySkill(src, dir, name string) error {
	if err := os.MkdirAll(userSkillsDir(dir), 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(dir, ".skill-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copyTree(src, stage); err != nil {
		return err
	}
	if err := os.Chmod(stage, 0o755); err != nil {
		return err
	}
	return os.Rename(stage, filepath.Join(userSkillsDir(dir), name))
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, file)
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		return copyEntry(file, filepath.Join(dst, rel), d)
	})
}

func copyEntry(file, target string, d fs.DirEntry) error {
	if d.IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if !d.Type().IsRegular() {
		return nil
	}
	info, err := d.Info()
	if err != nil {
		return err
	}
	return copyFile(file, target, info.Mode().Perm()&0o755|0o600)
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, werr := io.Copy(out, in)
	return errors.Join(werr, out.Close())
}
