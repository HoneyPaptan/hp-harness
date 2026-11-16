package cli

import (
	"errors"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func TestCrewViewPassThrough(t *testing.T) {
	svc := &fakeCrew{titles: map[string][]string{}, dirty: 3}
	a := crewAgent(t, customEP, &fake.Provider{}, svc, nil)
	if n, err := a.CrewPreflight(); err != nil || n != 3 {
		t.Fatalf("preflight %d %v", n, err)
	}
	snap, found, err := a.CrewView("c9")
	if err != nil || !found || snap.ID != "view-c9" {
		t.Fatalf("view %+v %v %v", snap, found, err)
	}
	chunk, err := a.CrewFollow("c9", 2, 5)
	if err != nil || chunk.Text != "c9" || chunk.Offset != 7 {
		t.Fatalf("follow %+v %v", chunk, err)
	}
	svc.preflightErr = errors.New("/crew needs a git repository here (run git init first)")
	if _, err := a.CrewPreflight(); err == nil {
		t.Fatal("preflight error lost")
	}
}
