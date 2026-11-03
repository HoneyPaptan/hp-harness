package permit

import "fmt"

func Decide(mode Mode, rules []Rule, grants *Grants, a Action, dir, home string) (Verdict, string) {
	if mode == ModeFull {
		return Allow, "full access"
	}
	if v, reason, ok := decideRules(rules, a); ok {
		return v, reason
	}
	if grants != nil && grants.Has(a) {
		return Allow, "granted"
	}
	if routineVerdict(mode, a, dir, home) {
		return Allow, "routine"
	}
	if mode == ModeAsk {
		return Ask, "needs approval"
	}
	return Review, "needs review"
}

func decideRules(rules []Rule, a Action) (Verdict, string, bool) {
	var winner *Rule
	for i := range rules {
		r := &rules[i]
		if !ruleToolMatch(r.Tool, a.Tool) {
			continue
		}
		if !Match(r.Pattern, a.Target) {
			continue
		}
		winner = r
	}
	if winner == nil {
		return "", "", false
	}
	switch winner.Verdict {
	case Deny:
		return Deny, fmt.Sprintf("denied by rule %s %s", winner.Tool, winner.Pattern), true
	case Ask:
		return Ask, fmt.Sprintf("asked by rule %s %s", winner.Tool, winner.Pattern), true
	default:
		return Allow, fmt.Sprintf("allowed by rule %s %s", winner.Tool, winner.Pattern), true
	}
}

func ruleToolMatch(ruleTool, actionTool string) bool {
	if ruleTool == "*" {
		return true
	}
	return ruleTool == actionTool
}

func routineVerdict(mode Mode, a Action, dir, home string) bool {
	if !Routine(a, dir, home) {
		return false
	}
	if mode == ModeAuto {
		return true
	}
	if mode == ModeAsk && (a.Kind == KindRead || a.Kind == KindMeta) {
		return true
	}
	return false
}
