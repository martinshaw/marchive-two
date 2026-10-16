// Package paths resolves the on-disk locations Marchive owns.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Resolve returns the data directory. A flag wins, then MARCHIVE_HOME, then
// ~/Library/Application Support/Marchive. The directory is created.
func Resolve(flag string) (string, error) {
	dir := strings.TrimSpace(flag)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("MARCHIVE_HOME"))
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "Library", "Application Support", "Marchive")
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, dir[2:])
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return abs, nil
}

func DB(dir string) string      { return filepath.Join(dir, "marchive.db") }
func Lock(dir string) string    { return filepath.Join(dir, "lock") }
func Port(dir string) string    { return filepath.Join(dir, "port") }
func Profile(dir string) string { return filepath.Join(dir, "chrome-profile") }
func Log(dir string) string     { return filepath.Join(dir, "marchive.log") }
