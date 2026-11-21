//go:build linux && mnet_enrollment_development

package osownership

import (
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// Pure construction, never Start/Run. Retained for exact placement inspection.
func residentCommand(exe string, uid, gid int, verdict *os.File, parent Namespace, parentPID int, gate *os.File) *exec.Cmd {
	cmd := exec.Command(exe, "--osownership-resident-no-tun-source")
	cmd.Stdin = gate
	cmd.Env = []string{"PATH=/usr/bin:/bin", "MNO_PARENT_DEV=" + strconv.FormatUint(parent.Device, 10), "MNO_PARENT_INO=" + strconv.FormatUint(parent.Inode, 10), "MNO_PARENT_PID=" + strconv.Itoa(parentPID)}
	cmd.ExtraFiles = []*os.File{verdict}
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWUSER | unix.CLONE_NEWNET, UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: uid, Size: 1}}, GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: gid, Size: 1}}, GidMappingsEnableSetgroups: false, Pdeathsig: syscall.SIGKILL}
	return cmd
}
