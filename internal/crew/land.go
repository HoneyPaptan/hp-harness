package crew

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	errDirty    = errors.New("your repository has uncommitted changes, commit or stash them first")
	errDetached = errors.New("your repository is on a detached HEAD, check out a branch first")
)

func (t *Task) active() bool {
	return t.Status == StatusQueued || t.Status == StatusStarting || t.Status == StatusRunning
}

func (t *Task) retryable() bool {
	return (t.Status == StatusFailed || t.Status == StatusStopped) && !t.Discarded
}

func (t *Task) reviewable() bool {
	return taskStopped(t) && !t.Discarded
}

func (t *Task) landable() bool {
	return t.Status == StatusFinished && t.Check != checkRunning && !t.Landed && !t.Discarded
}

func (t *Task) discardable() bool {
	return t.reviewable() && t.Check != checkRunning
}

func (s Service) gate(id, task, refusal string, allowed func(*Task) bool) (*State, *Task, error) {
	st, err := loadSafe(s.Env.Getenv, id)
	if err != nil {
		return nil, nil, err
	}
	t, err := findTask(st, task)
	if err != nil {
		return nil, nil, err
	}
	if taskPending(queuedRequests(s.Env.Getenv, id), t.Slug) {
		return nil, nil, fmt.Errorf("%s already has a request waiting", t.Title)
	}
	if !allowed(t) {
		return nil, nil, fmt.Errorf("%s %s", t.Title, refusal)
	}
	return st, t, nil
}

func (s Service) Diff(id, task string) ([]string, error) {
	st, t, err := s.gate(id, task, "has no worktree to show", (*Task).reviewable)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(t.Worktree); err != nil {
		return nil, fmt.Errorf("%s has no worktree to show", t.Title)
	}
	out, err := git(s.Env, t.Worktree, "diff", st.Base)
	if err != nil {
		return nil, err
	}
	names, _ := git(s.Env, t.Worktree, "ls-files", "-z", "--others", "--exclude-standard")
	for _, name := range strings.Split(names, "\x00") {
		if name != "" {
			added, _ := git(s.Env, t.Worktree, "diff", "--no-index", "--", os.DevNull, name)
			out += added
		}
	}
	if strings.TrimSpace(out) == "" {
		return []string{"(no changes)"}, nil
	}
	return textLines(out), nil
}

func landTarget(env Env, st *State) (string, error) {
	status, err := git(env, st.Repo, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return "", err
	}
	if countLines(status) > 0 {
		return "", errDirty
	}
	branch, err := git(env, st.Repo, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return "", errDetached
	}
	return strings.TrimSpace(branch), nil
}

func (s Service) LandCheck(id, task string) (string, error) {
	st, t, err := s.gate(id, task, "cannot be landed now", (*Task).landable)
	if err != nil {
		return "", err
	}
	target, err := landTarget(s.Env, st)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("merge %s into %s?", t.Branch, target), nil
}

func (s Service) Land(id, task string) (string, error) {
	st, t, err := s.gate(id, task, "cannot be landed now", (*Task).landable)
	if err != nil {
		return "", err
	}
	target, err := landTarget(s.Env, st)
	if err != nil {
		return "", err
	}
	if status, _ := git(s.Env, t.Worktree, "status", "--porcelain"); countLines(status) > 0 {
		if _, err := git(s.Env, t.Worktree, "add", "-A"); err != nil {
			return "", err
		}
		if _, err := git(s.Env, t.Worktree, "commit", "-m", t.Title); err != nil {
			return "", err
		}
	}
	if _, err := git(s.Env, st.Repo, "merge-base", "--is-ancestor", t.Branch, "HEAD"); err == nil {
		return "", fmt.Errorf("%s changed nothing that %s lacks", t.Title, target)
	}
	if out, err := git(s.Env, st.Repo, "merge", "--no-ff", "-m", "Merge "+t.Branch+": "+t.Title, t.Branch); err != nil {
		files, _ := git(s.Env, st.Repo, "diff", "--name-only", "--diff-filter=U")
		_, _ = git(s.Env, st.Repo, "merge", "--abort")
		if names := strings.Fields(files); len(names) > 0 {
			return "", fmt.Errorf("merging %s conflicts in %s, your repository is left as it was", t.Branch, strings.Join(names, ", "))
		}
		return "", fmt.Errorf("merging %s failed, your repository is left as it was: %s", t.Branch, firstLine(out))
	}
	if err := s.submit(id, request{Op: opLanded, Task: t.Slug}); err != nil {
		return "", err
	}
	return "landed " + t.Branch + " into " + target, nil
}

func (s Service) Discard(id, task string) error {
	st, t, err := s.gate(id, task, "cannot be discarded now", (*Task).discardable)
	if err != nil {
		return err
	}
	if _, err := os.Stat(t.Worktree); err == nil {
		if _, err := git(s.Env, st.Repo, "worktree", "remove", "--force", t.Worktree); err != nil {
			return err
		}
	}
	if _, err := git(s.Env, st.Repo, "rev-parse", "--verify", "-q", "refs/heads/"+t.Branch); err == nil {
		if _, err := git(s.Env, st.Repo, "branch", "-D", t.Branch); err != nil {
			return err
		}
	}
	return s.submit(id, request{Op: opDiscarded, Task: t.Slug})
}
