package crew

import (
	"fmt"
	"strings"
)

type planTask struct {
	title  string
	prompt string
	slug   string
}

func parsePlan(data []byte) ([]planTask, error) {
	var out []planTask
	var cur *planTask
	flush := func() {
		if cur == nil {
			return
		}
		cur.prompt = strings.TrimSpace(cur.prompt)
		if cur.prompt == "" {
			cur.prompt = cur.title
		}
		out = append(out, *cur)
		cur = nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			flush()
			cur = &planTask{title: strings.TrimSpace(strings.TrimPrefix(line, "## "))}
		case strings.HasPrefix(line, "# "):
		case cur != nil:
			cur.prompt += line + "\n"
		}
	}
	flush()
	if len(out) == 0 {
		return nil, fmt.Errorf("plan has no ## tasks")
	}
	return dedupeSlugs(out), nil
}

func dedupeSlugs(tasks []planTask) []planTask {
	used := make(map[string]int, len(tasks))
	for i := range tasks {
		slug := taskSlug(tasks[i].title)
		if n := used[slug]; n > 0 {
			slug = fmt.Sprintf("%s-%d", slug, n+1)
		}
		used[slug]++
		tasks[i].slug = slug
	}
	return tasks
}

func taskSlug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "task"
	}
	return s
}
