//go:build !windows

package musecli

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
