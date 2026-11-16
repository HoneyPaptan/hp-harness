package crew

import (
	"context"
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
	changed := false
	for i := range st.Tasks {
		t := &st.Tasks[i]
		if t.Status != StatusRunning {
			continue
		}
		if reason, ok := runFinished(env, t.RunID); ok {
			t.Status = StatusFinished
			t.EndReason = reason
			changed = true
			continue
		}
		if runAlive(env, t.RunID) {
			continue
		}
		if err := resumeRun(ctx, env, t.RunID); err != nil {
			t.Status = StatusFailed
			t.EndReason = err.Error()
			changed = true
		}
	}
	running := st.runningCount()
	for i := range st.Tasks {
		if running >= st.Concurrency {
			break
		}
		t := &st.Tasks[i]
		if t.Status != StatusQueued {
			continue
		}
		if err := startTask(ctx, env, st, t); err != nil {
			t.Status = StatusFailed
			t.EndReason = err.Error()
			changed = true
			continue
		}
		t.Status = StatusRunning
		running++
		changed = true
	}
	if changed {
		return st.save(env.Getenv)
	}
	return nil
}

func startTask(ctx context.Context, env Env, st *State, t *Task) error {
	if err := ensureWorktree(env, st, t); err != nil {
		return err
	}
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
		return err
	}
	id := firstLine(out)
	if id == "" {
		return fmt.Errorf("hp run printed no run id")
	}
	t.RunID = id
	return nil
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

func runFinished(env Env, id string) (string, bool) {
	st, err := replayRun(env, id)
	if err != nil {
		return "", false
	}
	if !st.Finished {
		return "", false
	}
	return st.EndReason, true
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

func replayRun(env Env, id string) (journal.State, error) {
	recs, err := journal.Read(host.Native{}, filepath.Join(runsDir(env.Getenv), id+".jsonl"))
	if err != nil {
		return journal.State{}, err
	}
	return journal.Replay(recs)
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
