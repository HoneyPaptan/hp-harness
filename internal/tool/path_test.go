package tool

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		dir, p, want string
	}{
		{"/w", "src/a.go", "/w/src/a.go"},
		{"/w", "/abs/x", "/abs/x"},
		{"/w", "a/../b", "/w/b"},
		{"/w", "/a/../b", "/b"},
	}
	for _, tc := range tests {
		got := Resolve(tc.dir, tc.p)
		if got != tc.want {
			t.Errorf("Resolve(%q,%q)=%q want %q", tc.dir, tc.p, got, tc.want)
		}
	}
}
