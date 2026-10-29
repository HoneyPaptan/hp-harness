package openai

import (
	"bufio"
	"io"
	"iter"
	"strings"
)

const sseMaxLine = 1024 * 1024

func sseData(r io.Reader) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		scan := bufio.NewScanner(r)
		scan.Buffer(make([]byte, 0, 64*1024), sseMaxLine)
		for scan.Scan() {
			line := scan.Text()
			payload, ok, err := sseLinePayload(line)
			if err != nil {
				yield("", err)
				return
			}
			if !ok {
				continue
			}
			if payload == "[DONE]" {
				return
			}
			if !yield(payload, nil) {
				return
			}
		}
		if err := scan.Err(); err != nil {
			yield("", err)
		}
	}
}

func sseLinePayload(line string) (string, bool, error) {
	if line == "" {
		return "", false, nil
	}
	if strings.HasPrefix(line, ":") {
		return "", false, nil
	}
	if !strings.HasPrefix(line, "data:") {
		return "", false, nil
	}
	payload := strings.TrimPrefix(line, "data:")
	if strings.HasPrefix(payload, " ") {
		payload = payload[1:]
	}
	return payload, true, nil
}
