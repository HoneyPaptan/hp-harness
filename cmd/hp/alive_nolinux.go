//go:build unix && !linux

package main

func hpImageAlive(int) (bool, bool) {
	return false, false
}
