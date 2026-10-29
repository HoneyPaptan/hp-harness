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
	dir := runsDir(env.Getenv)
	ids, empty, err := listRunIDs(env, dir)
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	if empty {
		fmt.Fprintln(env.Stdout, "no runs yet")
		return 0
	}
	for _, id := range ids {
		printRunLine(env, dir, id)
	}
	return 0
}

func listRunIDs(env Env, dir string) ([]string, bool, error) {
	ents, err := env.Host.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, true, nil
		}
		return nil, false, err
	}
	ids := runIDs(ents)
	if len(ids) == 0 {
		return nil, true, nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	if len(ids) > 20 {
		ids = ids[:20]
	}
	return ids, false, nil
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

func printRunLine(env Env, dir, id string) {
	jpath := filepath.Join(dir, id+".jsonl")
	recs, err := journal.Read(env.Host, jpath)
	if err != nil {
		fmt.Fprintf(env.Stdout, "%s  broken\n", id)
		return
	}
	st, err := journal.Replay(recs)
	if err != nil {
		fmt.Fprintf(env.Stdout, "%s  broken\n", id)
		return
	}
	fmt.Fprintf(env.Stdout, "%s  %s  %s/%s  %s\n",
		id, listStatus(st), st.Start.Preset, st.Start.Model, cutRunes(oneLine(firstPrompt(st)), 50))
}

func listStatus(st journal.State) string {
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
