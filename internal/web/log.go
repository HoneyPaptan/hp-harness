package web

import "sort"

type logItem struct {
	ev    Event
	start int64
	buf   []byte
}

type eventLog struct {
	items []logItem
	last  int64
}

func (l *eventLog) add(ev Event) Event {
	if ev.Kind == KindText {
		return l.addText(ev.Text)
	}
	l.last++
	ev.ID = l.last
	l.items = append(l.items, logItem{ev: ev})
	return ev
}

func (l *eventLog) addText(text string) Event {
	start := l.last
	l.last += int64(len(text))
	if n := len(l.items); n > 0 && l.items[n-1].ev.Kind == KindText {
		l.items[n-1].buf = append(l.items[n-1].buf, text...)
		l.items[n-1].ev.ID = l.last
		return Event{ID: l.last, Kind: KindText, Text: text}
	}
	ev := Event{ID: l.last, Kind: KindText}
	l.items = append(l.items, logItem{ev: ev, start: start, buf: []byte(text)})
	return Event{ID: l.last, Kind: KindText, Text: text}
}

func (l *eventLog) dropPartial() bool {
	n := len(l.items)
	for n > 0 && l.items[n-1].ev.Kind == KindThinking {
		n--
	}
	if n == 0 || l.items[n-1].ev.Kind != KindText {
		return false
	}
	l.items = l.items[:n-1]
	return true
}

func (l *eventLog) since(after int64) []Event {
	i := sort.Search(len(l.items), func(i int) bool { return l.items[i].ev.ID > after })
	out := make([]Event, 0, len(l.items)-i)
	for _, it := range l.items[i:] {
		out = append(out, it.read(after))
	}
	return out
}

func (it logItem) read(after int64) Event {
	if it.ev.Kind != KindText {
		return it.ev
	}
	ev := it.ev
	ev.Text = string(it.buf[max(after-it.start, 0):])
	return ev
}
