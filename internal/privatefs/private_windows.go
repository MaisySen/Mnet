//go:build windows

package privatefs

import (
	"fmt"
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
	inherit := ""
	if dir {
		inherit = "OICI"
	}
	sddl := "D:P(A;" + inherit + ";FA;;;" + owner.String() + ")(A;" + inherit + ";FA;;;SY)(A;" + inherit + ";FA;;;BA)"
	sd, err := windows.SecurityDescriptorFromString(sddl)
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
