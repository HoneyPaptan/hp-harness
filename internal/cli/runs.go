package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/tui"
)

func cmdRuns(env Env, args []string) int {
	if len(args) != 0 {
		return exitUsage(env, usagef("unexpected arguments"))
	}
	lines, err := runLines(env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	for _, line := range lines {
		fmt.Fprintln(env.Stdout, line)
	}
	return 0
}

func runLines(env Env) ([]string, error) {
	dir := runsDir(env.Getenv)
	ids, err := sortedRunIDs(env, dir)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []string{"no runs yet"}, nil
	}
	if len(ids) > 20 {
		ids = ids[:20]
	}
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, runLine(env, dir, id))
	}
	return lines, nil
}

func sortedRunIDs(env Env, dir string) ([]string, error) {
	ents, err := host.ReadNames(env.Host, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	ids := runIDs(ents)
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids, nil
}

const recentRunLimit = 20

func runsIn(env Env, dir string, limit int) ([]string, error) {
	base := runsDir(env.Getenv)
	ids, err := sortedRunIDs(env, base)
	if err != nil {
		return nil, err
	}
	var found []string
	for _, id := range ids {
		start, err := journal.ReadStart(env.Host, filepath.Join(base, id+".jsonl"))
		if err == nil && filepath.Clean(start.Dir) == filepath.Clean(dir) {
			found = append(found, id)
		}
		if len(found) == limit {
			break
		}
	}
	return found, nil
}

func runChoices(env Env, ids []string) []tui.RunChoice {
	base := runsDir(env.Getenv)
	now := env.Host.Now()
	choices := make([]tui.RunChoice, len(ids))
	for i, id := range ids {
		prompt := "(broken journal)"
		if st, err := replayRun(env, filepath.Join(base, id+".jsonl")); err == nil {
			prompt = cutRunes(oneLine(firstPrompt(st)), 50)
		}
		choices[i] = tui.RunChoice{ID: id, Label: fmt.Sprintf("%-8s %s  %s", runAge(id, now), prompt, id)}
	}
	return choices
}

func runAge(id string, now time.Time) string {
	if len(id) < 15 {
		return ""
	}
	t, err := time.Parse("20060102-150405", id[:15])
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func runIDs(ents []host.Entry) []string {
	var out []string
	for _, e := range ents {
		if e.IsDir || !strings.HasSuffix(e.Name, ".jsonl") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name, ".jsonl"))
	}
	return out
}

func runLine(env Env, dir, id string) string {
	st, err := replayRun(env, filepath.Join(dir, id+".jsonl"))
	if err != nil {
		return id + "  broken"
	}
	return fmt.Sprintf("%s  %s  %s/%s  %s",
		id, listStatus(env, id, st), st.Start.Preset, st.Start.Model, cutRunes(oneLine(firstPrompt(st)), 50))
}

func listStatus(env Env, id string, st journal.State) string {
	if runAlive(env, id) {
		return "running"
	}
	if st.Finished {
		return st.EndReason
	}
	return "resumable"
}

func firstPrompt(st journal.State) string {
	for _, m := range st.Messages {
		if m.Role == provider.RoleUser && m.Text != "" {
			return m.Text
		}
	}
	return ""
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
