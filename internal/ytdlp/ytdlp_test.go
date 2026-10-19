package ytdlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	name, err := AssetName("darwin", "arm64")
	if err != nil || name != "yt-dlp_macos" {
		t.Fatalf("darwin %s %v", name, err)
	}
	name, err = AssetName("linux", "amd64")
	if err != nil || name != "yt-dlp_linux" {
		t.Fatalf("linux %s %v", name, err)
	}
	name, err = AssetName("linux", "arm64")
	if err != nil || name != "yt-dlp_linux_aarch64" {
		t.Fatalf("linux arm %s %v", name, err)
	}
	name, err = AssetName("windows", "amd64")
	if err != nil || name != "yt-dlp.exe" {
		t.Fatalf("windows %s %v", name, err)
	}
	if _, err := AssetName("plan9", "amd64"); err == nil {
		t.Fatal("expected an unsupported system")
	}
}

func TestDownloadReleaseReportsProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4")
		_, _ = w.Write([]byte("ytdl"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "yt-dlp")
	var got, total int64
	if err := downloadRelease(context.Background(), srv.URL, dest, func(g, n int64) {
		got, total = g, n
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil || string(raw) != "ytdl" || got != 4 || total != 4 {
		t.Fatalf("body %q progress %d %d err %v", raw, got, total, err)
	}
}

func TestExecutableMissing(t *testing.T) {
	if _, err := Executable(t.TempDir()); err == nil {
		t.Fatal("expected a missing binary")
	}
}
