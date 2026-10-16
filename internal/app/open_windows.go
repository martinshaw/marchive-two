//go:build windows

package app

import "os/exec"

func openCommand(rawURL string) *exec.Cmd {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
}
