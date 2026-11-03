package permit

import (
	"bytes"
	"encoding/json"
	"errors"
)

var errBadVerdict = errors.New("permit: bad verdict")

func ParseRules(raw json.RawMessage) ([]Rule, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errBadRules
	}
	var rules []Rule
	for dec.More() {
		toolTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		tool, ok := toolTok.(string)
		if !ok {
			return nil, errBadRules
		}
		valTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch val := valTok.(type) {
		case string:
			verdict, err := parseRuleVerdict(val)
			if err != nil {
				return nil, err
			}
			rules = append(rules, Rule{Tool: tool, Pattern: "*", Verdict: verdict})
		case json.Delim:
			if val != '{' {
				return nil, errBadRules
			}
			for dec.More() {
				patTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				pattern, ok := patTok.(string)
				if !ok {
					return nil, errBadRules
				}
				verTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				verStr, ok := verTok.(string)
				if !ok {
					return nil, errBadRules
				}
				verdict, err := parseRuleVerdict(verStr)
				if err != nil {
					return nil, err
				}
				rules = append(rules, Rule{Tool: tool, Pattern: pattern, Verdict: verdict})
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
		default:
			return nil, errBadRules
		}
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return rules, nil
}

func parseRuleVerdict(s string) (Verdict, error) {
	switch Verdict(s) {
	case Allow, Deny, Ask:
		return Verdict(s), nil
	default:
		return "", errBadVerdict
	}
}

var errBadRules = errors.New("permit: bad rules")
