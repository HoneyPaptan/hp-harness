package crew

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const draftSystem = "You split a software goal into independent tasks that separate coding agents do at the same time, each in its own git worktree of the same repository. " +
	"Reply with markdown only: one '## ' heading per task, then a short self-contained instruction for that task under it. " +
	"Tasks must not depend on each other's output and should touch different files. Use between 2 and 6 tasks. No other text."

func (s Service) DraftSystem() string {
	return draftSystem
}

func (s Service) SavePlan(reply string) (string, string, error) {
	text := unfence(reply)
	if _, err := planTitles([]byte(text)); err != nil {
		return "", "", errors.New("the model reply was not a plan with ## task headings")
	}
	dir := plansDir(s.Env.Getenv)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, newCrewID()+".md")
	if err := os.WriteFile(path, []byte(text+"\n"), 0o600); err != nil {
		return "", "", err
	}
	return path, text, nil
}

func unfence(reply string) string {
	text := strings.TrimSpace(reply)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "```"))
}
