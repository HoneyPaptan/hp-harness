package crew

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type op string

const (
	opStop      op = "stop"
	opRetry     op = "retry"
	opPause     op = "pause"
	opUnpause   op = "unpause"
	opLanded    op = "landed"
	opDiscarded op = "discarded"
)

type request struct {
	Op   op     `json:"op"`
	Task string `json:"task,omitempty"`
}

type queued struct {
	path string
	req  request
	ok   bool
}

func controlDir(getenv func(string) string, id string) string {
	return filepath.Join(crewsDir(getenv), id+".control")
}

func enqueue(getenv func(string) string, id string, r request) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	dir := controlDir(getenv, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var salt [4]byte
	_, _ = rand.Read(salt[:])
	name := filepath.Join(dir, fmt.Sprintf("%020d-%s", time.Now().UnixNano(), hex.EncodeToString(salt[:])))
	if err := os.WriteFile(name+".tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(name+".tmp", name+".json")
}

func queuedRequests(getenv func(string) string, id string) []queued {
	dir := controlDir(getenv, id)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []queued
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		q := queued{path: filepath.Join(dir, e.Name())}
		if data, err := os.ReadFile(q.path); err == nil {
			q.ok = json.Unmarshal(data, &q.req) == nil
		}
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func hasControls(getenv func(string) string, id string) bool {
	return len(queuedRequests(getenv, id)) > 0
}

func applyControls(env Env, st *State) error {
	for _, q := range queuedRequests(env.Getenv, st.ID) {
		keep := false
		if q.ok {
			var err error
			if keep, err = applyRequest(env, st, q.req); err != nil {
				return err
			}
		}
		if !keep {
			_ = os.Remove(q.path)
		}
	}
	return nil
}

func applyRequest(env Env, st *State, r request) (bool, error) {
	switch r.Op {
	case opPause, opUnpause:
		paused := r.Op == opPause
		if st.Paused == paused {
			return false, nil
		}
		st.Paused = paused
	case opLanded, opDiscarded:
		t := st.task(r.Task)
		if t == nil {
			return false, nil
		}
		t.Landed, t.Discarded = t.Landed || r.Op == opLanded, t.Discarded || r.Op == opDiscarded
	case opStop:
		t := st.task(r.Task)
		if t == nil || !stopTask(env, t) {
			return false, nil
		}
	case opRetry:
		t := st.task(r.Task)
		if t == nil || !t.retryable() {
			return false, nil
		}
		if t.RunID != "" && runAlive(env, t.RunID) {
			return true, nil
		}
		retryTask(env, t)
	default:
		return false, nil
	}
	return false, st.save(env.Getenv)
}

func stopTask(env Env, t *Task) bool {
	switch t.Status {
	case StatusQueued:
	case StatusStarting, StatusRunning:
		if t.RunID == "" {
			t.RunID, _ = findRunByDir(env, t.Worktree)
		}
		if t.RunID != "" && env.Signal != nil {
			if pid, ok := livePid(env, runPidPath(env.Getenv, t.RunID)); ok {
				_ = env.Signal(pid)
			}
		}
	default:
		return false
	}
	t.Status = StatusStopped
	t.EndReason = ""
	return true
}

func retryTask(env Env, t *Task) {
	t.EndReason = ""
	if t.RunID != "" {
		_ = os.Remove(resumeStatePath(env.Getenv, t.RunID))
		if _, run, err := replayRunRecords(env, t.RunID); err == nil && !run.Finished && !run.Stopped {
			t.Status = StatusRunning
			return
		}
	}
	t.RunID = ""
	t.Status = StatusQueued
}

func (s *State) task(slug string) *Task {
	for i := range s.Tasks {
		if s.Tasks[i].Slug == slug {
			return &s.Tasks[i]
		}
	}
	return nil
}

func findTask(st *State, arg string) (*Task, error) {
	arg = strings.TrimSpace(arg)
	if t := st.task(arg); t != nil {
		return t, nil
	}
	if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= len(st.Tasks) {
		return &st.Tasks[n-1], nil
	}
	for i := range st.Tasks {
		if st.Tasks[i].Title == arg {
			return &st.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("crew %s has no task %q", st.ID, arg)
}

func taskPending(reqs []queued, slug string) bool {
	for _, q := range reqs {
		if q.ok && q.req.Task == slug {
			return true
		}
	}
	return false
}

func effectivePaused(st *State, reqs []queued) bool {
	paused := st.Paused
	for _, q := range reqs {
		switch {
		case !q.ok:
		case q.req.Op == opPause:
			paused = true
		case q.req.Op == opUnpause:
			paused = false
		}
	}
	return paused
}

func (s Service) Stop(id, task string) error {
	return s.taskControl(id, task, opStop, "is not running", (*Task).active)
}

func (s Service) Retry(id, task string) error {
	return s.taskControl(id, task, opRetry, "did not fail or stop", (*Task).retryable)
}

func (s Service) taskControl(id, task string, o op, refusal string, allowed func(*Task) bool) error {
	_, t, err := s.gate(id, task, refusal, allowed)
	if err != nil {
		return err
	}
	return s.submit(id, request{Op: o, Task: t.Slug})
}

func (s Service) Pause(id string) error {
	return s.crewControl(id, opPause)
}

func (s Service) Unpause(id string) error {
	return s.crewControl(id, opUnpause)
}

func (s Service) crewControl(id string, o op) error {
	st, err := loadSafe(s.Env.Getenv, id)
	if err != nil {
		return err
	}
	paused := effectivePaused(st, queuedRequests(s.Env.Getenv, id))
	switch {
	case o == opPause && st.done():
		return errors.New("the crew is done, nothing to pause")
	case o == opPause && paused:
		return errors.New("the crew is already paused")
	case o == opUnpause && !paused:
		return errors.New("the crew is not paused")
	}
	return s.submit(id, request{Op: o})
}

func (s Service) submit(id string, r request) error {
	if err := enqueue(s.Env.Getenv, id, r); err != nil {
		return err
	}
	if _, alive := supervisorPid(s.Env, id); alive {
		return nil
	}
	return s.spawn(id)
}

func loadSafe(getenv func(string) string, id string) (*State, error) {
	if !safeID(id) {
		return nil, fmt.Errorf("crew %q not found", id)
	}
	return load(getenv, id)
}
