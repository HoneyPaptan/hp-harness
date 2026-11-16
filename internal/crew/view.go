package crew

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

var (
	errNoRepo   = errors.New("/crew needs a git repository here (run git init first)")
	errNoCommit = errors.New("/crew needs a commit in this repository (run git commit first)")
)

func (s Service) Preflight() (int, error) {
	if _, err := git(s.Env, s.Env.Cwd, "rev-parse", "--show-toplevel"); err != nil {
		return 0, errNoRepo
	}
	if _, err := git(s.Env, s.Env.Cwd, "rev-parse", "HEAD"); err != nil {
		return 0, errNoCommit
	}
	out, err := git(s.Env, s.Env.Cwd, "status", "--porcelain")
	if err != nil {
		return 0, err
	}
	return countLines(out), nil
}

func (s Service) View(id string) (crewview.Snapshot, bool, error) {
	if id == "" {
		ids := crewIDs(s.Env)
		if len(ids) == 0 {
			return crewview.Snapshot{}, false, nil
		}
		id = ids[0]
	}
	if !safeID(id) {
		return crewview.Snapshot{}, false, nil
	}
	st, err := load(s.Env.Getenv, id)
	if errors.Is(err, os.ErrNotExist) {
		return crewview.Snapshot{}, false, nil
	}
	if err != nil {
		return crewview.Snapshot{}, false, err
	}
	reqs := queuedRequests(s.Env.Getenv, id)
	snap := crewview.Snapshot{
		ID: id, Label: st.label(s.Env.Getenv), Done: st.done() && len(reqs) == 0,
		Paused: effectivePaused(st, reqs), Rows: make([]crewview.Row, len(st.Tasks)),
	}
	for i := range st.Tasks {
		t := &st.Tasks[i]
		snap.Rows[i] = crewview.Row{
			Slug: t.Slug, Title: t.Title, Status: statusWord(t), RunID: runID(t),
			Changed: changedFiles(s.Env, st, t), Keys: taskKeys(t),
		}
		if word := pendingWord(reqs, t.Slug); word != "" {
			snap.Rows[i].Status, snap.Rows[i].Keys = word, ""
		}
	}
	return snap, true, nil
}

func taskKeys(t *Task) string {
	keys := ""
	for _, k := range []struct {
		key string
		ok  bool
	}{{"s", t.active()}, {"r", t.retryable()}, {"d", t.reviewable()}, {"a", t.landable()}, {"x", t.discardable()}} {
		if k.ok {
			keys += k.key
		}
	}
	return keys
}

func pendingWord(reqs []queued, slug string) string {
	for _, q := range reqs {
		if !q.ok || q.req.Task != slug {
			continue
		}
		switch q.req.Op {
		case opStop:
			return "stopping"
		case opRetry:
			return "retrying"
		case opLanded:
			return "landed"
		case opDiscarded:
			return "discarded"
		}
	}
	return ""
}

func (s Service) Follow(id string, index, offset int) (crewview.Chunk, error) {
	st, err := load(s.Env.Getenv, id)
	if err != nil {
		return crewview.Chunk{}, err
	}
	if index < 0 || index >= len(st.Tasks) {
		return crewview.Chunk{}, fmt.Errorf("crew %s has no task %d", id, index+1)
	}
	t := &st.Tasks[index]
	if t.RunID == "" {
		if !taskStopped(t) {
			return crewview.Chunk{Waiting: true}, nil
		}
		return crewview.Chunk{Ended: true, Trailer: s.trailer(st, t, "")}, nil
	}
	ended := s.taskEnded(t) && !st.verifyPending(t)
	data, _ := os.ReadFile(runLogPath(s.Env.Getenv, t.RunID))
	if offset > len(data) {
		offset = 0
	}
	chunk := crewview.Chunk{Text: string(data[offset:]), Offset: len(data), Ended: ended}
	if ended {
		chunk.Trailer = s.trailer(st, t, string(data))
	}
	return chunk, nil
}

func (s Service) taskEnded(t *Task) bool {
	if taskStopped(t) {
		return true
	}
	if runAlive(s.Env, t.RunID) {
		return false
	}
	_, run, err := replayRunRecords(s.Env, t.RunID)
	return err == nil && (run.Finished || run.Stopped)
}

func (s Service) trailer(st *State, t *Task, log string) []string {
	if t.Status == StatusFailed {
		return []string{statusWord(t)}
	}
	if t.Status == StatusStopped {
		return []string{"stopped"}
	}
	lines := []string{"finished · branch " + t.Branch}
	stat := diffStat(s.Env, st, t)
	if stat == "" {
		stat = "(no changes)"
	}
	lines = append(lines, textLines(stat)...)
	if answer := taskAnswer(s.Env, t.RunID); answer != "" && !strings.Contains(log, answer) {
		lines = append(lines, "")
		lines = append(lines, textLines(answer)...)
	}
	return append(lines, verifyLines(s.Env, st, t)...)
}

func taskStopped(t *Task) bool {
	return t.Status == StatusFinished || t.Status == StatusFailed || t.Status == StatusStopped
}

func runLogPath(getenv func(string) string, id string) string {
	return filepath.Join(runsDir(getenv), id+".log")
}

func safeID(id string) bool {
	return id != "" && !strings.HasPrefix(id, ".") && !strings.ContainsAny(id, "/\\\x00")
}
