package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

const keyUsage = "usage: hp key set NAME | hp key list | hp key rm NAME"

func checkKeyName(name string) error {
	if !keyNamePattern.MatchString(name) {
		return usagef("invalid key name %q: use A-Z, 0-9 and _", name)
	}
	return nil
}

func checkKeyValue(value string) error {
	if value == "" {
		return usagef("key value must not be empty")
	}
	if strings.ContainsAny(value, "\n\r\x00") {
		return usagef("key value must not contain a newline, carriage return or NUL")
	}
	return nil
}

func cmdKey(env Env, args []string) int {
	if len(args) == 0 {
		return exitUsage(env, usagef("%s", keyUsage))
	}
	switch args[0] {
	case "set":
		return cmdKeySet(env, args[1:])
	case "list":
		return cmdKeyList(env, args[1:])
	case "rm":
		return cmdKeyRemove(env, args[1:])
	default:
		return exitUsage(env, usagef("unknown key command %q\n%s", args[0], keyUsage))
	}
}

func cmdKeySet(env Env, args []string) int {
	if len(args) != 1 {
		return exitUsage(env, usagef("usage: hp key set NAME"))
	}
	name := args[0]
	if err := checkKeyName(name); err != nil {
		return exitUsage(env, err)
	}
	value, err := readKeyValue(env, name)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	if err := checkKeyValue(value); err != nil {
		return exitUsage(env, err)
	}
	file, err := keyFilePath(rawGetenv(env))
	if err != nil {
		return exitErr(env, "", "", err)
	}
	if err := keySet(file, name, value); err != nil {
		return exitErr(env, "", "", err)
	}
	fmt.Fprintf(env.Stdout, "set %s\n", name)
	return 0
}

func readKeyValue(env Env, name string) (string, error) {
	if env.Term == nil {
		return "", errors.New("no terminal available to read the key")
	}
	if env.Term.Interactive() {
		fmt.Fprintf(env.Stderr, "value for %s: ", name)
		value, err := env.Term.ReadSecret()
		fmt.Fprintln(env.Stderr)
		return value, err
	}
	return readKeyLine(env.Term)
}

func readKeyLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	return strings.TrimSuffix(line, "\r"), nil
}

func cmdKeyList(env Env, args []string) int {
	if len(args) != 0 {
		return exitUsage(env, usagef("usage: hp key list"))
	}
	getenv := rawGetenv(env)
	file, err := keyFilePath(getenv)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	kf, _, err := readKeyFile(file)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	for _, name := range keyListNames(kf, getenv) {
		fmt.Fprintf(env.Stdout, "%-24s %s\n", name, keySource(name, getenv))
	}
	return 0
}

func keySource(name string, getenv func(string) string) string {
	if getenv(name) != "" {
		return "env"
	}
	return "file"
}

func cmdKeyRemove(env Env, args []string) int {
	if len(args) != 1 {
		return exitUsage(env, usagef("usage: hp key rm NAME"))
	}
	name := args[0]
	if err := checkKeyName(name); err != nil {
		return exitUsage(env, err)
	}
	file, err := keyFilePath(rawGetenv(env))
	if err != nil {
		return exitErr(env, "", "", err)
	}
	removed, err := keyRemove(file, name)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	if !removed {
		return exitErr(env, "", "", fmt.Errorf("%s is not in %s", name, file))
	}
	fmt.Fprintf(env.Stdout, "removed %s\n", name)
	return 0
}
