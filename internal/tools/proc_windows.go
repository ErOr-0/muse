//go:build windows

package tools

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps run_command children from opening a console window
// when spawned from the GUI app.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
