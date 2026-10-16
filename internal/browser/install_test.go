package browser

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestInstallProgressAndCancel(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Chrome install is macOS only")
	}
	origLocate, origFetch, origPlace := locateChrome, fetchChrome, placeChrome
	t.Cleanup(func() {
		locateChrome, fetchChrome, placeChrome = origLocate, origFetch, origPlace
	})
	locateChrome = func() (string, error) { return "", errors.New("missing") }

	started := make(chan struct{})
	fetchChrome = func(ctx context.Context, dest string, report func(got, total int64)) error {
		if err := os.WriteFile(dest, []byte("dmg"), 0o644); err != nil {
			return err
		}
		report(40, 100)
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	placeChrome = func(context.Context, string) error { return errors.New("should not install") }

	in := NewInstaller()
	if err := in.Start(); err != nil {
		t.Fatal(err)
	}
	if err := in.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("download did not start")
	}
	snap := in.Snapshot()
	if snap.Phase != "downloading" || snap.Received != 40 || snap.Total != 100 {
		t.Fatalf("progress %+v", snap)
	}
	in.Cancel()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if in.Snapshot().Phase == "cancelled" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("phase %s", in.Snapshot().Phase)
}

func TestInstallCopiesAndFinishes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Chrome install is macOS only")
	}
	origLocate, origFetch, origPlace := locateChrome, fetchChrome, placeChrome
	t.Cleanup(func() {
		locateChrome, fetchChrome, placeChrome = origLocate, origFetch, origPlace
	})
	missing := true
	locateChrome = func() (string, error) {
		if missing {
			return "", errors.New("missing")
		}
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", nil
	}
	fetchChrome = func(ctx context.Context, dest string, report func(got, total int64)) error {
		report(10, 10)
		return os.WriteFile(dest, []byte("dmg"), 0o644)
	}
	placeChrome = func(context.Context, string) error {
		missing = false
		return nil
	}

	in := NewInstaller()
	if err := in.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := in.Snapshot()
		if snap.Phase == "done" && snap.Installed {
			if snap.Message != "Google Chrome is installed." {
				t.Fatalf("message %q", snap.Message)
			}
			return
		}
		if snap.Phase == "failed" {
			t.Fatal(snap.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("snapshot %+v", in.Snapshot())
}
