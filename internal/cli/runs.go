package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
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
	ents, err := env.Host.ReadDir(dir)
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

func latestRunIn(env Env, dir string) (string, error) {
	base := runsDir(env.Getenv)
	ids, err := sortedRunIDs(env, base)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		start, err := journal.ReadStart(env.Host, filepath.Join(base, id+".jsonl"))
		if err == nil && filepath.Clean(start.Dir) == filepath.Clean(dir) {
			return id, nil
		}
	}
	return "", nil
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
