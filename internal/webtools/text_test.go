package webtools

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const sampleHTML = `<!DOCTYPE html>
<html>
<head>
<title>Golden Title</title>
<script>ignored();</script>
<style>.hide{display:none}</style>
<noscript>no js</noscript>
</head>
<body>
<p>First paragraph.</p>
<p>Second with &amp; ampersand.</p>
<ul>
<li>alpha</li>
<li>beta</li>
</ul>
<pre>  keep  spaces
second line</pre>
</body>
</html>`

const wantGoldenBody = `First paragraph.
Second with & ampersand.
alpha
beta
  keep  spaces
second line`

func TestHTMLToTextGolden(t *testing.T) {
	title, body := htmlToText([]byte(sampleHTML))
	if title != "Golden Title" {
		t.Fatalf("title = %q, want Golden Title", title)
	}
	if body != wantGoldenBody {
		t.Fatalf("body mismatch:\n%s", diffLines(body, wantGoldenBody))
	}
}

func diffLines(got, want string) string {
	return "got:\n" + got + "\nwant:\n" + want
}

func TestTrimUTF8BytesMultibyte(t *testing.T) {
	s := strings.Repeat("日", 100)
	got := trimUTF8Bytes(s, 10)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid utf-8: %q", got)
	}
	if len(got) > 10 {
		t.Fatalf("len %d want <= 10", len(got))
	}
	wantRunes := 10 / 3
	if len([]rune(got)) != wantRunes {
		t.Fatalf("runes %d want %d", len([]rune(got)), wantRunes)
	}
}

func TestHTMLToTextBlockBreaks(t *testing.T) {
	_, body := htmlToText([]byte("<div>one</div><div>two</div>"))
	if !strings.Contains(body, "one") || !strings.Contains(body, "two") {
		t.Fatalf("body = %q", body)
	}
	if strings.Count(body, "\n\n") > 0 {
		t.Fatalf("double blank lines: %q", body)
	}
}
