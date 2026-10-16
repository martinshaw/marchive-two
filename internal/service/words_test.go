package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/store"
)

func TestWordCloudsAggregateItemsAndCaptures(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	archive := filepath.Join(dir, "archive")
	sourceID, scheduleID, err := st.CreateSourceWithSchedule(store.Source{
		Name: "Desk", URL: "https://example.com", Provider: "webpage",
	}, store.Schedule{
		IntervalSeconds: 3600, DownloadLocation: archive, Enabled: true, MaxNew: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	writeIndex(t, filepath.Join(archive, "one"), provider.WordIndex{
		Title: "Desk notes", WordCount: 3, UniqueCount: 2,
		Words: []provider.WordCount{{Word: "archives", Count: 2}, {Word: "desk", Count: 1}},
	})
	writeIndex(t, filepath.Join(archive, "two"), provider.WordIndex{
		Title: "Later", WordCount: 2, UniqueCount: 2,
		Words: []provider.WordCount{{Word: "archives", Count: 1}, {Word: "later", Count: 1}},
	})
	outside := filepath.Join(dir, "outside")
	writeIndex(t, outside, provider.WordIndex{
		Title: "Secret", Words: []provider.WordCount{{Word: "secret", Count: 9}},
	})
	oneID := insertWordItem(t, st, sourceID, first.ID, filepath.Join(archive, "one"), "one")
	insertWordItem(t, st, sourceID, first.ID, filepath.Join(archive, "two"), "two")
	insertWordItem(t, st, sourceID, second.ID, outside, "outside")

	svc := &Service{Store: st}
	item, err := svc.ItemWords(oneID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "Desk notes" || countOf(item.Words, "archives") != 2 || countOf(item.Words, "secret") != 0 {
		t.Fatalf("item %+v", item)
	}
	run, err := svc.RunWords(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Title != "Desk" || run.ItemCount != 2 || countOf(run.Words, "archives") != 3 || countOf(run.Words, "later") != 1 {
		t.Fatalf("run %+v", run)
	}
	line, err := svc.WordTimeline()
	if err != nil {
		t.Fatal(err)
	}
	if line.Title != "All captures" || countOf(line.Words, "archives") != 3 || countOf(line.Words, "secret") != 0 {
		t.Fatalf("timeline %+v", line)
	}
	if len(line.Captures) != 1 || line.Captures[0].RunID != first.ID || line.Captures[0].Counts["archives"] != 3 {
		t.Fatalf("captures %+v", line.Captures)
	}
}

func writeIndex(t *testing.T, dir string, index provider.WordIndex) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "words.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func insertWordItem(t *testing.T, st *store.Store, sourceID, runID int64, dir, key string) int64 {
	t.Helper()
	id, err := st.InsertItem(store.Item{
		SourceID: sourceID, RunID: runID, IdentityKey: key, Kind: "snapshot",
		Title: key, Status: store.StatusSucceeded, DownloadLocation: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func countOf(words []provider.WordCount, word string) int {
	for _, item := range words {
		if item.Word == word {
			return item.Count
		}
	}
	return 0
}
