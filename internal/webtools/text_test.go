package webtools

import (
	"strings"
	"testing"
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

func TestHTMLToTextBlockBreaks(t *testing.T) {
	_, body := htmlToText([]byte("<div>one</div><div>two</div>"))
	if !strings.Contains(body, "one") || !strings.Contains(body, "two") {
		t.Fatalf("body = %q", body)
	}
	if strings.Count(body, "\n\n") > 0 {
		t.Fatalf("double blank lines: %q", body)
	}
}
