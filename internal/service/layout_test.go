package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/store"
)

func TestNestSourceCapturesGroupsFlatItemDirs(t *testing.T) {
	archive := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := New(st, nil, nil)
	sourceID, scheduleID, err := st.CreateSourceWithSchedule(store.Source{
		Name: "BBC News Articles", URL: "https://www.bbc.co.uk/news", Provider: "article_index",
	}, store.Schedule{IntervalSeconds: 3600, DownloadLocation: archive, Enabled: true, MaxNew: 20})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	flat := filepath.Join(archive, "Alligator-kills-bee44ce0")
	if err := os.MkdirAll(flat, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flat, "text.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.InsertItem(store.Item{
		SourceID: sourceID, RunID: run.ID, IdentityKey: "one", Kind: "article",
		Title: "Alligator", URL: "https://example.com/one", Status: store.StatusSucceeded,
		DownloadLocation: flat,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.NestSourceCaptures(); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(archive, provider.SourceDirName(sourceID, "BBC News Articles"))
	moved := filepath.Join(parent, "Alligator-kills-bee44ce0")
	if _, err := os.Stat(filepath.Join(moved, "text.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(flat); !os.IsNotExist(err) {
		t.Fatal("flat directory still present")
	}
	item, err := st.GetItem(id)
	if err != nil {
		t.Fatal(err)
	}
	if item.DownloadLocation != moved {
		t.Fatalf("location %s", item.DownloadLocation)
	}
	if err := svc.NestSourceCaptures(); err != nil {
		t.Fatal(err)
	}
	if item, err = st.GetItem(id); err != nil || item.DownloadLocation != moved {
		t.Fatalf("second pass location %s %v", item.DownloadLocation, err)
	}
	if err := st.DeleteSchedule(scheduleID); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(archive, "2026-10-17T12-23-28Z-d49857fd")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	orphanID, err := st.InsertItem(store.Item{
		SourceID: sourceID, IdentityKey: "orphan", Kind: "snapshot",
		Title: "Snapshot", URL: "https://example.com/snap", Status: store.StatusSucceeded,
		DownloadLocation: orphan,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.NestSourceCaptures(); err != nil {
		t.Fatal(err)
	}
	orphanMoved := filepath.Join(parent, filepath.Base(orphan))
	got, err := st.GetItem(orphanID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DownloadLocation != orphanMoved {
		t.Fatalf("orphan location %s", got.DownloadLocation)
	}
	if _, err := os.Stat(orphanMoved); err != nil {
		t.Fatal(err)
	}
}
