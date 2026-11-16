package journal

import "testing"

func TestWorkedRecords(t *testing.T) {
	recs := []Record{
		{Type: TypeRunStart},
		{Type: TypeDetached},
		{Type: TypeUser},
		{Type: TypeDetached},
	}
	if got := WorkedRecords(recs); got != 2 {
		t.Fatalf("got %d want 2", got)
	}
}

func TestNextResume(t *testing.T) {
	recs := []Record{{Type: TypeRunStart}, {Type: TypeUser}}
	cases := []struct {
		name     string
		st       State
		prev     ResumeState
		resume   bool
		records  int
		attempts int
	}{
		{"first attempt", State{}, ResumeState{}, true, 2, 1},
		{"progress resets", State{}, ResumeState{Records: 1, Attempts: 3}, true, 2, 1},
		{"crash limit", State{}, ResumeState{Records: 2, Attempts: 3}, false, 2, 3},
		{"finished", State{Finished: true}, ResumeState{}, false, 0, 0},
		{"stopped", State{Stopped: true}, ResumeState{Records: 2, Attempts: 1}, false, 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resume, next := NextResume(tc.st, recs, tc.prev)
			if resume != tc.resume || next.Records != tc.records || next.Attempts != tc.attempts {
				t.Fatalf("resume=%v next=%+v", resume, next)
			}
		})
	}
}
