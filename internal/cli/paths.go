package cli

import (
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"time"
)

func runsDir(getenv func(string) string) string {
	if base := getenv("XDG_STATE_HOME"); base != "" {
		return path.Join(base, "hp", "runs")
	}
	home := getenv("HOME")
	if home == "" {
		home = "."
	}
	return path.Join(home, ".local", "state", "hp", "runs")
}

func newRunID(now time.Time, r io.Reader) (string, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", err
	}
	ts := now.UTC().Format("20060102-150405")
	return fmt.Sprintf("%s-%s", ts, hex.EncodeToString(b[:])), nil
}
