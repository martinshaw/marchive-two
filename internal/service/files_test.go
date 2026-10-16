package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martinshaw/marchive-two/internal/store"
)

func TestItemMessageListsFilesAndRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	archive := filepath.Join(dir, "archive")
	sourceID, _, err := st.CreateSourceWithSchedule(store.Source{
		Name: "Desk", URL: "https://example.com", Provider: "webpage",
	}, store.Schedule{IntervalSeconds: 3600, DownloadLocation: archive, Enabled: true, MaxNew: 20})
	if err != nil {
		t.Fatal(err)
	}
	itemDir := filepath.Join(archive, "one")
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(itemDir, "text.txt"), []byte("Readable sentence."), 0o644)
	os.WriteFile(filepath.Join(itemDir, "page.html"), []byte("<p>Hello</p>"), 0o644)
	os.WriteFile(filepath.Join(itemDir, "screenshot.jpg"), []byte("img"), 0o644)
	id, err := st.InsertItem(store.Item{
		SourceID: sourceID, IdentityKey: "one", Kind: "snapshot", Title: "Desk notes",
		URL: "https://example.com/notes", Status: store.StatusSucceeded, DownloadLocation: itemDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st}
	msg, err := svc.ItemMessage(id)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Title != "Desk notes" || msg.SourceName != "Desk" || len(msg.Files) != 3 || msg.Files[0].Name != "text.txt" {
		t.Fatalf("message %+v", msg)
	}
	path, err := svc.ItemFilePath(id, "text.txt")
	if err != nil || !stringsHasSuffix(path, "text.txt") {
		t.Fatalf("path %s %v", path, err)
	}
	if _, err := svc.ItemFilePath(id, "../marchive.db"); err == nil {
		t.Fatal("expected a path escape to fail")
	}
	outside := filepath.Join(dir, "outside")
	os.MkdirAll(outside, 0o755)
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("no"), 0o644)
	other, err := st.InsertItem(store.Item{
		SourceID: sourceID, IdentityKey: "out", Kind: "snapshot", Title: "Outside",
		Status: store.StatusSucceeded, DownloadLocation: outside,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ItemMessage(other); err == nil {
		t.Fatal("expected a folder outside the archive to fail")
	}
}

func stringsHasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
