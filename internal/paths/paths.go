// Package paths resolves the on-disk locations Marchive owns.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolve returns the data directory. A flag wins, then MARCHIVE_HOME, then
// the per-user application directory. The directory is created.
func Resolve(flag string) (string, error) {
	dir := strings.TrimSpace(flag)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("MARCHIVE_HOME"))
	}
	if dir == "" {
		var err error
		dir, err = userDataDir()
		if err != nil {
			return "", err
		}
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

// HomeArchive is the default place for captured files: a Marchive folder in the user's home directory.
func HomeArchive() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Marchive"), nil
}

func userDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return dataDirFor(runtime.GOOS, home, os.Getenv("APPDATA"), os.Getenv("XDG_DATA_HOME")), nil
}

func dataDirFor(goos, home, appData, xdg string) string {
	switch goos {
	case "windows":
		if strings.TrimSpace(appData) != "" {
			return filepath.Join(appData, "Marchive")
		}
		return filepath.Join(home, "AppData", "Roaming", "Marchive")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Marchive")
	default:
		if strings.TrimSpace(xdg) != "" {
			return filepath.Join(xdg, "Marchive")
		}
		return filepath.Join(home, ".local", "share", "Marchive")
	}
}

func DB(dir string) string      { return filepath.Join(dir, "marchive.db") }
func Lock(dir string) string    { return filepath.Join(dir, "lock") }
func Port(dir string) string    { return filepath.Join(dir, "port") }
func Profile(dir string) string { return filepath.Join(dir, "chrome-profile") }
func Log(dir string) string     { return filepath.Join(dir, "marchive.log") }
