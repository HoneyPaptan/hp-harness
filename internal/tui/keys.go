package tui

import (
	"bytes"
	"unicode/utf8"
)

type KeyType int

const (
	KeyRune KeyType = iota
	KeyEnter
	KeyBackspace
	KeyDelete
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyCtrlA
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlK
	KeyCtrlL
	KeyCtrlU
	KeyCtrlW
	KeyEsc
	KeyTab
	KeyShiftTab
	KeyPgUp
	KeyPgDn
	KeyPasteStart
	KeyPasteEnd
	KeyCtrlB
	KeyCtrlF
	KeyCtrlJ
	KeyCtrlN
	KeyCtrlP
	KeyCtrlY
	KeyCtrlZ
	KeyWordLeft
	KeyWordRight
	KeyWordBackspace
	KeyWordDelete
	KeyNewline
	KeyDraftStart
	KeyDraftEnd
)

type Key struct {
	Type KeyType
	Rune rune
}

const esc = 0x1b

var pasteEnd = []byte("\x1b[201~")

var controlKeys = map[byte]KeyType{
	0x01: KeyCtrlA,
	0x02: KeyCtrlB,
	0x03: KeyCtrlC,
	0x04: KeyCtrlD,
	0x05: KeyCtrlE,
	0x06: KeyCtrlF,
	0x08: KeyBackspace,
	0x09: KeyTab,
	0x0a: KeyCtrlJ,
	0x0b: KeyCtrlK,
	0x0c: KeyCtrlL,
	0x0d: KeyEnter,
	0x0e: KeyCtrlN,
	0x10: KeyCtrlP,
	0x15: KeyCtrlU,
	0x17: KeyCtrlW,
	0x19: KeyCtrlY,
	0x1a: KeyCtrlZ,
	0x7f: KeyBackspace,
}

var altKeys = map[byte]KeyType{
	'b':  KeyWordLeft,
	'f':  KeyWordRight,
	'd':  KeyWordDelete,
	0x7f: KeyWordBackspace,
	0x08: KeyWordBackspace,
	'\r': KeyNewline,
	'\n': KeyNewline,
}

var modifiedKeys = map[KeyType]KeyType{
	KeyLeft:   KeyWordLeft,
	KeyRight:  KeyWordRight,
	KeyHome:   KeyDraftStart,
	KeyEnd:    KeyDraftEnd,
	KeyDelete: KeyWordDelete,
}

const (
	codeEnter     = "13"
	codeEsc       = "27"
	codeBackspace = "127"
	otherKeysCode = "27"
	modAlt        = 2
	modCtrl       = 4
)

var finalKeys = map[byte]KeyType{
	'A': KeyUp,
	'B': KeyDown,
	'C': KeyRight,
	'D': KeyLeft,
	'F': KeyEnd,
	'H': KeyHome,
}

var tildeKeys = map[string]KeyType{
	"1":   KeyHome,
	"3":   KeyDelete,
	"4":   KeyEnd,
	"5":   KeyPgUp,
	"6":   KeyPgDn,
	"7":   KeyHome,
	"8":   KeyEnd,
	"200": KeyPasteStart,
	"201": KeyPasteEnd,
}

type Decoder struct {
	pending []byte
	pasting bool
}

func (d *Decoder) Feed(p []byte) []Key {
	d.pending = append(d.pending, p...)
	return d.drain(nil)
}

func (d *Decoder) Flush() []Key {
	if d.pasting {
		return nil
	}
	keys := d.drain(nil)
	for len(d.pending) > 0 {
		keys = append(keys, flushedKey(d.pending[0]))
		d.pending = d.pending[1:]
		keys = d.drain(keys)
	}
	return keys
}

func (d *Decoder) Waiting() bool {
	return len(d.pending) > 0 && !d.pasting
}

func flushedKey(b byte) Key {
	if b == esc {
		return Key{Type: KeyEsc}
	}
	return runeKey(utf8.RuneError)
}

func (d *Decoder) drain(keys []Key) []Key {
	for len(d.pending) > 0 {
		key, ok, size := d.next(d.pending)
		if size == 0 {
			break
		}
		d.pending = d.pending[size:]
		if !ok {
			continue
		}
		d.track(key)
		keys = append(keys, key)
	}
	return keys
}

func (d *Decoder) track(key Key) {
	switch key.Type {
	case KeyPasteStart:
		d.pasting = true
	case KeyPasteEnd:
		d.pasting = false
	}
}

func (d *Decoder) next(b []byte) (Key, bool, int) {
	if d.pasting {
		return nextPasted(b)
	}
	if b[0] == esc {
		return nextEscape(b)
	}
	if b[0] < 0x20 || b[0] == 0x7f {
		key, ok := controlKeys[b[0]]
		return Key{Type: key}, ok, 1
	}
	return nextRune(b)
}

func nextPasted(b []byte) (Key, bool, int) {
	if bytes.HasPrefix(b, pasteEnd) {
		return Key{Type: KeyPasteEnd}, true, len(pasteEnd)
	}
	if bytes.HasPrefix(pasteEnd, b) {
		return Key{}, false, 0
	}
	return nextRune(b)
}

func nextRune(b []byte) (Key, bool, int) {
	if !utf8.FullRune(b) {
		return Key{}, false, 0
	}
	r, size := utf8.DecodeRune(b)
	return runeKey(r), true, size
}

func runeKey(r rune) Key {
	return Key{Type: KeyRune, Rune: r}
}

func nextEscape(b []byte) (Key, bool, int) {
	if len(b) < 2 {
		return Key{}, false, 0
	}
	switch b[1] {
	case '[':
		return nextCSI(b)
	case 'O':
		return nextSS3(b)
	}
	if key, ok := altKeys[b[1]]; ok {
		return Key{Type: key}, true, 2
	}
	return Key{Type: KeyEsc}, true, 1
}

func nextSS3(b []byte) (Key, bool, int) {
	if len(b) < 3 {
		return Key{}, false, 0
	}
	key, ok := finalKeys[b[2]]
	return Key{Type: key}, ok, 3
}

func nextCSI(b []byte) (Key, bool, int) {
	for i := 2; i < len(b); i++ {
		switch {
		case b[i] >= 0x20 && b[i] <= 0x3f:
		case b[i] >= 0x40 && b[i] <= 0x7e:
			key, ok := csiKey(b[2:i], b[i])
			return Key{Type: key}, ok, i + 1
		default:
			return Key{}, false, i
		}
	}
	return Key{}, false, 0
}

func csiKey(params []byte, final byte) (KeyType, bool) {
	if final == 'Z' && len(params) == 0 {
		return KeyShiftTab, true
	}
	parts := bytes.Split(params, []byte(";"))
	switch final {
	case 'u':
		return codeKey(string(parts[0]), modifiers(parts, 1))
	case '~':
		if string(parts[0]) == otherKeysCode && len(parts) == 3 {
			return codeKey(string(parts[2]), modifiers(parts, 1))
		}
		key, ok := tildeKeys[string(parts[0])]
		return withModifiers(key, modifiers(parts, 1)), ok
	}
	key, ok := finalKeys[final]
	return withModifiers(key, modifiers(parts, 1)), ok
}

func modifiers(parts [][]byte, index int) int {
	if len(parts) <= index {
		return 0
	}
	field, _, _ := bytes.Cut(parts[index], []byte(":"))
	n := 0
	for _, c := range field {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return max(n-1, 0)
}

func withModifiers(key KeyType, mods int) KeyType {
	if mods&(modAlt|modCtrl) == 0 {
		return key
	}
	if moved, ok := modifiedKeys[key]; ok {
		return moved
	}
	return key
}

func codeKey(code string, mods int) (KeyType, bool) {
	switch code {
	case codeEnter:
		if mods != 0 {
			return KeyNewline, true
		}
		return KeyEnter, true
	case codeBackspace:
		if mods&(modAlt|modCtrl) != 0 {
			return KeyWordBackspace, true
		}
		return KeyBackspace, true
	case codeEsc:
		return KeyEsc, true
	}
	return 0, false
}
