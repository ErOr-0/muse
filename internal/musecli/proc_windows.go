//go:build windows

package musecli

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps child processes (the muse binary, any shim) from
// allocating a visible console window when spawned from the GUI app.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
