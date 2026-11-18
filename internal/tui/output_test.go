package tui

import (
	"strings"
	"testing"
)

func TestOutputShowsTheSuffixUnderAStreamingLine(t *testing.T) {
	var buf strings.Builder
	o := output{w: &buf, width: func() int { return 40 }}
	o.text("partial")
	o.setSuffix(suffixJoin + escStopHint)
	shown := reserveRow + saveCursor + belowRow + escStopHint + eraseRest + loadCursor
	if !strings.HasSuffix(buf.String(), shown) {
		t.Fatalf("suffix not drawn under the line: %q", buf.String())
	}
	cleared := saveCursor + belowRow + eraseRest + loadCursor
	buf.Reset()
	o.text(" more")
	if !strings.HasPrefix(buf.String(), cleared+" more") {
		t.Fatalf("row under the line not cleared before new text: %q", buf.String())
	}
	buf.Reset()
	o.endLine()
	if buf.String() != cleared+rowSeparator {
		t.Fatalf("row under the line not cleared before the line ended: %q", buf.String())
	}
	buf.Reset()
	o.setSuffix("")
	o.text("next")
	if strings.Contains(buf.String(), saveCursor) {
		t.Fatalf("an empty suffix drew a row: %q", buf.String())
	}
}

func TestOutputCutsTheSuffixRowToTheWidth(t *testing.T) {
	var buf strings.Builder
	o := output{w: &buf, width: func() int { return 10 }}
	o.text("x")
	o.setSuffix(suffixJoin + "0123456789abcdef")
	if !strings.Contains(buf.String(), belowRow+"01234567…"+eraseRest) {
		t.Fatalf("suffix not cut: %q", buf.String())
	}
}
