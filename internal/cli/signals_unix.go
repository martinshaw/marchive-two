//go:build unix

package cli

import (
	"os"
	"syscall"
)

func stopSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
