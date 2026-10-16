//go:build darwin

package app

import "os/exec"

func openCommand(rawURL string) *exec.Cmd {
	return exec.Command("open", rawURL)
}
