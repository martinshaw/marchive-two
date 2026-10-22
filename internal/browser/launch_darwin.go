//go:build darwin

package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chromedp/chromedp"
)

// startError means Chrome aborted before a page could load. Retrying only
// opens another crash dialog.
type startError struct{ err error }

func (e *startError) Error() string {
	if e == nil || e.err == nil {
		return "Chrome quit while starting"
	}
	return "Chrome quit while starting: " + e.err.Error()
}

func (e *startError) Unwrap() error { return e.err }

func (e *startError) NoRetry() bool { return true }

func (c *Client) allocator(ctx context.Context, bin string, headed bool) (context.Context, context.CancelFunc, error) {
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		// A leftover headless capture or sign-in window keeps the profile lock.
		// The new process then exits and the debugging port never opens.
		releaseProfile(c.profile)
		port, err := freePort()
		if err != nil {
			return nil, nil, err
		}
		out, err := exec.Command("open", chromeArgs(bin, c.profile, port, headed)...).CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			last = errors.New(msg)
			continue
		}
		ws, err := waitDevTools(ctx, port)
		if err == nil {
			allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, ws)
			profile := c.profile
			cancel := func() {
				allocCancel()
				killListener(port)
				releaseProfile(profile)
			}
			return allocCtx, cancel, nil
		}
		killListener(port)
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		last = err
	}
	if last == nil {
		last = errors.New("debugging port did not open")
	}
	return nil, nil, &startError{err: last}
}

func chromeArgs(bin, profile string, port int, headed bool) []string {
	args := []string{
		"-n", "-a", chromeApp(bin), "--args",
		"--user-data-dir=" + profile,
		"--remote-debugging-port=" + strconv.Itoa(port),
	}
	if headed {
		args = append(args, "--profile-directory=Default")
	} else {
		args = append(args, "--headless=new")
	}
	args = append(args,
		"--mute-audio",
		"--window-size=1280,1800",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--disable-breakpad",
		"--disable-crash-reporter",
		"--use-mock-keychain",
		"--password-store=basic",
	)
	return args
}

func startVisibleChrome(ctx context.Context, bin, profile, page string) *exec.Cmd {
	return exec.CommandContext(ctx, "open",
		"-W", "-n", "-a", chromeApp(bin), "--args",
		"--user-data-dir="+profile,
		"--profile-directory=Default",
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		page,
	)
}

func chromeApp(bin string) string {
	dir := bin
	for i := 0; i < 3; i++ {
		dir = filepath.Dir(dir)
	}
	if strings.HasSuffix(dir, ".app") {
		return dir
	}
	return bin
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	err = ln.Close()
	return port, err
}

func waitDevTools(ctx context.Context, port int) (string, error) {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	client := &http.Client{Timeout: 400 * time.Millisecond}
	deadline := time.Now().Add(12 * time.Second)
	var last error
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if time.Now().After(deadline) {
			if last == nil {
				last = errors.New("debugging port did not open")
			}
			return "", last
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			last = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var payload struct {
			WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
		}
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err == nil && payload.WebSocketDebuggerURL != "" {
			return payload.WebSocketDebuggerURL, nil
		}
		last = err
		time.Sleep(100 * time.Millisecond)
	}
}

// releaseProfile quits Chrome processes started on Marchive's profile and
// drops a lock whose owner has already exited. The user's normal Chrome uses
// a different profile and is left alone.
func releaseProfile(profile string) {
	pids := profileBrowserPIDs(profile)
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(pids) > 0 && time.Now().Before(deadline) && anyAlive(pids) {
		time.Sleep(50 * time.Millisecond)
	}
	for _, pid := range pids {
		if processAlive(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	removeDeadLock(profile)
}

func profileBrowserPIDs(profile string) []int {
	out, err := exec.Command("ps", "-axww", "-o", "pid=", "-o", "command=").Output()
	if err != nil {
		return nil
	}
	return browserPIDsFromPS(string(out), profile)
}

func browserPIDsFromPS(output, profile string) []int {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidText, cmd, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(pidText))
		if err != nil || pid <= 0 || !strings.Contains(cmd, profile) {
			continue
		}
		if strings.Contains(cmd, "--type=") {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

func anyAlive(pids []int) bool {
	for _, pid := range pids {
		if processAlive(pid) {
			return true
		}
	}
	return false
}

func removeDeadLock(profile string) {
	pid := lockedPID(profile)
	if pid > 0 && processAlive(pid) {
		return
	}
	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(profile, name))
	}
}

func lockedPID(profile string) int {
	target, err := os.Readlink(filepath.Join(profile, "SingletonLock"))
	if err != nil {
		return 0
	}
	return singletonPID(target)
}

func singletonPID(target string) int {
	target = filepath.Base(target)
	dash := strings.LastIndex(target, "-")
	if dash < 0 || dash == len(target)-1 {
		return 0
	}
	pid, err := strconv.Atoi(target[dash+1:])
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func killListener(port int) {
	pids := listenPIDs(port)
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	if len(pids) == 0 {
		return
	}
	time.Sleep(200 * time.Millisecond)
	for _, pid := range listenPIDs(port) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

func listenPIDs(port int) []int {
	out, err := exec.Command("lsof", "-nP", "-t", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 0 {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}
