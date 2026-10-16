package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/martinshaw/marchive-two/internal/version"
)

// Official stable universal disk image published by Google.
const chromeDMGURL = "https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg"

// InstallSnapshot is the state of a Chrome install, for the progress dialog.
type InstallSnapshot struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Phase     string `json:"phase"`
	Received  int64  `json:"received"`
	Total     int64  `json:"total"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

// Installer downloads and copies the current stable Google Chrome.
type Installer struct {
	mu       sync.Mutex
	phase    string
	received int64
	total    int64
	message  string
	errText  string
	cancel   context.CancelFunc
}

func NewInstaller() *Installer {
	return &Installer{phase: "idle", message: "Webpage, article, and text captures use Google Chrome. Marchive can download the current stable version and install it."}
}

func (c *Client) InstallSnapshot() InstallSnapshot {
	if c == nil || c.install == nil {
		return InstallSnapshot{Phase: "idle"}
	}
	return c.install.Snapshot()
}

func (c *Client) StartInstall() error {
	if c == nil || c.install == nil {
		return errors.New("Marchive is not running")
	}
	return c.install.Start()
}

func (c *Client) CancelInstall() {
	if c == nil || c.install == nil {
		return
	}
	c.install.Cancel()
}

func (in *Installer) Snapshot() InstallSnapshot {
	path, err := locateChrome()
	snap := InstallSnapshot{Installed: err == nil, Path: path}
	in.mu.Lock()
	defer in.mu.Unlock()
	snap.Phase = in.phase
	snap.Received = in.received
	snap.Total = in.total
	snap.Message = in.message
	snap.Error = in.errText
	if snap.Installed && snap.Phase != "downloading" && snap.Phase != "installing" {
		if snap.Phase != "done" {
			snap.Phase = "idle"
		}
		snap.Message = "Google Chrome is installed."
		snap.Error = ""
	}
	if snap.Phase == "" {
		snap.Phase = "idle"
	}
	return snap
}

// Start begins a download if Chrome is missing. A second call while work is
// in progress attaches to that work.
func (in *Installer) Start() error {
	if runtime.GOOS != "darwin" {
		return errors.New("installing Chrome is supported on macOS")
	}
	if _, err := locateChrome(); err == nil {
		return errors.New("Google Chrome is already installed")
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	in.cancel = cancel
	in.phase = "downloading"
	in.received = 0
	in.total = 0
	in.errText = ""
	in.message = "Downloading Google Chrome…"
	go in.run(ctx)
	return nil
}

func (in *Installer) Cancel() {
	in.mu.Lock()
	cancel := in.cancel
	in.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (in *Installer) run(ctx context.Context) {
	dmg, err := os.CreateTemp("", "marchive-chrome-*.dmg")
	if err != nil {
		in.fail(err)
		return
	}
	dmgPath := dmg.Name()
	dmg.Close()
	defer os.Remove(dmgPath)

	err = fetchChrome(ctx, dmgPath, func(got, total int64) {
		in.mu.Lock()
		in.received = got
		in.total = total
		in.mu.Unlock()
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			in.finish("cancelled", "Install cancelled.", "")
			return
		}
		in.fail(err)
		return
	}

	in.mu.Lock()
	in.phase = "installing"
	in.message = "Installing Google Chrome…"
	in.mu.Unlock()

	if err := placeChrome(ctx, dmgPath); err != nil {
		if errors.Is(err, context.Canceled) {
			in.finish("cancelled", "Install cancelled.", "")
			return
		}
		in.fail(err)
		return
	}
	if _, err := locateChrome(); err != nil {
		in.fail(err)
		return
	}
	in.finish("done", "Google Chrome is installed.", "")
}

func (in *Installer) fail(err error) {
	in.finish("failed", "Could not install Google Chrome.", err.Error())
}

func (in *Installer) finish(phase, message, errText string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.phase = phase
	in.message = message
	in.errText = errText
	in.cancel = nil
}

var locateChrome = Find

var fetchChrome = downloadChromeDMG

var placeChrome = installChromeDMG

func downloadChromeDMG(ctx context.Context, dest string, report func(got, total int64)) error {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, chromeDMGURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Marchive/"+version.Version)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download Chrome: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download Chrome: HTTP %d", resp.StatusCode)
	}
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()
	total := resp.ContentLength
	var got int64
	buf := make([]byte, 256*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				return err
			}
			got += int64(n)
			if report != nil {
				report(got, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("download Chrome: %w", readErr)
		}
	}
	if report != nil {
		report(got, total)
	}
	return nil
}

func installChromeDMG(ctx context.Context, dmgPath string) error {
	mount, err := os.MkdirTemp("", "marchive-chrome-mount-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(mount)
	attach := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mount, dmgPath)
	if out, err := attach.CombinedOutput(); err != nil {
		return fmt.Errorf("open Chrome disk image: %s", tidyOutput(out, err))
	}
	defer exec.Command("hdiutil", "detach", "-quiet", mount).Run()

	src := filepath.Join(mount, "Google Chrome.app")
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return errors.New("the Chrome disk image did not contain Google Chrome.app")
	}
	dest, err := installTarget()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	copy := exec.CommandContext(ctx, "ditto", src, dest)
	if out, err := copy.CombinedOutput(); err != nil {
		os.RemoveAll(dest)
		return fmt.Errorf("copy Chrome: %s", tidyOutput(out, err))
	}
	_ = exec.Command("xattr", "-cr", dest).Run()
	return nil
}

func installTarget() (string, error) {
	if writable("/Applications") {
		return "/Applications/Google Chrome.app", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Applications", "Google Chrome.app"), nil
}

func writable(dir string) bool {
	file, err := os.CreateTemp(dir, ".marchive-write-*")
	if err != nil {
		return false
	}
	name := file.Name()
	file.Close()
	os.Remove(name)
	return true
}

func tidyOutput(out []byte, err error) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return err.Error()
	}
	return text
}
