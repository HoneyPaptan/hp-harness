package prompt

import (
	"fmt"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/skill"
)

type Info struct {
	Dir    string
	Date   time.Time
	Skills []skill.Skill
}

func Build(in Info) string {
	var b strings.Builder
	b.WriteString(baseText)
	fmt.Fprintf(&b, "\nWorking directory: %s\nDate: %s\n", in.Dir, in.Date.Format("2006-01-02"))
	if len(in.Skills) > 0 {
		b.WriteString("\nSkills (call the skill tool with a name before doing a task it covers):\n")
		for _, sk := range in.Skills {
			fmt.Fprintf(&b, "- %s: %s\n", sk.Name, cutDesc(sk.Description, 200))
		}
	}
	return b.String()
}

const baseText = `You are hp, a coding agent in a terminal. Use the tools to read, change and run code in the working directory. Read a file before editing it, prefer small exact edits, and run commands or tests to check your work. Keep replies short: say what you changed and why. When the task is done, reply with text only.`

func cutDesc(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	n := max - 3
	if n < 0 {
		n = 0
	}
	return string(r[:n]) + "..."
}
