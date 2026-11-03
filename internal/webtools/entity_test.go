package webtools

import "testing"

func TestUnescapeNumericEntities(t *testing.T) {
	if got := unescapeEntities("&#39;"); got != "'" {
		t.Fatalf("decimal: %q", got)
	}
	if got := unescapeEntities("&#x27;"); got != "'" {
		t.Fatalf("hex lower: %q", got)
	}
	if got := unescapeEntities("&#X27;"); got != "'" {
		t.Fatalf("hex upper: %q", got)
	}
	if got := unescapeEntities("&#1114112;"); got != "\uFFFD" {
		t.Fatalf("out of range: %q", got)
	}
	if got := unescapeEntities("&#x110000;"); got != "\uFFFD" {
		t.Fatalf("hex out of range: %q", got)
	}
	if got := unescapeEntities("&#xD800;"); got != "\uFFFD" {
		t.Fatalf("surrogate: %q", got)
	}
	if got := unescapeEntities("&#;"); got != "&#;" {
		t.Fatalf("empty numeric stays: %q", got)
	}
}

func TestUnescapeNamedEntities(t *testing.T) {
	cases := map[string]string{
		"&amp;":    "&",
		"&lt;":     "<",
		"&gt;":     ">",
		"&quot;":   "\"",
		"&apos;":   "'",
		"&nbsp;x":  " x",
		"&copy;":   "\u00A9",
		"&reg;":    "\u00AE",
		"&trade;":  "\u2122",
		"&hellip;": "\u2026",
		"&mdash;":  "\u2014",
		"&ndash;":  "\u2013",
		"&lsquo;":  "\u2018",
		"&rsquo;":  "\u2019",
		"&ldquo;":  "\u201C",
		"&rdquo;":  "\u201D",
		"&bull;":   "\u2022",
		"&middot;": "\u00B7",
		"&laquo;":  "\u00AB",
		"&raquo;":  "\u00BB",
		"&times;":  "\u00D7",
	}
	for in, want := range cases {
		if got := unescapeEntities(in); got != want {
			t.Fatalf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestUnescapeUnknownNamed(t *testing.T) {
	in := "before &nope; after"
	want := "before &nope; after"
	if got := unescapeEntities(in); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
