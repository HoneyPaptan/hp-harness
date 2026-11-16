package crew

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

const noCrews = "no crews yet\n"

func statusText(env Env, st *State) string {
	var b strings.Builder
	if st.Paused {
		b.WriteString("paused\n")
	}
	for i := range st.Tasks {
		t := &st.Tasks[i]
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s\n",
			t.Title, statusWord(t), runID(t), t.Branch, changedFiles(env, st, t))
	}
	return b.String()
}

func reportText(env Env, st *State) string {
	var b strings.Builder
	for i := range st.Tasks {
		t := &st.Tasks[i]
		fmt.Fprintf(&b, "## %s\n\n", t.Title)
		if t.Status != StatusFinished {
			fmt.Fprintf(&b, "%s\n\n", statusWord(t))
			continue
		}
		fmt.Fprintf(&b, "run %s  branch %s\n\n", t.RunID, t.Branch)
		stat := diffStat(env, st, t)
		if stat == "" {
			stat = "(no changes)"
		}
		fmt.Fprintf(&b, "%s\n\n", stat)
		answer := taskAnswer(env, t.RunID)
		if answer == "" {
			answer = "(no final answer)"
		}
		fmt.Fprintf(&b, "%s\n\n", answer)
	}
	return b.String()
}

func listText(env Env) string {
	ids := crewIDs(env)
	if len(ids) == 0 {
		return noCrews
	}
	var b strings.Builder
	for _, id := range ids {
		st, err := load(env.Getenv, id)
		if err != nil {
			fmt.Fprintf(&b, "%s  broken\n", id)
			continue
		}
		fmt.Fprintf(&b, "%s  %s  %d/%d done%s\n", id, st.label(env.Getenv), st.doneCount(), len(st.Tasks), pausedSuffix(st))
	}
	return b.String()
}

func pausedSuffix(st *State) string {
	if st.Paused {
		return "  paused"
	}
	return ""
}

func crewIDs(env Env) []string {
	ents, err := os.ReadDir(crewsDir(env.Getenv))
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids
}

func textLines(text string) []string {
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}
