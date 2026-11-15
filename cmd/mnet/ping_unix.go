//go:build !windows

package main

import "os/exec"

func runPing(target string) (string, error) {
	out, err := exec.Command("ping", "-c", "3", "-W", "2", target).CombinedOutput()
	return string(out), err
}
