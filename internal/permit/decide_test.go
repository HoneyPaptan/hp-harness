package permit

import (
	"encoding/json"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern, s string
		want       bool
	}{
		{"*", "anything goes/here", true},
		{"", "", true},
		{"", "x", false},
		{"docs/*", "docs/a.md", true},
		{"docs/*", "other/docs/a.md", false},
		{"git push *", "git push origin main", true},
		{"git push *", "git pull origin main", false},
		{"exact", "exact", true},
		{"exact", "exactx", false},
	}
	for _, tc := range tests {
		got := Match(tc.pattern, tc.s)
		if got != tc.want {
			t.Errorf("Match(%q,%q)=%v want %v", tc.pattern, tc.s, got, tc.want)
		}
	}
}

func TestParseRulesOrder(t *testing.T) {
	raw := json.RawMessage(`{"*": "ask", "bash": {"git *": "allow", "git push *": "deny"}, "edit": {"*": "deny", "docs/*": "allow"}}`)
	rules, err := ParseRules(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []Rule{
		{Tool: "*", Pattern: "*", Verdict: Ask},
		{Tool: "bash", Pattern: "git *", Verdict: Allow},
		{Tool: "bash", Pattern: "git push *", Verdict: Deny},
		{Tool: "edit", Pattern: "*", Verdict: Deny},
		{Tool: "edit", Pattern: "docs/*", Verdict: Allow},
	}
	if len(rules) != len(want) {
		t.Fatalf("len %d want %d", len(rules), len(want))
	}
	for i := range want {
		if rules[i] != want[i] {
			t.Fatalf("rules[%d]=%+v want %+v", i, rules[i], want[i])
		}
	}
}

func TestParseRulesBadVerdict(t *testing.T) {
	_, err := ParseRules(json.RawMessage(`{"bash": {"x": "maybe"}}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseMode(t *testing.T) {
	for _, s := range []string{"ask", "auto", "full-access"} {
		if _, err := ParseMode(s); err != nil {
			t.Fatalf("ParseMode(%q): %v", s, err)
		}
	}
	if _, err := ParseMode("yolo"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecideFullAccess(t *testing.T) {
	a := Action{Tool: "bash", Target: "rm -rf /", Kind: KindExec}
	v, _ := Decide(ModeFull, nil, nil, a, "/w", "/home/u")
	if v != Allow {
		t.Fatalf("got %q", v)
	}
}

func TestDecideDenyBeatsRoutine(t *testing.T) {
	rules := []Rule{{Tool: "bash", Pattern: "*", Verdict: Deny}}
	a := Action{Tool: "bash", Target: "git status", Kind: KindExec}
	v, reason := Decide(ModeAuto, rules, nil, a, "/w", "/home/u")
	if v != Deny || reason != "denied by rule bash *" {
		t.Fatalf("got %q %q", v, reason)
	}
}

func TestDecideLaterRuleWins(t *testing.T) {
	rules := []Rule{
		{Tool: "bash", Pattern: "git *", Verdict: Allow},
		{Tool: "bash", Pattern: "git push *", Verdict: Deny},
	}
	a := Action{Tool: "bash", Target: "git push origin", Kind: KindExec}
	v, _ := Decide(ModeAuto, rules, nil, a, "/w", "/home/u")
	if v != Deny {
		t.Fatalf("got %q", v)
	}
	a = Action{Tool: "bash", Target: "git status", Kind: KindExec}
	v, _ = Decide(ModeAuto, rules, nil, a, "/w", "/home/u")
	if v != Allow {
		t.Fatalf("got %q", v)
	}
}

func TestDecideDenyBeatsGrant(t *testing.T) {
	rules := []Rule{{Tool: "bash", Pattern: "*", Verdict: Deny}}
	a := Action{Tool: "bash", Target: "git push", Kind: KindExec}
	g := &Grants{}
	g.Add(a)
	v, _ := Decide(ModeAuto, rules, g, a, "/w", "/home/u")
	if v != Deny {
		t.Fatalf("got %q", v)
	}
}

func TestDecideGrantAllowsReviewed(t *testing.T) {
	a := Action{Tool: "bash", Target: "git push", Kind: KindExec}
	g := &Grants{}
	g.Add(a)
	v, _ := Decide(ModeAuto, nil, g, a, "/w", "/home/u")
	if v != Allow {
		t.Fatalf("got %q", v)
	}
}

func TestDecideAskModeReadAndWrite(t *testing.T) {
	dir := "/w"
	home := "/home/u"
	read := Action{Tool: "read", Target: "README.md", Kind: KindRead}
	v, _ := Decide(ModeAsk, nil, nil, read, dir, home)
	if v != Allow {
		t.Fatalf("read got %q", v)
	}
	write := Action{Tool: "write", Target: "internal/x.go", Kind: KindWrite}
	v, _ = Decide(ModeAsk, nil, nil, write, dir, home)
	if v != Ask {
		t.Fatalf("write got %q", v)
	}
}

func TestDecideAutoReview(t *testing.T) {
	a := Action{Tool: "bash", Target: "git push", Kind: KindExec}
	v, _ := Decide(ModeAuto, nil, nil, a, "/w", "/home/u")
	if v != Review {
		t.Fatalf("got %q", v)
	}
}

func TestDecideRuleAskInAuto(t *testing.T) {
	rules := []Rule{{Tool: "*", Pattern: "*", Verdict: Ask}}
	a := Action{Tool: "bash", Target: "git push", Kind: KindExec}
	v, _ := Decide(ModeAuto, rules, nil, a, "/w", "/home/u")
	if v != Ask {
		t.Fatalf("got %q want ask", v)
	}
}

func TestDecideMCPRule(t *testing.T) {
	rules := []Rule{{Tool: "mcp", Pattern: "slack__*", Verdict: Deny}}
	a := Action{Tool: "mcp", Target: "slack__post", Kind: KindExternal}
	v, _ := Decide(ModeAuto, rules, nil, a, "/w", "/home/u")
	if v != Deny {
		t.Fatalf("got %q", v)
	}
}

func TestDecideWebFetchRules(t *testing.T) {
	rules := []Rule{{Tool: "web_fetch", Pattern: "https://pkg.go.dev/*", Verdict: Allow}}
	allowed := Action{Tool: "web_fetch", Target: "https://pkg.go.dev/std", Kind: KindExternal}
	v, _ := Decide(ModeAuto, rules, nil, allowed, "/w", "/home/u")
	if v != Allow {
		t.Fatalf("allowed got %q", v)
	}
	other := Action{Tool: "web_fetch", Target: "https://example.com/", Kind: KindExternal}
	v, _ = Decide(ModeAuto, rules, nil, other, "/w", "/home/u")
	if v != Review {
		t.Fatalf("other got %q", v)
	}
}

func TestDecideWebSearchAllowAll(t *testing.T) {
	rules := []Rule{{Tool: "web_search", Pattern: "*", Verdict: Allow}}
	a := Action{Tool: "web_search", Target: "anything", Kind: KindExternal}
	v, _ := Decide(ModeAuto, rules, nil, a, "/w", "/home/u")
	if v != Allow {
		t.Fatalf("got %q", v)
	}
}

func TestParseRulesWebToolNames(t *testing.T) {
	raw := json.RawMessage(`{"web_fetch": {"https://pkg.go.dev/*": "allow"}, "web_search": "allow"}`)
	rules, err := ParseRules(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("len %d", len(rules))
	}
}

func TestGrantsExactMatch(t *testing.T) {
	g := &Grants{}
	g.Add(Action{Tool: "bash", Target: "git push", Kind: KindExec})
	a := Action{Tool: "bash", Target: "git push origin", Kind: KindExec}
	if g.Has(a) {
		t.Fatal("target must match exactly")
	}
}
