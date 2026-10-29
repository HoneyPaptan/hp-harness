package journal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

func ReadStart(h host.Host, path string) (RunStart, error) {
	recs, err := Read(h, path)
	if err != nil {
		return RunStart{}, err
	}
	if err := checkStart(recs); err != nil {
		return RunStart{}, err
	}
	return decode[RunStart](recs[0])
}

func Read(h host.Host, path string) ([]Record, error) {
	data, err := h.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseRecords(data)
}

func Open(h host.Host, path string) (*Writer, []Record, error) {
	data, err := h.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	recs, torn, err := parseRecordsRepair(data)
	if err != nil {
		return nil, nil, err
	}
	if err := checkStart(recs); err != nil {
		return nil, nil, err
	}
	if torn {
		if err := writeRecords(h, path, recs); err != nil {
			return nil, nil, err
		}
	}
	next := recs[len(recs)-1].Seq + 1
	w := &Writer{h: h, path: path, seq: next}
	return w, recs, nil
}

func parseRecords(data []byte) ([]Record, error) {
	recs, _, err := parseRecordsRepair(data)
	return recs, err
}

func parseRecordsRepair(data []byte) ([]Record, bool, error) {
	lines := splitLines(data)
	if len(lines) == 0 {
		return nil, false, nil
	}
	recs := make([]Record, 0, len(lines))
	for i, line := range lines {
		var rec Record
		err := json.Unmarshal([]byte(line), &rec)
		if err != nil {
			if i == len(lines)-1 {
				return recs, true, nil
			}
			return nil, false, fmt.Errorf("line %d: %w", i+1, err)
		}
		recs = append(recs, rec)
	}
	return recs, false, nil
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func writeRecords(h host.Host, path string, recs []Record) error {
	var b strings.Builder
	for _, rec := range recs {
		line, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return h.WriteFile(path, []byte(b.String()))
}
