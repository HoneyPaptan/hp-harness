package transport

import (
	"strings"
	"testing"
)

func collectSSE(input string) ([]string, error) {
	var out []string
	for payload, err := range SSEData(strings.NewReader(input)) {
		if err != nil {
			return out, err
		}
		out = append(out, payload)
	}
	return out, nil
}

func TestSSEData(t *testing.T) {
	input := ": comment\n\nevent: ping\ndata: {\"a\":1}\n\ndata: spaced\n\ndata: [DONE]\n\ndata: after\n"
	got, err := collectSSE(input)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`{"a":1}`, "spaced"}
	if len(got) != len(want) {
		t.Fatalf("payloads = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSSEDataScannerError(t *testing.T) {
	long := strings.Repeat("x", MaxSSELine+1)
	_, err := collectSSE("data: " + long + "\n")
	if err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestSSEDataEarlyStop(t *testing.T) {
	n := 0
	for _, err := range SSEData(strings.NewReader("data: a\n\ndata: b\n")) {
		if err != nil {
			t.Fatal(err)
		}
		n++
		if n == 1 {
			break
		}
	}
}

func TestSSEDataEmpty(t *testing.T) {
	got, err := collectSSE("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestSSEDataDoneOnly(t *testing.T) {
	got, err := collectSSE("data: [DONE]\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
