//go:build linux

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const hpName = "hp"

func hpImageAlive(pid int) (bool, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, true
		}
		return false, false
	}
	fields := strings.FieldsFunc(string(data), func(r rune) bool { return r == 0 })
	if len(fields) == 0 {
		return false, true
	}
	base := filepath.Base(fields[0])
	if base == hpName || base == exeName() {
		return true, true
	}
	return false, true
}

func exeName() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Base(exe)
}

func processStart(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	i := strings.LastIndexByte(string(data), ')')
	if i < 0 {
		return ""
	}
	fields := strings.Fields(string(data)[i+1:])
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
