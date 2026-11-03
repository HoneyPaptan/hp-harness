package permit

import "strings"

const expandChars = "{}*?["

func shellWords(command string) ([]string, bool) {
	var words []string
	var cur strings.Builder
	var quote rune
	inWord := false
	for _, r := range command {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case strings.ContainsRune(expandChars, r):
			return nil, false
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, true
}
