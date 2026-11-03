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

var execForbiddenPrefix = map[string][]string{
	"rg":   {"--pre", "-z", "--search-zip", "--pre-glob"},
	"grep": {"--pre", "--pre-glob"},
	"go":   {"-exec", "-toolexec", "-o", "-ldflags", "-gcflags", "-asmflags", "-overlay", "-modfile"},
	"find": {"-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprintf", "-fls"},
}

var gitForbiddenPrefix = []string{
	"--ext-diff", "--textconv", "-c", "--exec-path", "--output=", "--exec-path=",
}

var gitLogShowForbidden = []string{"-o", "--output", "--output="}

var gitBranchForbidden = []string{
	"-D", "-d", "--delete", "-m", "-M", "--move", "-c", "-C", "--copy",
	"-f", "--force", "--set-upstream-to", "-u",
}

func Routine(a Action, dir, home string) bool {
	switch a.Kind {
	case KindRead, KindNet, KindMeta:
		return true
	case KindWrite:
		if a.Linked {
			return false
		}
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
	words, ok := shellWords(command)
	if !ok {
		return false
	}
	command = strings.Join(words, " ")
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
		if execUnsafe(prefix, command) {
			return false
		}
		return true
	}
	return false
}

func execUnsafe(prefix, command string) bool {
	if strings.Contains(command, "$") {
		return true
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	head := fields[0]
	if prefs, ok := execForbiddenPrefix[head]; ok {
		if fieldForbidden(fields[1:], prefs) {
			return true
		}
	}
	if head != "git" {
		return false
	}
	sub := strings.TrimPrefix(prefix, "git ")
	if sub == prefix {
		sub = ""
	}
	if gitExecUnsafe(sub, fields[1:]) {
		return true
	}
	return false
}

func gitExecUnsafe(sub string, args []string) bool {
	if fieldForbidden(args, gitForbiddenPrefix) {
		return true
	}
	if sub == "log" || sub == "show" {
		if fieldForbidden(args, gitLogShowForbidden) {
			return true
		}
	}
	if sub == "branch" && fieldForbidden(args, gitBranchForbidden) {
		return true
	}
	return false
}

func fieldForbidden(args, prefixes []string) bool {
	for _, a := range args {
		for _, p := range prefixes {
			if a == p || strings.HasPrefix(a, p) {
				return true
			}
		}
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

func commandMatchesPrefix(command, prefix string) bool {
	return command == prefix || strings.HasPrefix(command, prefix+" ")
}

func shellMeta(command string) bool {
	if strings.ContainsAny(command, ";\n|&><`\\") {
		return true
	}
	return strings.Contains(command, "&&") || strings.Contains(command, "||") || strings.Contains(command, "$(")
}
