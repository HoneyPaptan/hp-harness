package crew

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const defaultEditor = "vi"

func editorArgv(getenv func(string) string) []string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if argv := strings.Fields(getenv(name)); len(argv) > 0 {
			return argv
		}
	}
	return []string{defaultEditor}
}

func (s Service) EditPlan(plan string) ([]string, error) {
	if s.Env.Handover == nil {
		return nil, errors.New("editing the plan needs a terminal")
	}
	path := resolvePath(s.Env.Cwd, plan)
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	argv := editorArgv(s.Env.Getenv)
	editErr := s.Env.Handover(append(argv, path))
	titles, found, err := s.PlanTitles(plan)
	switch {
	case editErr != nil:
		return titles, fmt.Errorf("%s did not finish: %w", argv[0], editErr)
	case !found:
		return nil, errors.New("the plan file is gone")
	}
	return titles, err
}
