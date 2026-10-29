package cli

import (
	"bytes"
	"testing"
	"time"
)

func TestRunsDir(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"xdg", map[string]string{"XDG_STATE_HOME": "/state", "HOME": "/home/u"}, "/state/hp/runs"},
		{"default", map[string]string{"HOME": "/home/u"}, "/home/u/.local/state/hp/runs"},
		{"no home", map[string]string{}, ".local/state/hp/runs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runsDir(getenvMap(tc.env))
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestNewRunID(t *testing.T) {
	now := time.Date(2026, 3, 4, 15, 4, 5, 0, time.UTC)
	r := bytes.NewReader([]byte{0xab, 0xcd})
	id, err := newRunID(now, r)
	if err != nil {
		t.Fatal(err)
	}
	want := "20260304-150405-abcd"
	if id != want {
		t.Fatalf("got %q want %q", id, want)
	}
}
