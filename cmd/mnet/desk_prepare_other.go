//go:build !linux

package main

import "errors"

func prepareDeskRunFiles(string, string) error {
	return errors.New("Shared Desk engineering draft preparation currently requires Linux")
}
