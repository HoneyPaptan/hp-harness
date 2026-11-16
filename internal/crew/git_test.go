package crew

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "t@example.com")
	runGit(t, repo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-q", "-m", "init")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func testEnvGit(getenv func(string) string) Env {
	return Env{Getenv: getenv, Cwd: "/proj", Alive: func(int) bool { return false }}
}

func TestGitRootHead(t *testing.T) {
	repo := initRepo(t)
	env := testEnvGit(getenvMap(map[string]string{"HOME": "/h"}))
	root, head, err := gitRootHead(env, repo)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(root) != filepath.Clean(repo) {
		t.Fatalf("root %q want %q", root, repo)
	}
	if len(head) != 40 {
		t.Fatalf("head %q", head)
	}
}

func TestWorktreeAndDiff(t *testing.T) {
	repo := initRepo(t)
	state := t.TempDir()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": state, "HOME": "/h"})
	env := testEnvGit(getenv)
	_, base, err := gitRootHead(env, repo)
	if err != nil {
		t.Fatal(err)
	}
	st := newState(getenv, "c1", "/p/plan.md", repo, base, runFlags{n: 2}, []planTask{
		{title: "Change", prompt: "change", slug: "change"},
	})
	task := &st.Tasks[0]
	if err := ensureWorktree(env, st, task); err != nil {
		t.Fatal(err)
	}
	task.Status = StatusRunning
	if got := changedFiles(env, st, task); got != "0 files" {
		t.Fatalf("changed files before edit %q", got)
	}
	if err := os.WriteFile(filepath.Join(task.Worktree, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := changedFiles(env, st, task); got != "1 file" {
		t.Fatalf("changed files after edit %q", got)
	}
	if stat := diffStat(env, st, task); !strings.Contains(stat, "b.txt") {
		t.Fatalf("diffstat %q", stat)
	}
	branch := runGit(t, repo, "branch", "--show-current")
	if branch == task.Branch {
		t.Fatal("main repo branch moved")
	}
	wtBranch := runGit(t, task.Worktree, "branch", "--show-current")
	if strings.TrimSpace(wtBranch) != task.Branch {
		t.Fatalf("worktree branch %q want %q", wtBranch, task.Branch)
	}
}

func TestWorktreeAlreadyThere(t *testing.T) {
	repo := initRepo(t)
	state := t.TempDir()
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": state, "HOME": "/h"})
	env := testEnvGit(getenv)
	_, base, err := gitRootHead(env, repo)
	if err != nil {
		t.Fatal(err)
	}
	st := newState(getenv, "c2", "/p", repo, base, runFlags{n: 1}, []planTask{
		{title: "T", prompt: "t", slug: "t"},
	})
	task := &st.Tasks[0]
	if err := ensureWorktree(env, st, task); err != nil {
		t.Fatal(err)
	}
	if err := ensureWorktree(env, st, task); err != nil {
		t.Fatalf("second ensureWorktree: %v", err)
	}
}
