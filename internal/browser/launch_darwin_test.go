//go:build darwin

package browser

import "testing"

func TestChromeApp(t *testing.T) {
	got := chromeApp("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
	if got != "/Applications/Google Chrome.app" {
		t.Fatalf("app = %s", got)
	}
	if chromeApp("/usr/bin/chromium") != "/usr/bin/chromium" {
		t.Fatal("non-bundle path should stay unchanged")
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
