//go:build windows

package privatefs

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// POSIX mode bits do not establish a Windows DACL. Remove inherited grants
// and allow only the existing owner, LocalSystem and local administrators.
// Keeping the existing owner lets an elevated CLI and LocalSystem service
// share the same dedicated state directory without giving normal users access.
func protect(path string, dir bool) error {
	current, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read private state owner: %w", err)
	}
	if current == nil {
		return fmt.Errorf("private state has no security descriptor")
	}
	owner, _, err := current.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("private state has no owner")
	}
	sd, err := privateDescriptor(owner, dir)
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return fmt.Errorf("build private state DACL")
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return fmt.Errorf("protect private state DACL: %w", err)
	}
	return nil
}

func privateDescriptor(user *windows.SID, dir bool) (*windows.SECURITY_DESCRIPTOR, error) {
	inherit := ""
	if dir {
		inherit = "OICI"
	}
	sddl := "D:P(A;" + inherit + ";FA;;;" + user.String() + ")(A;" + inherit + ";FA;;;SY)(A;" + inherit + ";FA;;;BA)"
	return windows.SecurityDescriptorFromString(sddl)
}

func privateCreateAttributes(dir bool) (*windows.SecurityAttributes, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sd, err := privateDescriptor(user.User.Sid, dir)
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}, nil
}

func createPath(path string) (*uint16, error) {
	if path == "" {
		return nil, os.ErrNotExist
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Preserve support for long local and UNC paths without changing device paths.
	if len(absolute) >= 248 && !strings.HasPrefix(absolute, `\\?\`) && !strings.HasPrefix(absolute, `\\.\`) {
		if strings.HasPrefix(absolute, `\\`) {
			absolute = `\\?\UNC\` + absolute[2:]
		} else {
			absolute = `\\?\` + absolute
		}
	}
	return windows.UTF16PtrFromString(absolute)
}

// CreateFile exclusively creates a private, writable file. The DACL applies
// at creation, before another process could open a handle through inheritance.
func CreateFile(path string) (*os.File, error) {
	name, err := createPath(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	attrs, err := privateCreateAttributes(false)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, attrs, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// CreateTemp exclusively creates a private temporary file in dir with prefix.
func CreateTemp(dir, prefix string) (*os.File, error) {
	return CreateFile(filepath.Join(dir, prefix+rand.Text()))
}

// MkdirTemp exclusively creates a private temporary directory in dir with prefix.
func MkdirTemp(dir, prefix string) (string, error) {
	path := filepath.Join(dir, prefix+rand.Text())
	name, err := createPath(path)
	if err != nil {
		return "", &os.PathError{Op: "mkdir", Path: path, Err: err}
	}
	attrs, err := privateCreateAttributes(true)
	if err != nil {
		return "", &os.PathError{Op: "mkdir", Path: path, Err: err}
	}
	if err := windows.CreateDirectory(name, attrs); err != nil {
		return "", &os.PathError{Op: "mkdir", Path: path, Err: err}
	}
	return path, nil
}
