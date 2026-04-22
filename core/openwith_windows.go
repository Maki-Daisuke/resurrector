package main

import (
	"fmt"
	"os/exec"
	"syscall"
)

func openWithDialog(path string) error {
	cmd := exec.Command("rundll32.exe", "shell32.dll,OpenAs_RunDLL", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting Open With dialog: %w", err)
	}
	return nil
}