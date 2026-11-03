package tui

import (
	"strings"
	"testing"
)

func TestDraw(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		text   string
		cursor int
		want   string
	}{
		{"empty", 80, "", 0, "> \r\x1b[2C"},
		{"plain", 80, "hi", 2, "> hi\r\x1b[4C"},
		{"cursor mid", 80, "hello", 2, "> hello\r\x1b[4C"},
		{"cursor at start", 80, "hello", 0, "> hello\r\x1b[2C"},
		{"one cell free", 5, "ab", 2, "> ab\r\x1b[4C"},
		{"fills width cursor wraps", 4, "ab", 2, "> ab\r\n\r"},
		{"fills width cursor mid", 4, "ab", 1, "> ab\r\x1b[3C"},
		{"one past width", 4, "abc", 3, "> ab\r\nc\r\x1b[1C"},
		{"cursor first cell of wrap", 4, "abc", 2, "> ab\r\nc\r"},
		{"cursor above wrap", 4, "abc", 1, "> ab\r\nc\x1b[1A\r\x1b[3C"},
		{"two full rows", 4, "abcdef", 6, "> ab\r\ncdef\r\n\r"},
		{"default width", 0, "hi", 2, "> hi\r\x1b[4C"},
		{"wide cursor end", 80, "日本", 2, "> 日本\r\x1b[6C"},
		{"wide cursor mid", 80, "日本", 1, "> 日本\r\x1b[4C"},
		{"wide fills width", 6, "日本語", 3, "> 日本\r\n語\r\x1b[2C"},
		{"wide leaves blank cell", 5, "日本語", 3, "> 日\r\n本語\r\x1b[4C"},
		{"cursor on wrapped wide", 5, "日本", 1, "> 日\r\n本\r"},
		{"emoji", 80, "😀a", 2, "> 😀a\r\x1b[5C"},
		{"combining", 80, "éx", 3, "> éx\r\x1b[4C"},
		{"combining at width stays", 3, "é", 2, "> é\r\n\r"},
		{"multi line end", 80, "ab\ncd", 5, "> ab\r\ncd\r\x1b[2C"},
		{"multi line first row", 80, "ab\ncd", 1, "> ab\r\ncd\x1b[1A\r\x1b[3C"},
		{"multi line second row", 80, "ab\ncd\nef", 4, "> ab\r\ncd\r\nef\x1b[1A\r\x1b[1C"},
		{"trailing newline", 80, "ab\n", 3, "> ab\r\n\r"},
		{"blank line", 80, "a\n\nb", 2, "> a\r\n\r\nb\x1b[1A\r"},
		{"newline at full row", 4, "ab\ncd", 5, "> ab\r\ncd\r\x1b[2C"},
		{"wrap inside line", 4, "abcd\nx", 6, "> ab\r\ncd\r\nx\r\x1b[1C"},
		{"escape shown not sent", 80, "a\x1b[2J", 5, "> a^[[2J\r\x1b[8C"},
		{"tab expands", 80, "a\tb", 3, "> a    b\r\x1b[8C"},
		{"delete shown", 80, "\x7f", 1, "> ^?\r\x1b[4C"},
		{"c1 replaced", 80, "\u009b", 1, "> �\r\x1b[3C"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			if err := NewRenderer(&out, tc.width).Draw("> ", []rune(tc.text), tc.cursor, ""); err != nil {
				t.Fatal(err)
			}
			want := "\r\x1b[J" + tc.want
			if got := out.String(); got != want {
				t.Fatalf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestDrawFooter(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		text   string
		cursor int
		footer string
		want   string
	}{
		{"width 100", 100, "hi", 2, "groq/openai/gpt-oss-120b · jev off",
			"> hi\r\n\r\ngroq/openai/gpt-oss-120b · jev off\x1b[2A\r\x1b[4C"},
		{"width 40", 40, "hi", 2, "xai/grok · jev",
			"> hi\r\n\r\nxai/grok · jev\x1b[2A\r\x1b[4C"},
		{"footer cut", 10, "", 0, "abc · jev off",
			"> \r\n\r\nabc · jev…\x1b[2A\r\x1b[2C"},
		{"footer at width", 10, "", 0, "abc defghi",
			"> \r\n\r\nabc defghi\x1b[2A\r\x1b[2C"},
		{"multi row input", 80, "ab\ncd", 5, "m · jev",
			"> ab\r\ncd\r\n\r\nm · jev\x1b[2A\r\x1b[2C"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			if err := NewRenderer(&out, tc.width).Draw("> ", []rune(tc.text), tc.cursor, tc.footer); err != nil {
				t.Fatal(err)
			}
			want := "\r\x1b[J" + tc.want
			if got := out.String(); got != want {
				t.Fatalf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestDrawMenu(t *testing.T) {
	footer := "groq/openai/gpt-oss-120b · jev off"
	tests := []struct {
		name     string
		width    int
		text     string
		cursor   int
		rows     []string
		selected int
		more     string
		want     string
	}{
		{
			"width 40 filter typed",
			40,
			"gr",
			2,
			[]string{"groq"},
			0,
			pickerHint,
			"provider: gr\r\n› groq\r\n↑ ↓ Enter, Esc cancels\r\n\r\ngroq/openai/gpt-oss-120b · jev off\x1b[4A\r\x1b[12C",
		},
		{
			"width 100 selected row",
			100,
			"",
			0,
			[]string{"groq", "openrouter", "ollama"},
			1,
			pickerHint,
			"provider: \r\n  groq\r\n› openrouter\r\n  ollama\r\n↑ ↓ Enter, Esc cancels\r\n\r\ngroq/openai/gpt-oss-120b · jev off\x1b[6A\r\x1b[10C",
		},
		{
			"width 40 more line",
			40,
			"",
			0,
			[]string{"a", "b", "c", "d", "e", "f", "g", "h"},
			0,
			"+4 more · " + pickerHint,
			"provider: \r\n› a\r\n  b\r\n  c\r\n  d\r\n  e\r\n  f\r\n  g\r\n  h\r\n+4 more · ↑ ↓ Enter, Esc cancels\r\n\r\ngroq/openai/gpt-oss-120b · jev off\x1b[11A\r\x1b[10C",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			r := NewRenderer(&out, tc.width)
			if err := r.DrawMenu(providerPrompt, []rune(tc.text), tc.cursor, tc.rows, tc.selected, "", tc.more, footer); err != nil {
				t.Fatal(err)
			}
			want := "\r\x1b[J" + tc.want
			if got := out.String(); got != want {
				t.Fatalf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestDrawModelMenuFrames(t *testing.T) {
	footer := "groq/openai/gpt-oss-120b · jev off"
	tests := []struct {
		name string
		note string
		more string
		text string
		cur  int
		rows []string
		sel  int
		want string
	}{
		{
			"loading width 40",
			"",
			pickerLoading,
			"",
			0,
			nil,
			-1,
			"model for groq: \r\nloading models\r\n\r\ngroq/openai/gpt-oss-120b · jev off\x1b[3A\r\x1b[16C",
		},
		{
			"listed width 40",
			"",
			pickerHint,
			"gpt",
			3,
			[]string{"openai/gpt-oss-120b"},
			0,
			"model for groq: gpt\r\n› openai/gpt-oss-120b\r\n↑ ↓ Enter, Esc cancels\r\n\r\ngroq/openai/gpt-oss-120b · jev off\x1b[4A\r\x1b[19C",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			r := NewRenderer(&out, 40)
			if err := r.DrawMenu(modelPrompt("groq"), []rune(tc.text), tc.cur, tc.rows, tc.sel, tc.note, tc.more, footer); err != nil {
				t.Fatal(err)
			}
			want := "\r\x1b[J" + tc.want
			if got := out.String(); got != want {
				t.Fatalf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestDrawMenuClearErasesEverything(t *testing.T) {
	var out strings.Builder
	r := NewRenderer(&out, 40)
	_ = r.DrawMenu(providerPrompt, nil, 0, []string{"groq", "openrouter", "ollama"}, 0, "", pickerHint, "groq/openai/gpt-oss-120b · jev off")
	out.Reset()
	if err := r.Clear(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "\r\x1b[J"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDrawFooterIsDim(t *testing.T) {
	var out strings.Builder
	r := NewRenderer(&out, 80)
	r.SetColor(true)
	if err := r.Draw("> ", []rune("hi"), 2, "m · jev"); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "\r\x1b[J> hi\r\n\r\n"+sgrDim+"m · jev"+sgrReset+"\x1b[2A\r\x1b[4C"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFinishErasesFooter(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cursor int
		want   string
	}{
		{"single row", "a", 1, "\r\n\x1b[J"},
		{"cursor above last row", "a\nb\nc", 1, "\x1b[2B\r\n\x1b[J"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			r := NewRenderer(&out, 80)
			r.Draw("> ", []rune(tc.text), tc.cursor, "m · jev")
			out.Reset()
			if err := r.Finish(); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			out.Reset()
			r.Draw("> ", nil, 0, "")
			if got, want := out.String(), "\r\x1b[J> \r\x1b[2C"; got != want {
				t.Fatalf("after finish got %q want %q", got, want)
			}
		})
	}
}

func TestClearErasesFooter(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cursor int
		want   string
	}{
		{"cursor on first row", "a", 1, "\r\x1b[J"},
		{"cursor on last prompt row", "a\nb", 2, "\x1b[1A\r\x1b[J"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			r := NewRenderer(&out, 80)
			r.Draw("> ", []rune(tc.text), tc.cursor, "m · jev")
			out.Reset()
			if err := r.Clear(); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestDrawOutputIsInline(t *testing.T) {
	var out strings.Builder
	r := NewRenderer(&out, 4)
	r.Draw("> ", []rune("abc\ndef"), 1, "")
	r.Finish()
	for _, banned := range []string{"\x1b[?1049", "\x1b[?47", "\x1b[2J", "\x1b[H"} {
		if strings.Contains(out.String(), banned) {
			t.Fatalf("output has %q: %q", banned, out.String())
		}
	}
}

func TestRedraw(t *testing.T) {
	tests := []struct {
		name         string
		text, second string
		cursor       int
		want         string
	}{
		{"same row", "a", "ab", 1, "\r\x1b[J> ab\r\n\r"},
		{"from wrapped row", "abc", "abcd", 3, "\x1b[1A\r\x1b[J> ab\r\ncd\r\x1b[2C"},
		{"from second line", "a\nb\nc", "x", 3, "\x1b[1A\r\x1b[J> x\r\x1b[3C"},
		{"shrinks from cursor on last", "a\nb\nc", "", 5, "\x1b[2A\r\x1b[J> \r\x1b[2C"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			r := NewRenderer(&out, 4)
			r.Draw("> ", []rune(tc.text), tc.cursor, "")
			out.Reset()
			r.Draw("> ", []rune(tc.second), len(tc.second), "")
			if got := out.String(); got != tc.want {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestFinish(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cursor int
		want   string
	}{
		{"single row", "a", 1, "\r\n"},
		{"cursor on last row", "a\nb", 3, "\r\n"},
		{"cursor above last row", "a\nb\nc", 1, "\x1b[2B\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			r := NewRenderer(&out, 80)
			r.Draw("> ", []rune(tc.text), tc.cursor, "")
			out.Reset()
			if err := r.Finish(); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			out.Reset()
			r.Draw("> ", nil, 0, "")
			if got, want := out.String(), "\r\x1b[J> \r\x1b[2C"; got != want {
				t.Fatalf("after finish got %q want %q", got, want)
			}
		})
	}
}

func TestSetWidth(t *testing.T) {
	var out strings.Builder
	r := NewRenderer(&out, 80)
	r.SetWidth(4)
	r.Draw("> ", []rune("abc"), 3, "")
	if got, want := out.String(), "\r\x1b[J> ab\r\nc\r\x1b[1C"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBracketedPaste(t *testing.T) {
	tests := []struct {
		name  string
		write func(*strings.Builder) error
		want  string
	}{
		{"enable", func(b *strings.Builder) error { return EnableBracketedPaste(b) }, "\x1b[?2004h"},
		{"disable", func(b *strings.Builder) error { return DisableBracketedPaste(b) }, "\x1b[?2004l"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			if err := tc.write(&out); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Fatalf("got %q", out.String())
			}
		})
	}
}

func TestRuneWidth(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want int
	}{
		{"ascii", 'a', 1},
		{"latin", 'é', 1},
		{"box drawing", '─', 1},
		{"han", '日', 2},
		{"hiragana", 'あ', 2},
		{"hangul", '한', 2},
		{"fullwidth", 'Ａ', 2},
		{"emoji", '😀', 2},
		{"rocket", '🚀', 2},
		{"sparkles", '✨', 2},
		{"combining acute", '́', 0},
		{"zero width joiner", '‍', 0},
		{"variation selector", '️', 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RuneWidth(tc.r); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}
