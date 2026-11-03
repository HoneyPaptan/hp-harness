package host

import (
	"slices"
	"testing"
)

func TestToolExecEnv(t *testing.T) {
	got := ToolExecEnv([]string{"FOO=1", "HP_DETACHED=1", "AI_AGENT=old", "BAR=2"})
	want := []string{"FOO=1", "BAR=2", "AI_AGENT=hp"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestToolExecEnvNilUsesProcess(t *testing.T) {
	t.Setenv("HP_TEST_EXECENV", "yes")
	t.Setenv("HP_DETACHED", "1")
	t.Setenv("AI_AGENT", "other")
	got := ToolExecEnv(nil)
	if !slices.Contains(got, "HP_TEST_EXECENV=yes") {
		t.Fatalf("missing test var in %v", got)
	}
	if slices.Contains(got, "HP_DETACHED=1") {
		t.Fatal("HP_DETACHED not filtered")
	}
	count := 0
	for _, e := range got {
		if e == "AI_AGENT=hp" {
			count++
		}
		if e == "AI_AGENT=other" {
			t.Fatal("old AI_AGENT kept")
		}
	}
	if count != 1 {
		t.Fatalf("AI_AGENT count %d", count)
	}
}
