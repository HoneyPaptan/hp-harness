package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/fake"
)

func userRecords(t *testing.T, h host.Host) []string {
	t.Helper()
	var texts []string
	for _, m := range replayJournal(t, h).Messages {
		if m.Role == provider.RoleUser && len(m.ToolResults) == 0 {
			texts = append(texts, m.Text)
		}
	}
	return texts
}

func TestContinueAfterFailedFirstTurn(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		retry  string
		wantMs []provider.Message
		users  []string
	}{
		{
			"same prompt is not written twice",
			"first",
			[]provider.Message{{Role: provider.RoleUser, Text: "first"}},
			[]string{"first"},
		},
		{
			"a different prompt is a new message",
			"second",
			[]provider.Message{
				{Role: provider.RoleUser, Text: "first"},
				{Role: provider.RoleUser, Text: "second"},
			},
			[]string{"first", "second"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host.NewMemory()
			failing := &fake.Provider{Turns: []fake.Turn{{Err: boom}}}
			if _, err := Run(context.Background(), builtinDeps(h, failing), continueCfg(5)); !errors.Is(err, boom) {
				t.Fatalf("err %v", err)
			}
			retry := &fake.Provider{Turns: []fake.Turn{fake.TextTurn("ok")}}
			if _, err := ContinueInterrupted(context.Background(), builtinDeps(h, retry), "/j", tt.retry); err != nil {
				t.Fatal(err)
			}
			if got := retry.Requests[0].Messages; !reflect.DeepEqual(got, tt.wantMs) {
				t.Fatalf("messages %+v", got)
			}
			if got := userRecords(t, h); !reflect.DeepEqual(got, tt.users) {
				t.Fatalf("user messages %v", got)
			}
		})
	}
}
