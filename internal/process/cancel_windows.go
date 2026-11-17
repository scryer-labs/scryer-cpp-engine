package process

import (
	"os/exec"
	"strconv"
)

func ConfigureCancellation(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
	}
}
