package permit

type Verdict string

const (
	Allow  Verdict = "allow"
	Deny   Verdict = "deny"
	Ask    Verdict = "ask"
	Review Verdict = "review"
)

type Rule struct {
	Tool    string
	Pattern string
	Verdict Verdict
}
