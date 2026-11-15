package prompt

import (
	"fmt"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/skill"
)

type Instruction struct {
	Path string
	Text string
}

type Info struct {
	Dir          string
	Date         time.Time
	Skills       []skill.Skill
	Instructions []Instruction
	Browser      bool
}

func Build(in Info) string {
	var b strings.Builder
	if in.Browser {
		b.WriteString(browserText)
	} else {
		b.WriteString(baseText)
	}
	fmt.Fprintf(&b, "\nWorking directory: %s\nDate: %s\n", in.Dir, in.Date.Format("2006-01-02"))
	if len(in.Skills) > 0 {
		b.WriteString("\nSkills (call the skill tool with a name before doing a task it covers):\n")
		for _, sk := range in.Skills {
			fmt.Fprintf(&b, "- %s: %s\n", sk.Name, cutDesc(sk.Description, 200))
		}
	}
	if len(in.Instructions) > 0 {
		b.WriteString("\n\nProject instructions (from AGENTS.md files; the user's messages take priority, and a file closer to the working directory wins on conflicts):\n")
		for _, ins := range in.Instructions {
			fmt.Fprintf(&b, "<instructions path=\"%s\">\n%s\n</instructions>\n", ins.Path, ins.Text)
		}
	}
	return b.String()
}

const baseText = `You are hp, a coding agent in a terminal. Use the tools to read, change and run code in the working directory. Read a file before editing it, prefer small exact edits, and run commands or tests to check your work. Keep replies short: say what you changed and why. When the task is done, reply with text only.`

const browserText = `You are hp, an agent running in a web browser. Use the tools to read and change the files in the working directory. Read a file before editing it and prefer small exact edits. There is no shell, so you cannot run commands or tests; check your work by reading what you wrote. Finish the whole task without asking for confirmation unless the request is truly ambiguous. When the user asks for a document, report, plan, guide, summary or any other long writeup, write it as a Markdown file with the write tool (under docs/ unless they name a path) and reply with two or three sentences on what it holds; never paste the whole document into the reply. Use web_search when you need facts you are unsure of and name the pages you used. Keep replies short. When the task is done, reply with text only.`

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
