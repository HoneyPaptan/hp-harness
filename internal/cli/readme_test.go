package cli

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var readmeCommandRow = regexp.MustCompile("(?m)^\\| `hp ([a-z]+)`")

func TestReadmeListsEveryCommand(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	var documented []string
	for _, m := range readmeCommandRow.FindAllStringSubmatch(string(data), -1) {
		documented = append(documented, m[1])
	}
	var known []string
	for _, c := range commandHelps {
		known = append(known, c.name)
		if !slices.Contains(documented, c.name) {
			t.Errorf("README has no row for hp %s", c.name)
		}
	}
	for _, name := range documented {
		if !slices.Contains(known, name) {
			t.Errorf("README documents hp %s, which hp --help does not list", name)
		}
	}
	if !strings.Contains(string(data), "hp -c") {
		t.Error("README does not mention hp -c")
	}
}

func TestReadmeNumbersMatchTheBuild(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if want := fmt.Sprintf("%d provider presets", len(presets)); !strings.Contains(text, want) {
		t.Errorf("README does not say %q", want)
	}
	for _, file := range []string{".size-budget", ".wasm-size-budget"} {
		raw, err := os.ReadFile("../../" + file)
		if err != nil {
			t.Fatal(err)
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		if want := humanInt(n); !strings.Contains(text, want+" ") && !strings.Contains(text, want+"`") && !strings.Contains(text, want+")") {
			t.Errorf("README does not state the %s value %s", file, want)
		}
	}
}

func humanInt(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
