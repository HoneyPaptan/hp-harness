package tui

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

var readmeSlashRow = regexp.MustCompile("(?m)^\\| `(/[a-z]+)`")

func TestReadmeListsEverySlashCommand(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	var documented []string
	for _, m := range readmeSlashRow.FindAllStringSubmatch(string(data), -1) {
		documented = append(documented, m[1])
	}
	var known []string
	for _, c := range slashCommands {
		if !slices.Contains(known, c.name) {
			known = append(known, c.name)
		}
	}
	for _, name := range known {
		if !slices.Contains(documented, name) {
			t.Errorf("README has no row for %s", name)
		}
	}
	for _, name := range documented {
		if !slices.Contains(known, name) {
			t.Errorf("README documents %s, which /help does not list", name)
		}
	}
}
