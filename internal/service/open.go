package service

import (
	"os/exec"
	"runtime"
)

func openPath(path string) error {
	if runtime.GOOS != "darwin" {
		return &Error{Status: 400, Msg: "showing a folder is supported on macOS"}
	}
	return exec.Command("open", path).Start()
}
