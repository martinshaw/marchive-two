// Package app starts the single Marchive process.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/httpapi"
	"github.com/martinshaw/marchive-two/internal/lock"
	"github.com/martinshaw/marchive-two/internal/paths"
	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/runner"
	"github.com/martinshaw/marchive-two/internal/service"
	"github.com/martinshaw/marchive-two/internal/store"
)

// ErrAlreadyRunning means another process holds the data directory.
var ErrAlreadyRunning = errors.New("marchive is already running")

// Options controls a foreground server.
type Options struct {
	DataDir     string
	OpenBrowser bool
}

// Serve listens on 127.0.0.1 until ctx is cancelled.
func Serve(ctx context.Context, opts Options) error {
	lk, err := lock.Acquire(paths.Lock(opts.DataDir))
	if errors.Is(err, lock.ErrBusy) {
		return ErrAlreadyRunning
	}
	if err != nil {
		return err
	}
	defer lk.Release()

	st, err := store.Open(paths.DB(opts.DataDir))
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.FailInterrupted(); err != nil {
		return err
	}

	br := browser.New(paths.Profile(opts.DataDir))
	reg := provider.DefaultRegistry()
	runr := runner.New(st, reg, br)
	svc := service.New(st, runr, br)
	svc.DataDir = opts.DataDir

	configured, err := st.ListenPort()
	if err != nil {
		return err
	}
	addr := "127.0.0.1:0"
	if configured != 0 {
		addr = "127.0.0.1:" + strconv.Itoa(configured)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if configured != 0 {
			return fmt.Errorf("could not listen on port %d: %w", configured, err)
		}
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	svc.ActivePort = port
	if err := os.WriteFile(paths.Port(opts.DataDir), []byte(strconv.Itoa(port)), 0o644); err != nil {
		ln.Close()
		return err
	}
	defer os.Remove(paths.Port(opts.DataDir))

	runCtx, runCancel := context.WithCancel(ctx)
	runr.Start(runCtx)
	defer func() {
		runCancel()
		runr.Shutdown()
	}()

	server := &http.Server{Handler: httpapi.New(svc)}
	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	slog.Info("listening", "url", base)
	if opts.OpenBrowser {
		if err := Open(base + "/"); err != nil {
			slog.Error("open browser", "err", err)
		}
	}

	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = server.Shutdown(shut)
		runCancel()
		return nil
	case err := <-errCh:
		runCancel()
		return err
	}
}

// Probe returns the base URL when a healthy server is already listening.
func Probe(dir string) (string, bool) {
	raw, err := os.ReadFile(paths.Port(dir))
	if err != nil {
		return "", false
	}
	base := "http://127.0.0.1:" + strings.TrimSpace(string(raw))
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/health", nil)
	if err != nil {
		return "", false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	return base, true
}

// WaitReady polls until the server answers or the timeout passes.
func WaitReady(dir string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if base, ok := Probe(dir); ok {
			return base, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", errors.New("Marchive did not start. See " + paths.Log(dir))
}

// StartBackground launches `marchive serve` detached from this terminal.
func StartBackground(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(paths.Log(dir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "serve", "--data-dir", dir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	return logFile.Close()
}

// Open asks macOS to open a URL.
func Open(rawURL string) error {
	return openCommand(rawURL).Start()
}
