//go:build windows

package main

import "os/exec"

func runPing(target string) (string, error) {
	out, err := exec.Command("ping", "-n", "3", "-w", "2000", target).CombinedOutput()
	return string(out), err
}
