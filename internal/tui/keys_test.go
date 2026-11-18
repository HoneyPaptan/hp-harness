package tui

import (
	"slices"
	"testing"
)

func typed(s string) []Key {
	var keys []Key
	for _, r := range s {
		keys = append(keys, runeKey(r))
	}
	return keys
}

func named(types ...KeyType) []Key {
	var keys []Key
	for _, kind := range types {
		keys = append(keys, Key{Type: kind})
	}
	return keys
}

func joined(groups ...[]Key) []Key {
	return slices.Concat(groups...)
}

func TestDecoderSingleRead(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Key
	}{
		{"ascii", "ab", typed("ab")},
		{"utf8", "é日😀", typed("é日😀")},
		{"invalid byte", "a\xffb", typed("a�b")},
		{"carriage return", "\r", named(KeyEnter)},
		{"line feed is ctrl j", "\n", named(KeyCtrlJ)},
		{"delete byte", "\x7f", named(KeyBackspace)},
		{"ctrl h", "\x08", named(KeyBackspace)},
		{"ctrl a", "\x01", named(KeyCtrlA)},
		{"ctrl c", "\x03", named(KeyCtrlC)},
		{"ctrl d", "\x04", named(KeyCtrlD)},
		{"ctrl e", "\x05", named(KeyCtrlE)},
		{"ctrl k", "\x0b", named(KeyCtrlK)},
		{"ctrl l", "\x0c", named(KeyCtrlL)},
		{"ctrl u", "\x15", named(KeyCtrlU)},
		{"ctrl w", "\x17", named(KeyCtrlW)},
		{"tab", "\t", named(KeyTab)},
		{"shift tab", "\x1b[Z", named(KeyShiftTab)},
		{"unbound control dropped", "\x1ca", typed("a")},
		{"ctrl b", "\x02", named(KeyCtrlB)},
		{"ctrl f", "\x06", named(KeyCtrlF)},
		{"ctrl n", "\x0e", named(KeyCtrlN)},
		{"ctrl p", "\x10", named(KeyCtrlP)},
		{"ctrl y", "\x19", named(KeyCtrlY)},
		{"ctrl z", "\x1a", named(KeyCtrlZ)},
		{"alt b", "\x1bb", named(KeyWordLeft)},
		{"alt f", "\x1bf", named(KeyWordRight)},
		{"alt d", "\x1bd", named(KeyWordDelete)},
		{"alt backspace", "\x1b\x7f", named(KeyWordBackspace)},
		{"alt enter", "\x1b\r", named(KeyNewline)},
		{"shift enter as csi u", "\x1b[13;2u", named(KeyNewline)},
		{"shift enter as other keys", "\x1b[27;2;13~", named(KeyNewline)},
		{"plain enter as csi u", "\x1b[13u", named(KeyEnter)},
		{"ctrl backspace as csi u", "\x1b[127;5u", named(KeyWordBackspace)},
		{"alt left", "\x1b[1;3D", named(KeyWordLeft)},
		{"ctrl left", "\x1b[1;5D", named(KeyWordLeft)},
		{"shift right selects", "\x1b[1;2C", []Key{{Type: KeyRight, Shift: true}}},
		{"shift ctrl left selects a word", "\x1b[1;6D", []Key{{Type: KeyWordLeft, Shift: true}}},
		{"shift home selects to the line start", "\x1b[1;2H", []Key{{Type: KeyHome, Shift: true}}},
		{"shift delete is not a selection", "\x1b[3;2~", named(KeyDelete)},
		{"ctrl underscore undoes", "\x1f", named(KeyUndo)},
		{"alt underscore redoes", "\x1b_", named(KeyRedo)},
		{"alt w copies", "\x1bw", named(KeyCopy)},
		{"ctrl o", "\x0f", named(KeyCtrlO)},
		{"ctrl t", "\x14", named(KeyCtrlT)},
		{"ctrl home", "\x1b[1;5H", named(KeyDraftStart)},
		{"ctrl end", "\x1b[1;5F", named(KeyDraftEnd)},
		{"ctrl delete", "\x1b[3;5~", named(KeyWordDelete)},
		{"esc then letter", "\x1bq", joined(named(KeyEsc), typed("q"))},
		{"esc twice then a letter", "\x1b\x1bq", joined(named(KeyEsc, KeyEsc), typed("q"))},
		{"csi up", "\x1b[A", named(KeyUp)},
		{"csi down", "\x1b[B", named(KeyDown)},
		{"csi right", "\x1b[C", named(KeyRight)},
		{"csi left", "\x1b[D", named(KeyLeft)},
		{"csi home", "\x1b[H", named(KeyHome)},
		{"csi end", "\x1b[F", named(KeyEnd)},
		{"ss3 up", "\x1bOA", named(KeyUp)},
		{"ss3 down", "\x1bOB", named(KeyDown)},
		{"ss3 right", "\x1bOC", named(KeyRight)},
		{"ss3 left", "\x1bOD", named(KeyLeft)},
		{"ss3 home", "\x1bOH", named(KeyHome)},
		{"ss3 end", "\x1bOF", named(KeyEnd)},
		{"tilde home 1", "\x1b[1~", named(KeyHome)},
		{"tilde home 7", "\x1b[7~", named(KeyHome)},
		{"tilde end 4", "\x1b[4~", named(KeyEnd)},
		{"tilde end 8", "\x1b[8~", named(KeyEnd)},
		{"tilde delete", "\x1b[3~", named(KeyDelete)},
		{"ctrl right", "\x1b[1;5C", named(KeyWordRight)},
		{"modified delete", "\x1b[3;2~", named(KeyDelete)},
		{"unknown csi dropped", "\x1b[99~a", typed("a")},
		{"unknown final dropped", "\x1b[2Ja", typed("a")},
		{"unknown ss3 dropped", "\x1bOZa", typed("a")},
		{"malformed csi resyncs", "\x1b[1\x03", named(KeyCtrlC)},
		{"esc then rune", "\x1bx", joined(named(KeyEsc), typed("x"))},
		{"double esc then csi", "\x1b\x1b[A", named(KeyEsc, KeyUp)},
		{"sequence between runes", "a\x1b[Db", joined(typed("a"), named(KeyLeft), typed("b"))},
		{"paste", "\x1b[200~hi\x1b[201~", joined(named(KeyPasteStart), typed("hi"), named(KeyPasteEnd))},
		{"paste keeps newline as rune", "\x1b[200~a\nb\r\x1b[201~", joined(named(KeyPasteStart), typed("a\nb\r"), named(KeyPasteEnd))},
		{"paste keeps controls", "\x1b[200~\x03\x1b[A\x1b[201~", joined(named(KeyPasteStart), typed("\x03\x1b[A"), named(KeyPasteEnd))},
		{"typing after paste", "\x1b[200~a\x1b[201~\r", joined(named(KeyPasteStart), typed("a"), named(KeyPasteEnd, KeyEnter))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var d Decoder
			got := d.Feed([]byte(tc.input))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestDecoderEverySplit(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Key
	}{
		{"two byte rune", "é", typed("é")},
		{"three byte rune", "日", typed("日")},
		{"four byte rune", "😀", typed("😀")},
		{"csi", "\x1b[1;5C", named(KeyWordRight)},
		{"alt enter", "\x1b\r", named(KeyNewline)},
		{"ss3", "\x1bOH", named(KeyHome)},
		{"paste with rune", "\x1b[200~日\x1b\n\x1b[201~", joined(named(KeyPasteStart), typed("日\x1b\n"), named(KeyPasteEnd))},
	}
	for _, tc := range tests {
		for cut := range len(tc.input) + 1 {
			t.Run(tc.name, func(t *testing.T) {
				var d Decoder
				got := d.Feed([]byte(tc.input[:cut]))
				got = append(got, d.Feed([]byte(tc.input[cut:]))...)
				if !slices.Equal(got, tc.want) {
					t.Fatalf("cut %d: got %v want %v", cut, got, tc.want)
				}
			})
		}
	}
}

func TestDecoderByteAtATime(t *testing.T) {
	input := "a\x1b[200~日\n\x1b[201~\x1bOA😀\r"
	want := joined(typed("a"), named(KeyPasteStart), typed("日\n"), named(KeyPasteEnd, KeyUp), typed("😀"), named(KeyEnter))
	var d Decoder
	var got []Key
	for i := range len(input) {
		got = append(got, d.Feed([]byte{input[i]})...)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDecoderFlush(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantFeed  []Key
		wantFlush []Key
	}{
		{"nothing pending", "a", typed("a"), nil},
		{"lone esc", "\x1b", nil, named(KeyEsc)},
		{"esc after rune", "a\x1b", typed("a"), named(KeyEsc)},
		{"unfinished csi", "\x1b[", nil, joined(named(KeyEsc), typed("["))},
		{"unfinished ss3", "\x1bO", nil, joined(named(KeyEsc), typed("O"))},
		{"unfinished params", "\x1b[1;", nil, joined(named(KeyEsc), typed("[1;"))},
		{"unfinished rune", "\xe6\x97", nil, typed("��")},
		{"esc inside paste waits", "\x1b[200~a\x1b", joined(named(KeyPasteStart), typed("a")), nil},
		{"end marker part waits", "\x1b[200~\x1b[20", named(KeyPasteStart), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var d Decoder
			if got := d.Feed([]byte(tc.input)); !slices.Equal(got, tc.wantFeed) {
				t.Fatalf("feed got %v want %v", got, tc.wantFeed)
			}
			if got := d.Flush(); !slices.Equal(got, tc.wantFlush) {
				t.Fatalf("flush got %v want %v", got, tc.wantFlush)
			}
		})
	}
}

func TestDecoderFlushThenContinue(t *testing.T) {
	var d Decoder
	d.Feed([]byte("\x1b"))
	d.Flush()
	got := d.Feed([]byte("[A"))
	if want := typed("[A"); !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := d.Flush(); got != nil {
		t.Fatalf("second flush got %v", got)
	}
}

func TestDecoderPasteSurvivesFlush(t *testing.T) {
	var d Decoder
	d.Feed([]byte("\x1b[200~a\x1b"))
	d.Flush()
	got := d.Feed([]byte("[201~\r"))
	if want := named(KeyPasteEnd, KeyEnter); !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
