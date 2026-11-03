package permit

import (
	"strings"
)

var routineCommands = []string{
	"git status",
	"git diff",
	"git log",
	"git show",
	"git branch",
	"ls",
	"pwd",
	"cat",
	"head",
	"tail",
	"wc",
	"grep",
	"rg",
	"find",
	"go test",
	"go build",
	"go vet",
	"gofmt -l",
	"make",
	"make test",
	"make check",
	"npm test",
	"npm run",
	"pnpm test",
	"bun test",
	"cargo test",
	"cargo build",
	"pytest",
}

func Routine(a Action, dir, home string) bool {
	switch a.Kind {
	case KindRead, KindNet, KindMeta:
		return true
	case KindWrite:
		return writeRoutine(a.Target, dir, home)
	case KindExec:
		return bashRoutine(a.Target)
	default:
		return false
	}
}

func writeRoutine(target, dir, home string) bool {
	path := resolvePath(dir, target)
	if !insideDir(path, dir) {
		return false
	}
	return !Sensitive(path, dir, home)
}

func bashRoutine(command string) bool {
	if shellMeta(command) {
		return false
	}
	command = strings.TrimSpace(command)
	for _, prefix := range routineCommands {
		if !commandMatchesPrefix(command, prefix) {
			continue
		}
		if prefix == "find" && (strings.Contains(command, "-exec") || strings.Contains(command, "-delete")) {
			return false
		}
		return true
	}
	return false
}

func commandMatchesPrefix(command, prefix string) bool {
	return command == prefix || strings.HasPrefix(command, prefix+" ")
}

func shellMeta(command string) bool {
	if strings.ContainsAny(command, ";\n|&><`") {
		return true
	}
	return strings.Contains(command, "&&") || strings.Contains(command, "||") || strings.Contains(command, "$(")
}
