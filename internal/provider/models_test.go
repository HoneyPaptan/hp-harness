package provider

import (
	"reflect"
	"testing"
)

func TestCleanModelIDs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "empty", in: nil, want: nil},
		{name: "trim and sort", in: []string{" z ", "a", "  b  "}, want: []string{"a", "b", "z"}},
		{name: "drop empty", in: []string{"", "  ", "ok"}, want: []string{"ok"}},
		{name: "drop duplicates", in: []string{"m", "m", "n", "m"}, want: []string{"m", "n"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanModelIDs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("CleanModelIDs() = %v want %v", got, tc.want)
			}
		})
	}
}
