package crew

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/crewview"
)

type Service struct{ Env Env }

func (s Service) PlanTitles(plan string) ([]string, bool, error) {
	path := resolvePath(s.Env.Cwd, plan)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return nil, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, true, err
	}
	titles, err := planTitles(data)
	return titles, true, err
}

func (s Service) Start(req crewview.Start) (string, error) {
	if req.N < 1 {
		return "", errors.New("agents at once must be at least 1")
	}
	planPath := resolvePath(s.Env.Cwd, req.Plan)
	data, err := os.ReadFile(planPath)
	if err != nil {
		return "", err
	}
	tasks, err := parsePlan(data)
	if err != nil {
		return "", err
	}
	repo, base, err := gitRootHead(s.Env, s.Env.Cwd)
	if err != nil {
		return "", err
	}
	o := runFlags{n: req.N, model: req.Model, baseURL: req.BaseURL, keyEnv: req.KeyEnv}
	st := newState(s.Env.Getenv, newCrewID(), planPath, repo, base, o, tasks)
	st.Label = strings.Join(strings.Fields(req.Label), " ")
	if err := st.save(s.Env.Getenv); err != nil {
		return "", err
	}
	if err := s.spawn(st.ID); err != nil {
		return "", fmt.Errorf("crew %s created but its supervisor did not start: %w", st.ID, err)
	}
	return st.ID, nil
}

func (s Service) spawn(id string) error {
	if s.Env.Spawn == nil {
		return errors.New("detaching is not supported here")
	}
	pid, err := s.Env.Spawn([]string{"crew", "resume", id}, logPath(s.Env.Getenv, id))
	if err != nil {
		return err
	}
	return os.WriteFile(pidPath(s.Env.Getenv, id), []byte(strconv.Itoa(pid)), 0o600)
}

func (s Service) List() ([]string, error) {
	return textLines(listText(s.Env)), nil
}

func (s Service) Status(id string) ([]string, error) {
	st, err := load(s.Env.Getenv, id)
	if err != nil {
		return nil, err
	}
	return textLines(statusText(s.Env, st)), nil
}

func (s Service) Report(id string) ([]string, error) {
	st, err := load(s.Env.Getenv, id)
	if err != nil {
		return nil, err
	}
	return textLines(reportText(s.Env, st)), nil
}

func (s Service) Brief(id string) (string, bool, error) {
	st, err := load(s.Env.Getenv, id)
	if err != nil {
		return "", false, err
	}
	text := st.brief()
	if st.done() {
		return text, true, nil
	}
	if _, alive := supervisorPid(s.Env, id); !alive {
		text += " · supervisor not running"
	}
	return text, false, nil
}

func logPath(getenv func(string) string, id string) string {
	return filepath.Join(crewsDir(getenv), id+".log")
}

func planTitles(data []byte) ([]string, error) {
	tasks, err := parsePlan(data)
	if err != nil {
		return nil, err
	}
	titles := make([]string, len(tasks))
	for i, t := range tasks {
		titles[i] = t.title
	}
	return titles, nil
}

func (s *State) brief() string {
	counts := map[string]int{}
	for i := range s.Tasks {
		counts[briefWord(s.Tasks[i].Status)]++
	}
	var parts []string
	for _, word := range []string{"queued", "running", "finished", "failed", "stopped"} {
		if n := counts[word]; n > 0 {
			parts = append(parts, strconv.Itoa(n)+" "+word)
		}
	}
	return strings.Join(parts, " · ")
}

func briefWord(s Status) string {
	if s == StatusStarting {
		return string(StatusRunning)
	}
	return string(s)
}
