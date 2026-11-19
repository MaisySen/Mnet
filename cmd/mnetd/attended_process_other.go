//go:build !linux

package main

func maybeRunAttendedProcess() bool { return false }
