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
	"npm test",
	"pnpm test",
	"bun test",
	"cargo test",
	"cargo build",
	"pytest",
}

var routineMakeTargets = map[string]bool{
	"test": true, "check": true, "build": true,
	"lint": true, "fmt": true, "vet": true,
}

var routineRunScripts = map[string]bool{
	"test": true, "build": true, "lint": true,
	"dev": true, "check": true, "typecheck": true,
}

var findForbidden = []string{
	"-exec", "-execdir", "-ok", "-okdir", "-delete",
	"-fprint", "-fprintf", "-fls",
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
	if makeRoutine(command) {
		return true
	}
	if pkgRunRoutine(command, "npm") || pkgRunRoutine(command, "pnpm") || pkgRunRoutine(command, "bun") {
		return true
	}
	for _, prefix := range routineCommands {
		if !commandMatchesPrefix(command, prefix) {
			continue
		}
		if prefix == "find" && findUnsafe(command) {
			return false
		}
		return true
	}
	return false
}

func makeRoutine(command string) bool {
	if command == "make" {
		return true
	}
	const head = "make "
	if !strings.HasPrefix(command, head) {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(command[len(head):]))
	if len(fields) != 1 {
		return false
	}
	return routineMakeTargets[fields[0]]
}

func pkgRunRoutine(command, pkg string) bool {
	head := pkg + " run "
	if !strings.HasPrefix(command, head) {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(command[len(head):]))
	if len(fields) != 1 {
		return false
	}
	return routineRunScripts[fields[0]]
}

func findUnsafe(command string) bool {
	fields := strings.Fields(command)
	for _, f := range fields {
		for _, bad := range findForbidden {
			if f == bad || strings.HasPrefix(f, bad+"=") {
				return true
			}
		}
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
