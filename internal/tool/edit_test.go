package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestEdit(t *testing.T) {
	ctx := context.Background()
	dir := "/w"
	tests := []struct {
		name    string
		start   string
		raw     string
		wantErr bool
		want    string
	}{
		{
			name:  "single replace",
			start: "foo bar foo",
			raw:   `{"path":"f.txt","old_string":"bar","new_string":"baz"}`,
			want:  "foo baz foo",
		},
		{
			name:  "replace all",
			start: "aa aa",
			raw:   `{"path":"f.txt","old_string":"aa","new_string":"b","replace_all":true}`,
			want:  "b b",
		},
		{
			name:    "empty old",
			start:   "x",
			raw:     `{"path":"f.txt","old_string":"","new_string":"y"}`,
			wantErr: true,
			want:    "x",
		},
		{
			name:    "same strings",
			start:   "x",
			raw:     `{"path":"f.txt","old_string":"x","new_string":"x"}`,
			wantErr: true,
			want:    "x",
		},
		{
			name:    "zero matches",
			start:   "hello",
			raw:     `{"path":"f.txt","old_string":"zzz","new_string":"a"}`,
			wantErr: true,
			want:    "hello",
		},
		{
			name:    "two matches",
			start:   "ab ab",
			raw:     `{"path":"f.txt","old_string":"ab","new_string":"c"}`,
			wantErr: true,
			want:    "ab ab",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := host.NewMemory()
			if err := h.WriteFile(dir+"/f.txt", []byte(tc.start)); err != nil {
				t.Fatal(err)
			}
			res, err := editTool.Run(ctx, Env{Host: h, Dir: dir}, json.RawMessage(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if res.IsError != tc.wantErr {
				t.Fatalf("IsError=%v text=%q", res.IsError, res.Text)
			}
			if tc.name == "two matches" && res.Text != "2 matches for old_string in f.txt; add more context or set replace_all" {
				t.Fatalf("text %q", res.Text)
			}
			if tc.name == "replace all" && res.Text != "replaced 2 occurrence(s) in f.txt" {
				t.Fatalf("text %q", res.Text)
			}
			data, _ := h.ReadFile(dir + "/f.txt")
			if string(data) != tc.want {
				t.Fatalf("file %q want %q", data, tc.want)
			}
		})
	}
}
