package crew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const pollInterval = 2 * time.Second

func supervise(ctx context.Context, env Env, st *State) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := advance(ctx, env, st); err != nil {
			return err
		}
		if st.done() {
			return nil
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
	}
}

func advance(ctx context.Context, env Env, st *State) error {
	for i := range st.Tasks {
		t := &st.Tasks[i]
		switch t.Status {
		case StatusRunning:
			if err := settleRunning(ctx, env, st, t); err != nil {
				return err
			}
		case StatusStarting:
			if err := settleStarting(ctx, env, st, t); err != nil {
				return err
			}
		}
	}
	active := st.activeCount()
	for i := range st.Tasks {
		if active >= st.Concurrency {
			break
		}
		t := &st.Tasks[i]
		if t.Status != StatusQueued {
			continue
		}
		if err := startTask(ctx, env, st, t); err != nil {
			return err
		}
		active++
	}
	return nil
}

func settleRunning(ctx context.Context, env Env, st *State, t *Task) error {
	if _, run, err := replayRunRecords(env, t.RunID); err == nil {
		if ended, err := settleEnded(env, st, t, run); ended {
			return err
		}
	}
	if runAlive(env, t.RunID) {
		return nil
	}
	return resumeDead(ctx, env, st, t)
}

func settleEnded(env Env, st *State, t *Task, run journal.State) (bool, error) {
	switch {
	case run.Finished:
		t.Status = StatusFinished
		t.EndReason = run.EndReason
	case run.Stopped:
		t.Status = StatusStopped
	default:
		return false, nil
	}
	return true, st.save(env.Getenv)
}

func resumeDead(ctx context.Context, env Env, st *State, t *Task) error {
	unlock, err := host.Native{}.Lock(recoverLockPath(env.Getenv))
	if errors.Is(err, host.ErrLocked) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unlock()
	if runAlive(env, t.RunID) {
		return nil
	}
	recs, run, err := replayRunRecords(env, t.RunID)
	if err == nil {
		if ended, err := settleEnded(env, st, t, run); ended {
			return err
		}
	}
	resume, next := journal.NextResume(run, recs, loadResumeState(env, t.RunID))
	if !resume {
		return failTask(env, st, t, fmt.Errorf("keeps crashing after %d resumes", journal.CrashLimit))
	}
	if err := saveResumeState(env, t.RunID, next); err != nil {
		return err
	}
	if err := resumeRun(ctx, env, t.RunID); err != nil {
		return failTask(env, st, t, err)
	}
	return nil
}

func recoverLockPath(getenv func(string) string) string {
	return filepath.Join(runsDir(getenv), "recover.lock")
}

func settleStarting(ctx context.Context, env Env, st *State, t *Task) error {
	if id, ok := findRunByDir(env, t.Worktree); ok {
		t.RunID = id
		t.Status = StatusRunning
		return st.save(env.Getenv)
	}
	return launchTask(ctx, env, st, t)
}

func startTask(ctx context.Context, env Env, st *State, t *Task) error {
	if err := ensureWorktree(env, st, t); err != nil {
		return failTask(env, st, t, err)
	}
	t.Status = StatusStarting
	if err := st.save(env.Getenv); err != nil {
		return err
	}
	return launchTask(ctx, env, st, t)
}

func launchTask(ctx context.Context, env Env, st *State, t *Task) error {
	args := []string{"run", "-d", "-C", t.Worktree}
	if st.Model != "" {
		args = append(args, "-m", st.Model)
	}
	if st.BaseURL != "" {
		args = append(args, "--base-url", st.BaseURL)
	}
	if st.KeyEnv != "" {
		args = append(args, "--key-env", st.KeyEnv)
	}
	args = append(args, "--", t.Prompt)
	out, err := runHP(ctx, env, args...)
	if err != nil {
		return failTask(env, st, t, err)
	}
	id := firstLine(out)
	if id == "" {
		return failTask(env, st, t, fmt.Errorf("hp run printed no run id"))
	}
	t.RunID = id
	t.Status = StatusRunning
	return st.save(env.Getenv)
}

func failTask(env Env, st *State, t *Task, err error) error {
	t.Status = StatusFailed
	t.EndReason = err.Error()
	return st.save(env.Getenv)
}

func resumeRun(ctx context.Context, env Env, id string) error {
	if _, err := runHP(ctx, env, "resume", "-d", id); err != nil {
		return err
	}
	return nil
}

func runHP(ctx context.Context, env Env, args ...string) (string, error) {
	if env.RunHP != nil {
		return env.RunHP(ctx, args...)
	}
	return execHP(ctx, env, args...)
}

func runAlive(env Env, id string) bool {
	pid, ok := readRunPid(env.Getenv, id)
	if !ok {
		return false
	}
	return env.Alive != nil && env.Alive(pid)
}

func readRunPid(getenv func(string) string, id string) (int, bool) {
	data, err := os.ReadFile(filepath.Join(runsDir(getenv), id+".pid"))
	if err != nil {
		return 0, false
	}
	pidText, _, _ := strings.Cut(strings.TrimSpace(string(data)), " ")
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return 0, false
	}
	return pid, true
}

func replayRunRecords(env Env, id string) ([]journal.Record, journal.State, error) {
	recs, err := journal.Read(host.Native{}, filepath.Join(runsDir(env.Getenv), id+".jsonl"))
	if err != nil {
		return nil, journal.State{}, err
	}
	st, err := journal.Replay(recs)
	if err != nil {
		return recs, journal.State{}, err
	}
	return recs, st, nil
}

func replayRun(env Env, id string) (journal.State, error) {
	_, st, err := replayRunRecords(env, id)
	return st, err
}

func findRunByDir(env Env, dir string) (string, bool) {
	ents, err := os.ReadDir(runsDir(env.Getenv))
	if err != nil {
		return "", false
	}
	for i := len(ents) - 1; i >= 0; i-- {
		if ents[i].IsDir() || !strings.HasSuffix(ents[i].Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(ents[i].Name(), ".jsonl")
		start, err := journal.ReadStart(host.Native{}, filepath.Join(runsDir(env.Getenv), id+".jsonl"))
		if err == nil && filepath.Clean(start.Dir) == filepath.Clean(dir) {
			return id, true
		}
	}
	return "", false
}

func loadResumeState(env Env, id string) journal.ResumeState {
	var st journal.ResumeState
	data, err := os.ReadFile(resumeStatePath(env.Getenv, id))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

func saveResumeState(env Env, id string, st journal.ResumeState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(resumeStatePath(env.Getenv, id), data, 0o600)
}

func resumeStatePath(getenv func(string) string, id string) string {
	return filepath.Join(runsDir(getenv), id+".recover")
}

func taskAnswer(env Env, id string) string {
	st, err := replayRun(env, id)
	if err != nil {
		return ""
	}
	return finalAnswer(st)
}

func finalAnswer(st journal.State) string {
	for i := len(st.Messages) - 1; i >= 0; i-- {
		m := st.Messages[i]
		if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Text) != "" {
			return strings.TrimSpace(m.Text)
		}
	}
	return ""
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
