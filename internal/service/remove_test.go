package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martinshaw/marchive-two/internal/store"
)

func TestDeleteCanKeepOrRemoveItemFiles(t *testing.T) {
	archive := t.TempDir()
	outside := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := New(st, nil, nil)
	enabled := true
	sourceID, scheduleID, err := svc.CreateSource(SourceInput{
		URL:              "https://example.com/news",
		Provider:         "article_index",
		IntervalSeconds:  3600,
		DownloadLocation: archive,
		MaxNew:           5,
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}

	keep := writeItemDir(t, archive, "keep")
	keepID := insertItem(t, st, sourceID, run.ID, "keep", keep)
	if err := svc.DeleteItem(keepID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(keep, "text.txt")); err != nil {
		t.Fatal(err)
	}

	gone := writeItemDir(t, archive, "gone")
	goneID := insertItem(t, st, sourceID, run.ID, "gone", gone)
	if err := svc.DeleteItem(goneID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatalf("item folder still present: %v", err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatal(err)
	}

	rootItem := insertItem(t, st, sourceID, run.ID, "root", archive)
	if err := svc.DeleteItem(rootItem, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatal("archive root was removed")
	}

	escaped := writeItemDir(t, outside, "escaped")
	escapedID := insertItem(t, st, sourceID, run.ID, "escaped", escaped)
	if err := svc.DeleteItem(escapedID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(escaped); err != nil {
		t.Fatal("folder outside the archive was removed")
	}

	captureDir := writeItemDir(t, archive, "capture")
	insertItem(t, st, sourceID, run.ID, "capture", captureDir)
	if err := svc.DeleteRun(run.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(captureDir); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRun(run.ID); err == nil {
		t.Fatal("capture still present")
	}

	run, err = st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	sourceDir := writeItemDir(t, archive, "source")
	insertItem(t, st, sourceID, run.ID, "source", sourceDir)
	if err := svc.DeleteSource(sourceID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sourceDir); !os.IsNotExist(err) {
		t.Fatalf("source files still present: %v", err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("an earlier item folder was removed with the source")
	}
}

func writeItemDir(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "text.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func insertItem(t *testing.T, st *store.Store, sourceID, runID int64, key, dir string) int64 {
	t.Helper()
	id, err := st.InsertItem(store.Item{
		SourceID:         sourceID,
		RunID:            runID,
		IdentityKey:      key,
		Kind:             "article",
		Title:            key,
		URL:              "https://example.com/" + key,
		Status:           store.StatusSucceeded,
		DownloadLocation: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
