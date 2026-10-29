package journal

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func TestRead(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	start := RunStart{RunID: "r1", Model: "m", System: "s", Dir: "/w", StepLimit: 3}

	mkClean := func(t *testing.T) (host.Host, string) {
		t.Helper()
		h := host.NewMemory()
		h.Clock = fixed
		path := "/j"
		w, err := Create(h, path, start)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.User("a"); err != nil {
			t.Fatal(err)
		}
		return h, path
	}

	tests := []struct {
		name    string
		setup   func(t *testing.T) (host.Host, string)
		wantLen int
		wantErr string
	}{
		{
			name: "clean",
			setup: func(t *testing.T) (host.Host, string) {
				return mkClean(t)
			},
			wantLen: 2,
		},
		{
			name: "torn last line",
			setup: func(t *testing.T) (host.Host, string) {
				h, path := mkClean(t)
				raw, err := h.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				torn := append(raw, []byte(`{"seq":99,"ts":"x","type":"user","data":{`)...)
				if err := h.WriteFile(path, torn); err != nil {
					t.Fatal(err)
				}
				return h, path
			},
			wantLen: 2,
		},
		{
			name: "corrupt middle line",
			setup: func(t *testing.T) (host.Host, string) {
				h, path := mkClean(t)
				raw, err := h.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				parts := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
				parts = append(parts[:1], append([]string{"not-json"}, parts[1:]...)...)
				if err := h.WriteFile(path, []byte(strings.Join(parts, "\n")+"\n")); err != nil {
					t.Fatal(err)
				}
				return h, path
			},
			wantErr: "line 2",
		},
		{
			name: "empty file",
			setup: func(t *testing.T) (host.Host, string) {
				h := host.NewMemory()
				path := "/empty"
				if err := h.WriteFile(path, nil); err != nil {
					t.Fatal(err)
				}
				return h, path
			},
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, path := tc.setup(t)
			recs, err := Read(h, path)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(recs) != tc.wantLen {
				t.Fatalf("len %d", len(recs))
			}
		})
	}
}

func TestOpen(t *testing.T) {
	fixed := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	start := RunStart{RunID: "r1", Model: "m", System: "s", Dir: "/w", StepLimit: 3}

	t.Run("empty file", func(t *testing.T) {
		h := host.NewMemory()
		if err := h.WriteFile("/e", nil); err != nil {
			t.Fatal(err)
		}
		_, _, err := Open(h, "/e")
		if !errors.Is(err, ErrEmpty) {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("torn tail repair", func(t *testing.T) {
		h := host.NewMemory()
		h.Clock = fixed
		path := "/j"
		w, err := Create(h, path, start)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.User("a"); err != nil {
			t.Fatal(err)
		}
		raw, err := h.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		torn := append(raw, []byte(`{"broken`)...)
		if err := h.WriteFile(path, torn); err != nil {
			t.Fatal(err)
		}
		w2, recs, err := Open(h, path)
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != 2 {
			t.Fatalf("len %d", len(recs))
		}
		if w2.seq != 2 {
			t.Fatalf("seq %d", w2.seq)
		}
		if err := w2.User("b"); err != nil {
			t.Fatal(err)
		}
		all, err := Read(h, path)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 3 {
			t.Fatalf("len %d", len(all))
		}
	})

	t.Run("missing run_start", func(t *testing.T) {
		h := host.NewMemory()
		rec := Record{Seq: 0, TS: fixed, Type: TypeUser, Data: json.RawMessage(`{"text":"x"}`)}
		if err := h.WriteFile("/bad", []byte(line(rec)+"\n")); err != nil {
			t.Fatal(err)
		}
		_, _, err := Open(h, "/bad")
		if !errors.Is(err, ErrNoRunStart) {
			t.Fatalf("err %v", err)
		}
	})
}

func line(rec Record) string {
	b, _ := json.Marshal(rec)
	return string(b)
}
