package core

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestE2EScriptedRun(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	dir := t.TempDir()
	jpath := filepath.Join(dir, "run.journal")
	file := "f.txt"
	write := provider.ToolCall{
		ID: "w", Name: "write",
		Args: json.RawMessage(`{"path":"f.txt","content":"line1\nline2\n"}`),
	}
	edit := provider.ToolCall{
		ID: "e", Name: "edit",
		Args: json.RawMessage(`{"path":"f.txt","old_string":"line1","new_string":"line1-edited"}`),
	}
	read := provider.ToolCall{
		ID: "r", Name: "read", Args: json.RawMessage(`{"path":"f.txt"}`),
	}
	bash := provider.ToolCall{
		ID: "b", Name: "bash",
		Args: json.RawMessage(`{"command":"cat f.txt | wc -l"}`),
	}
	grep := provider.ToolCall{
		ID: "g", Name: "grep",
		Args: json.RawMessage(`{"pattern":"edited","path":"f.txt"}`),
	}
	fp := &fake.Provider{Turns: []fake.Turn{
		fake.ToolTurn(write),
		fake.ToolTurn(edit),
		fake.ToolTurn(read),
		fake.ToolTurn(bash),
		fake.ToolTurn(grep),
		fake.TextTurn("complete"),
	}}
	reg := tool.NewRegistry(tool.Builtins()...)
	cfg := Config{
		RunID: "e2e", Model: "m", System: "s", Dir: dir,
		JournalPath: jpath, Prompt: "work", StepLimit: 20,
	}
	out, err := Run(context.Background(), Deps{Host: host.Native{}, Provider: fp, Tools: reg}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "complete" || out.Reason != ReasonFinal {
		t.Fatalf("out %+v", out)
	}
	want := "line1-edited\nline2\n"
	got, err := host.Native{}.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("file %q want %q", got, want)
	}
	recs, err := journal.Read(host.Native{}, jpath)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range recs {
		if rec.Type != journal.TypeToolResult {
			continue
		}
		var tr journal.ToolResultData
		if err := json.Unmarshal(rec.Data, &tr); err != nil {
			t.Fatal(err)
		}
		if tr.Result.IsError {
			t.Fatalf("tool error %+v", tr.Result)
		}
	}
	st, err := journal.Replay(recs)
	if err != nil {
		t.Fatal(err)
	}
	recs2, err := journal.Read(host.Native{}, jpath)
	if err != nil {
		t.Fatal(err)
	}
	st2, err := journal.Replay(recs2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(st, st2) {
		t.Fatal("replay drift")
	}
	last := fp.Requests[len(fp.Requests)-1]
	sent := st.Messages[:len(st.Messages)-1]
	if !reflect.DeepEqual(last.Messages, sent) {
		t.Fatalf("history mismatch")
	}
}
