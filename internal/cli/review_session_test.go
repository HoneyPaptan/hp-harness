package cli

import (
	"context"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/permit"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestReviewRequestCarriesTheRunID(t *testing.T) {
	cases := []struct {
		name  string
		runID func() string
		want  string
	}{
		{name: "run id", runID: func() string { return "run-9" }, want: "run-9"},
		{name: "no run id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fake.Provider{Turns: []fake.Turn{fake.TextTurn(`{"hold": false, "quote": "", "reason": ""}`)}}
			rv := testReviewer(fp)
			rv.runID = tc.runID
			rv.review(context.Background(), permit.Action{Tool: "bash", Target: "ls"}, nil)
			if got := fp.Requests[0].SessionID; got != tc.want {
				t.Fatalf("SessionID = %q, want %q", got, tc.want)
			}
		})
	}
}
