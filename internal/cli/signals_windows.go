//go:build windows

package cli

import "os"

func stopSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
