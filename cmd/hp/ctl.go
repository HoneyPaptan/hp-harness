package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func runCtl(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty control command")
	}
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		if len(out) != 0 {
			return fmt.Errorf("%s: %v: %s", strings.Join(argv, " "), err, strings.TrimSpace(string(out)))
		}
		return err
	}
	return nil
}
