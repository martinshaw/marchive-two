package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/martinshaw/marchive-two/internal/store"
)

func TestFinderProxyPointsAtArchive(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "My Folder")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := New(st, nil, nil)
	enabled := true
	if _, _, err := svc.CreateSource(SourceInput{
		URL:              "https://example.com/feed",
		Provider:         "podcast",
		IntervalSeconds:  3600,
		DownloadLocation: dir,
		MaxNew:           1,
		Enabled:          &enabled,
	}); err != nil {
		t.Fatal(err)
	}
	body, err := svc.FinderProxy(nested)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	want := fileURL(nested)
	if !strings.Contains(text, want) {
		t.Fatalf("proxy %s missing %s", text, want)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(text, "<key>URL</key>") {
		t.Fatalf("expected an internet location, got %s", text)
	}
	if _, err := svc.FinderProxy(t.TempDir()); err == nil {
		t.Fatal("expected a path outside the archive to be refused")
	}
}
