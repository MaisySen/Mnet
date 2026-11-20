//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"fintex.au/mnet/internal/desk"
	"golang.org/x/sys/unix"
)

// Prepare only drafts in a newly created directory. No existing file is
// replaced, no key is read/copied, and no daemon or network request is made.
func prepareDeskRunFiles(requestPath, output string) (result error) {
	fd, err := unix.Open(requestPath, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.New("desk preparation file unavailable")
	}
	file := os.NewFile(uintptr(fd), requestPath)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("desk preparation file must be regular, not a link/device/FIFO")
	}
	request, err := desk.DecodePreparation(file)
	if err != nil {
		return err
	}
	prepared, err := desk.Prepare(request)
	if err != nil {
		return err
	}
	output = filepath.Clean(output)
	parentFD, err := unix.Open(filepath.Dir(output), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.New("desk output parent must be an existing private directory")
	}
	defer unix.Close(parentFD)
	var parentStat unix.Stat_t
	if err = unix.Fstat(parentFD, &parentStat); err != nil {
		return err
	}
	if parentStat.Uid != uint32(os.Geteuid()) || parentStat.Mode&0022 != 0 || filepath.Base(output) == "." || filepath.Base(output) == ".." || output == "/" {
		return errors.New("desk output parent must be owned by this user and not writable by others")
	}
	if err = unix.Mkdirat(parentFD, filepath.Base(output), 0700); err != nil {
		return errors.New("desk output must be a new directory; nothing replaced")
	}
	// Keep every write anchored to the opened directory, even if an ancestor
	// is renamed. Never recursively remove a pathname after a partial failure.
	outputFD, err := unix.Openat(parentFD, filepath.Base(output), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.New("desk output created but could not be opened; inspect it before retry")
	}
	defer unix.Close(outputFD)
	defer func() {
		if result != nil {
			result = errors.Join(result, errors.New("partial private drafts retained; do not run them; use a fresh output directory"))
		}
	}()
	write := func(dirFD int, name string, value any) error {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		fd, err := unix.Openat(dirFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if err != nil {
			return err
		}
		f := os.NewFile(uintptr(fd), name)
		_, writeErr := f.Write(data)
		syncErr := f.Sync()
		closeErr := f.Close()
		return errors.Join(writeErr, syncErr, closeErr)
	}
	for _, endpoint := range []struct {
		name string
		spec desk.RunSpec
	}{{"primary", prepared.Primary}, {"recipient", prepared.Recipient}} {
		if err = unix.Mkdirat(outputFD, endpoint.name, 0700); err != nil {
			return err
		}
		roleFD, openErr := unix.Openat(outputFD, endpoint.name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if openErr != nil {
			return openErr
		}
		err = unix.Mkdirat(roleFD, "replay", 0700)
		if err == nil {
			err = write(roleFD, "run.json", endpoint.spec)
		}
		err = errors.Join(err, unix.Fsync(roleFD), unix.Close(roleFD))
		if err != nil {
			return err
		}
	}
	if err = write(outputFD, "layout.json", prepared.Primary.Layout); err != nil {
		return err
	}
	note := []byte("SHARED DESK DRAFTS ONLY\n\nNo identity, approval, listener or input was created. Supplied inspection is not trust.\nCompare both exact public-key pins through an independent trusted source.\nReview layout.json before choosing either run. Do not edit the paired bindings independently.\nCopy each role directory to that role's computer only. Private keys are not included.\nCertificate/key paths must resolve on that computer. Never transfer a private key to the peer.\nLaunch only by explicitly selecting mnetd -desk-run run.json after independent review.\nBoth local default-decline reviews, actual geometry/map checks and one-use replay protection still apply.\nA decline, failure or completed run consumes this attempt. Prepare fresh drafts for any retry.\nThis is local engineering setup, not Multies sign-in or persistent trusted pairing.\n")
	noteFD, err := unix.Openat(outputFD, "REVIEW.txt", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(noteFD), "REVIEW.txt")
	_, err = f.Write(note)
	err = errors.Join(err, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	return errors.Join(unix.Fsync(outputFD), unix.Fsync(parentFD))
}
