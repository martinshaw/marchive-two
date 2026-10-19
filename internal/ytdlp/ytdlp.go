// Package ytdlp downloads and updates the yt-dlp binary Marchive uses for YouTube.
package ytdlp

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

	"github.com/martinshaw/marchive-two/internal/paths"
	"github.com/martinshaw/marchive-two/internal/version"
)

// Snapshot is the state of a yt-dlp download, for the progress dialog.
type Snapshot struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Phase     string `json:"phase"`
	Received  int64  `json:"received"`
	Total     int64  `json:"total"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

// Installer downloads the current yt-dlp release for this computer.
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
	return &Installer{phase: "idle", message: "YouTube captures use yt-dlp. Marchive can download the current release and replace it when you ask."}
}

// Status reports the managed binary without starting a download.
func Status(dataDir string) (path, ver string) {
	path, err := Executable(dataDir)
	if err != nil {
		return "", ""
	}
	return path, readVersion(dataDir)
}

// Executable is the managed yt-dlp binary. It is stored in the app data directory.
func Executable(dataDir string) (string, error) {
	dir, err := binDir(dataDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, binaryName())
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", errors.New("yt-dlp is not installed")
	}
	return path, nil
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "yt-dlp.exe"
	}
	return "yt-dlp"
}

func binDir(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		var err error
		dataDir, err = paths.Resolve("")
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dataDir, "bin"), nil
}

func versionFile(dataDir string) (string, error) {
	dir, err := binDir(dataDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "yt-dlp.version"), nil
}

func readVersion(dataDir string) string {
	path, err := versionFile(dataDir)
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// AssetName is the official release file for this operating system.
func AssetName(goos, goarch string) (string, error) {
	switch goos {
	case "darwin":
		return "yt-dlp_macos", nil
	case "windows":
		return "yt-dlp.exe", nil
	case "linux":
		if goarch == "arm64" || goarch == "arm" {
			return "yt-dlp_linux_aarch64", nil
		}
		return "yt-dlp_linux", nil
	default:
		return "", fmt.Errorf("yt-dlp is not published for %s", goos)
	}
}

func releaseURL(name string) string {
	return "https://github.com/yt-dlp/yt-dlp/releases/latest/download/" + name
}

func (in *Installer) Snapshot(dataDir string) Snapshot {
	path, ver := Status(dataDir)
	snap := Snapshot{Installed: path != "", Path: path, Version: ver}
	in.mu.Lock()
	defer in.mu.Unlock()
	snap.Phase = in.phase
	snap.Received = in.received
	snap.Total = in.total
	snap.Message = in.message
	snap.Error = in.errText
	if snap.Phase == "" {
		snap.Phase = "idle"
	}
	if snap.Installed && snap.Phase != "downloading" && snap.Phase != "installing" {
		if snap.Phase != "done" {
			snap.Phase = "idle"
		}
		if ver != "" {
			snap.Message = "yt-dlp " + ver + " is installed."
		} else {
			snap.Message = "yt-dlp is installed."
		}
		snap.Error = ""
	}
	return snap
}

// Start downloads the current release. Update replaces a binary that is already there.
func (in *Installer) Start(dataDir string, update bool) error {
	if _, err := AssetName(runtime.GOOS, runtime.GOARCH); err != nil {
		return err
	}
	if _, err := Executable(dataDir); err == nil && !update {
		return errors.New("yt-dlp is already installed")
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
	if update {
		in.message = "Downloading the current yt-dlp…"
	} else {
		in.message = "Downloading yt-dlp…"
	}
	go in.run(ctx, dataDir)
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

func (in *Installer) run(ctx context.Context, dataDir string) {
	name, err := AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		in.fail(err)
		return
	}
	dir, err := binDir(dataDir)
	if err != nil {
		in.fail(err)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		in.fail(err)
		return
	}
	tmp, err := os.CreateTemp(dir, "yt-dlp-download-*")
	if err != nil {
		in.fail(err)
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	err = fetchRelease(ctx, releaseURL(name), tmpPath, func(got, total int64) {
		in.mu.Lock()
		in.received = got
		in.total = total
		in.mu.Unlock()
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			in.finish("cancelled", "Download cancelled.", "")
			return
		}
		in.fail(err)
		return
	}
	in.mu.Lock()
	in.phase = "installing"
	in.message = "Checking yt-dlp…"
	in.mu.Unlock()

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		in.fail(err)
		return
	}
	clearQuarantine(tmpPath)
	ver, err := binaryVersion(ctx, tmpPath)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			in.finish("cancelled", "Download cancelled.", "")
			return
		}
		in.fail(err)
		return
	}
	dest := filepath.Join(dir, binaryName())
	if err := replaceFile(tmpPath, dest); err != nil {
		in.fail(err)
		return
	}
	versionPath, err := versionFile(dataDir)
	if err != nil {
		in.fail(err)
		return
	}
	if err := os.WriteFile(versionPath, []byte(ver+"\n"), 0o644); err != nil {
		in.fail(err)
		return
	}
	in.finish("done", "yt-dlp "+ver+" is installed.", "")
}

func (in *Installer) fail(err error) {
	in.finish("failed", "Could not install yt-dlp.", err.Error())
}

func (in *Installer) finish(phase, message, errText string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.phase = phase
	in.message = message
	in.errText = errText
	in.cancel = nil
}

var fetchRelease = downloadRelease

func downloadRelease(ctx context.Context, rawURL, dest string, report func(got, total int64)) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Marchive/"+version.Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("yt-dlp download returned HTTP %d", resp.StatusCode)
	}
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()
	var got int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				return err
			}
			got += int64(n)
			if report != nil {
				report(got, resp.ContentLength)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func binaryVersion(ctx context.Context, path string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cmdCtx, path, "--version").Output()
	if err != nil {
		return "", errors.New("the downloaded yt-dlp did not run")
	}
	ver := strings.TrimSpace(string(out))
	if ver == "" {
		return "", errors.New("the downloaded yt-dlp did not report a version")
	}
	if i := strings.IndexAny(ver, "\r\n"); i >= 0 {
		ver = strings.TrimSpace(ver[:i])
	}
	return ver, nil
}

func replaceFile(tmp, dest string) error {
	if err := os.Rename(tmp, dest); err == nil {
		return nil
	}
	_ = os.Remove(dest)
	return os.Rename(tmp, dest)
}

func clearQuarantine(path string) {
	if runtime.GOOS != "darwin" {
		return
	}
	_ = exec.Command("xattr", "-d", "com.apple.quarantine", path).Run()
}
