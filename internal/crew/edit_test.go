package crew

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEditorArgv(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"visual first", map[string]string{"VISUAL": "nvim", "EDITOR": "nano"}, []string{"nvim"}},
		{"editor next", map[string]string{"EDITOR": "nano"}, []string{"nano"}},
		{"vi last", map[string]string{}, []string{"vi"}},
		{"blank visual falls through", map[string]string{"VISUAL": "  ", "EDITOR": "nano"}, []string{"nano"}},
		{"arguments kept", map[string]string{"VISUAL": "code --wait"}, []string{"code", "--wait"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := editorArgv(func(k string) string { return tc.env[k] })
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv %v want %v", got, tc.want)
			}
		})
	}
}

func editPlanFixture(t *testing.T, body string) (Service, string, *[][]string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	env := Env{Cwd: dir, Getenv: func(k string) string { return map[string]string{"VISUAL": "ed --flag"}[k] }}
	return Service{Env: env}, path, &calls
}

func TestEditPlanRereadsTitles(t *testing.T) {
	cases := []struct {
		name    string
		before  string
		after   string
		editErr error
		titles  []string
		errPart string
	}{
		{"more tasks", "## a\n", "## a\n## b\n", nil, []string{"a", "b"}, ""},
		{"fewer tasks", "## a\n## b\n", "## a\n", nil, []string{"a"}, ""},
		{"invalid plan", "## a\n", "nothing here\n", nil, nil, "no ## tasks"},
		{"editor failed", "## a\n", "## a\n## b\n", errors.New("exit status 1"), []string{"a", "b"}, "ed did not finish"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, path, calls := editPlanFixture(t, tc.before)
			svc.Env.Handover = func(argv []string) error {
				*calls = append(*calls, argv)
				return errors.Join(os.WriteFile(path, []byte(tc.after), 0o600), tc.editErr)
			}
			titles, err := svc.EditPlan("plan.md")
			if !reflect.DeepEqual(titles, tc.titles) {
				t.Fatalf("titles %v want %v", titles, tc.titles)
			}
			if (err == nil) != (tc.errPart == "") || err != nil && !strings.Contains(err.Error(), tc.errPart) {
				t.Fatalf("err %v want %q", err, tc.errPart)
			}
			if want := []string{"ed", "--flag", path}; len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], want) {
				t.Fatalf("handover %v want %v", *calls, want)
			}
		})
	}
}

func TestEditPlanNeedsTerminal(t *testing.T) {
	svc, _, _ := editPlanFixture(t, "## a\n")
	if _, err := svc.EditPlan("plan.md"); err == nil || !strings.Contains(err.Error(), "needs a terminal") {
		t.Fatalf("err %v", err)
	}
}

func TestEditPlanMissingFile(t *testing.T) {
	svc, _, _ := editPlanFixture(t, "## a\n")
	svc.Env.Handover = func([]string) error { t.Fatal("editor ran for a missing file"); return nil }
	if _, err := svc.EditPlan("nope.md"); err == nil {
		t.Fatal("want an error")
	}
}
