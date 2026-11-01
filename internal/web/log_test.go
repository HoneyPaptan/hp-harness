package web

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

func TestEventLogSince(t *testing.T) {
	var l eventLog
	l.add(Event{Kind: KindUser, Text: "q"})
	first := l.addText("héllo ")
	second := l.addText("world")
	end := l.add(Event{Kind: KindTurnEnd})
	cases := []struct {
		name  string
		after int64
		kinds string
		text  string
	}{
		{name: "all", after: 0, kinds: "user text turn_end", text: "héllo world"},
		{name: "before text", after: 1, kinds: "text turn_end", text: "héllo world"},
		{name: "mid text", after: first.ID, kinds: "text turn_end", text: "world"},
		{name: "end of text", after: second.ID, kinds: "turn_end"},
		{name: "caught up", after: end.ID, kinds: ""},
		{name: "future", after: end.ID + 9, kinds: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := l.since(c.after)
			if kinds(got) != c.kinds || textOf(got) != c.text {
				t.Fatalf("%q %q", kinds(got), textOf(got))
			}
			checkIncreasing(t, got)
		})
	}
}

func TestEventLogDropPartial(t *testing.T) {
	var l eventLog
	if l.dropPartial() {
		t.Fatal("dropped from empty")
	}
	l.add(Event{Kind: KindUser})
	seen := l.addText("par")
	if !l.dropPartial() {
		t.Fatal("nothing dropped")
	}
	retry := l.add(Event{Kind: KindRetry, Discard: true})
	l.addText("full")
	got := l.since(seen.ID - 1)
	if kinds(got) != "retry text" || !got[0].Discard || textOf(got) != "full" {
		t.Fatalf("%+v", got)
	}
	if retry.ID <= seen.ID {
		t.Fatalf("ids %d %d", retry.ID, seen.ID)
	}
}

func TestCapText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		cut  bool
	}{
		{name: "short", in: "abc"},
		{name: "exact", in: strings.Repeat("a", toolTextCap)},
		{name: "over", in: strings.Repeat("a", toolTextCap+1), cut: true},
		{name: "rune boundary", in: "a" + strings.Repeat("é", toolTextCap), cut: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, cut := capText(c.in)
			if cut != c.cut || len(got) > toolTextCap || strings.ContainsRune(got, 0xfffd) {
				t.Fatalf("len %d cut %v", len(got), cut)
			}
			if !strings.HasPrefix(c.in, got) {
				t.Fatal("not a prefix")
			}
		})
	}
}

func TestReplayedRunEndHasNoStats(t *testing.T) {
	data, err := json.Marshal(journal.RunEnd{Reason: "final"})
	if err != nil {
		t.Fatal(err)
	}
	evs := recordEvents(journal.Record{Type: journal.TypeRunEnd, Data: data})
	if len(evs) != 1 || evs[0].Kind != KindTurnEnd || evs[0].Reason != "final" {
		t.Fatalf("%+v", evs)
	}
	raw, err := json.Marshal(evs[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"steps", "input_tokens", "output_tokens", "ttft_ms"} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("%s in %s", key, raw)
		}
	}
}

func TestTurnEndAlwaysCarriesStats(t *testing.T) {
	raw, err := json.Marshal(turnEndEvent(Stats{Reason: "final"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"steps":0`, `"input_tokens":0`, `"output_tokens":0`, `"ttft_ms":0`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("no %s in %s", key, raw)
		}
	}
}

func TestDropPartial(t *testing.T) {
	text := Event{Kind: KindText, Text: "x"}
	think := Event{Kind: KindThinking}
	user := Event{Kind: KindUser}
	cases := []struct {
		name  string
		items []Event
		want  bool
		kinds string
	}{
		{name: "text", items: []Event{user, text}, want: true, kinds: "user"},
		{name: "thinking after text", items: []Event{user, text, think}, want: true, kinds: "user"},
		{name: "thinking only", items: []Event{user, think}, kinds: "user thinking"},
		{name: "empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var l eventLog
			for _, ev := range c.items {
				l.add(ev)
			}
			if got := l.dropPartial(); got != c.want {
				t.Fatalf("dropped %v", got)
			}
			if got := kinds(l.since(0)); got != c.kinds {
				t.Fatalf("kinds %q", got)
			}
		})
	}
}
