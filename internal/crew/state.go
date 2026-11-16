package crew

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Status string

const (
	StatusQueued   Status = "queued"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusFinished Status = "finished"
	StatusFailed   Status = "failed"
	StatusStopped  Status = "stopped"
)

type Task struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
	Branch    string `json:"branch"`
	Worktree  string `json:"worktree"`
	RunID     string `json:"run_id,omitempty"`
	Status    Status `json:"status"`
	EndReason string `json:"end_reason,omitempty"`
}

type State struct {
	ID          string    `json:"id"`
	Plan        string    `json:"plan"`
	Repo        string    `json:"repo"`
	Base        string    `json:"base"`
	Concurrency int       `json:"concurrency"`
	Model       string    `json:"model,omitempty"`
	BaseURL     string    `json:"base_url,omitempty"`
	KeyEnv      string    `json:"key_env,omitempty"`
	Label       string    `json:"label,omitempty"`
	Created     time.Time `json:"created"`
	Tasks       []Task    `json:"tasks"`
}

func runsDir(getenv func(string) string) string {
	return stateHome(getenv, "runs")
}

func crewsDir(getenv func(string) string) string {
	return stateHome(getenv, "crews")
}

func stateHome(getenv func(string) string, leaf string) string {
	if base := getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, "hp", leaf)
	}
	home := getenv("HOME")
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".local", "state", "hp", leaf)
}

func plansDir(getenv func(string) string) string {
	return filepath.Join(crewsDir(getenv), "plans")
}

func statePath(getenv func(string) string, id string) string {
	return filepath.Join(crewsDir(getenv), id+".json")
}

func newCrewID() string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(b[:]))
}

func newState(getenv func(string) string, id, plan, repo, base string, o runFlags, tasks []planTask) *State {
	st := &State{
		ID: id, Plan: plan, Repo: repo, Base: base,
		Concurrency: o.n, Model: o.model, BaseURL: o.baseURL, KeyEnv: o.keyEnv,
		Created: time.Now().UTC(),
	}
	for _, pt := range tasks {
		st.Tasks = append(st.Tasks, Task{
			Slug:     pt.slug,
			Title:    pt.title,
			Prompt:   pt.prompt,
			Branch:   fmt.Sprintf("crew/%s/%s", id, pt.slug),
			Worktree: filepath.Join(crewsDir(getenv), id, pt.slug),
			Status:   StatusQueued,
		})
	}
	return st
}

func (s *State) save(getenv func(string) string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(crewsDir(getenv), 0o700); err != nil {
		return err
	}
	tmp := statePath(getenv, s.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(getenv, s.ID))
}

func load(getenv func(string) string, id string) (*State, error) {
	data, err := os.ReadFile(statePath(getenv, id))
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("crew %q state is damaged: %v", id, err)
	}
	if s.ID != id {
		return nil, fmt.Errorf("crew %q not found", id)
	}
	return &s, nil
}

func (s *State) done() bool {
	for i := range s.Tasks {
		switch s.Tasks[i].Status {
		case StatusFinished, StatusFailed, StatusStopped:
		default:
			return false
		}
	}
	return true
}

func (s *State) activeCount() int {
	n := 0
	for i := range s.Tasks {
		if s.Tasks[i].Status == StatusRunning || s.Tasks[i].Status == StatusStarting {
			n++
		}
	}
	return n
}

func (s *State) doneCount() int {
	n := 0
	for i := range s.Tasks {
		if s.Tasks[i].Status == StatusFinished {
			n++
		}
	}
	return n
}
