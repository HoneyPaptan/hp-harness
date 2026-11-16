package main

import "testing"

func TestNeedsSignals(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"--version"}, false},
		{[]string{"version"}, false},
		{[]string{"runs"}, false},
		{[]string{"providers"}, false},
		{[]string{"list"}, false},
		{[]string{"remove", "x"}, false},
		{[]string{"permissions"}, false},
		{[]string{"stop", "id"}, false},
		{[]string{"help"}, false},
		{[]string{"-h"}, false},
		{[]string{"run", "hi"}, true},
		{[]string{"resume", "id"}, true},
		{[]string{"attach", "id"}, true},
		{[]string{"key", "set", "A"}, true},
		{[]string{"-c"}, true},
		{[]string{"web"}, true},
		{[]string{"add", "x"}, true},
		{[]string{"recover"}, true},
	}
	for _, tc := range cases {
		if got := needsSignals(tc.args); got != tc.want {
			t.Errorf("needsSignals(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
