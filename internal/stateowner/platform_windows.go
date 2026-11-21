//go:build windows

package stateowner

import (
	"fmt"
	"os"

	"fintex.au/mnet/internal/privatefs"
	"golang.org/x/sys/windows"
)

func SingleLink(path string, _ os.FileInfo) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var st windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &st); err != nil {
		return err
	}
	if st.NumberOfLinks != 1 || st.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || st.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return fmt.Errorf("store file must be regular, non-reparse and single-link")
	}
	return nil
}

func lockFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// No FILE_SHARE_DELETE: ownership file cannot be renamed/replaced while held.
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(h), path)
	fail := func(err error) (*os.File, error) { file.Close(); return nil, err }
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() {
		return fail(fmt.Errorf("store lock must be regular"))
	}
	if err := SingleLink(path, info); err != nil {
		return fail(err)
	}
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped); err != nil {
		return fail(fmt.Errorf("store already owned or lock unavailable: %w", err))
	}
	if err := privatefs.ProtectFile(path); err != nil {
		return fail(err)
	}
	return file, nil
}

func Rename(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// Windows uses MoveFileEx WRITE_THROUGH at publication rather than POSIX
// directory fsync. This is not a claim of identical power-loss guarantees.
// Verify the resulting parent remains present; native Windows execution is
// required in addition to cross-compilation before this isolated code ships.
func SyncDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("store parent is not a directory")
	}
	return nil
}
