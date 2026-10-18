//go:build darwin

package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChromeApp(t *testing.T) {
	got := chromeApp("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
	if got != "/Applications/Google Chrome.app" {
		t.Fatalf("app = %s", got)
	}
	if chromeApp("/usr/bin/chromium") != "/usr/bin/chromium" {
		t.Fatal("non-bundle path should stay unchanged")
	}
}

func TestChromeArgsUsesAVisibleWindowWhenHeaded(t *testing.T) {
	visible := strings.Join(chromeArgs("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/tmp/profile", 9, true), "\n")
	if strings.Contains(visible, "headless") {
		t.Fatalf("visible args include headless:\n%s", visible)
	}
	if !strings.Contains(visible, "--profile-directory=Default") {
		t.Fatalf("visible args:\n%s", visible)
	}
	hidden := strings.Join(chromeArgs("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/tmp/profile", 9, false), "\n")
	if !strings.Contains(hidden, "--headless=new") || strings.Contains(hidden, "--profile-directory=Default") {
		t.Fatalf("hidden args:\n%s", hidden)
	}
}

func TestSingletonPID(t *testing.T) {
	if got := singletonPID("Martins-MacBook-Pro.local-33821"); got != 33821 {
		t.Fatalf("pid = %d", got)
	}
	if got := singletonPID("host-name-12"); got != 12 {
		t.Fatalf("pid = %d", got)
	}
	if singletonPID("nope") != 0 || singletonPID("host-") != 0 {
		t.Fatal("expected no pid")
	}
}

func TestBrowserPIDsFromPS(t *testing.T) {
	const profile = "/Users/martin/Library/Application Support/Marchive/chrome-profile"
	output := strings.Join([]string{
		"14094 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"15157 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir=" + profile + " --new-window about:blank",
		"15163 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome Helper --type=gpu-process --user-data-dir=" + profile,
		"33821 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir=" + profile + " --headless=new",
	}, "\n")
	got := browserPIDsFromPS(output, profile)
	if len(got) != 2 || got[0] != 15157 || got[1] != 33821 {
		t.Fatalf("pids %v", got)
	}
	if browserPIDsFromPS(output, "") != nil {
		t.Fatal("empty profile should match nothing")
	}
}

func TestRemoveDeadLock(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		if err := os.Symlink("host-999999", filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	removeDeadLock(dir)
	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still present: %v", name, err)
		}
	}
}

func TestStartErrorDoesNotRetry(t *testing.T) {
	err := &startError{err: errTest}
	if !err.NoRetry() {
		t.Fatal("startup failure should not be retried")
	}
}

var errTest = errString("quit")

type errString string

func (e errString) Error() string { return string(e) }
