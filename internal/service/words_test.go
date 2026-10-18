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
	item, err := svc.ItemWords(oneID, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "Desk notes" || countOf(item.Words, "archives") != 2 || countOf(item.Words, "secret") != 0 {
		t.Fatalf("item %+v", item)
	}
	run, err := svc.RunWords(first.ID, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if run.Title != "Desk" || run.ItemCount != 2 || countOf(run.Words, "archives") != 3 || countOf(run.Words, "later") != 1 {
		t.Fatalf("run %+v", run)
	}
	line, err := svc.WordTimeline(false, false)
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

func TestWordCloudDropsCommonWordsUnlessAsked(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	archive := filepath.Join(dir, "archive")
	sourceID, scheduleID, err := st.CreateSourceWithSchedule(store.Source{
		Name: "Desk", URL: "https://example.com", Provider: "webpage",
	}, store.Schedule{IntervalSeconds: 3600, DownloadLocation: archive, Enabled: true, MaxNew: 20})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	writeIndex(t, filepath.Join(archive, "one"), provider.WordIndex{
		Title: "Desk notes", WordCount: 11, UniqueCount: 2,
		Words: []provider.WordCount{{Word: "the", Count: 9}, {Word: "archives", Count: 2}},
	})
	id := insertWordItem(t, st, sourceID, run.ID, filepath.Join(archive, "one"), "one")
	svc := &Service{Store: st}
	cloud, err := svc.ItemWords(id, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if countOf(cloud.Words, "the") != 0 || countOf(cloud.Words, "archives") != 2 || cloud.WordCount != 2 {
		t.Fatalf("filtered %+v", cloud)
	}
	kept, err := svc.ItemWords(id, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if countOf(kept.Words, "the") != 9 {
		t.Fatalf("kept %+v", kept)
	}
	line, err := svc.WordTimeline(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if countOf(line.Words, "the") != 0 || line.Captures[0].Counts["the"] != 0 || line.Captures[0].Counts["archives"] != 2 {
		t.Fatalf("timeline %+v counts %+v", line.Words, line.Captures)
	}
}

func TestWordCloudDropsURLWordsUnlessAsked(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	archive := filepath.Join(dir, "archive")
	sourceID, scheduleID, err := st.CreateSourceWithSchedule(store.Source{
		Name: "The Guardian", URL: "https://www.theguardian.com/uk", Provider: "article_index",
	}, store.Schedule{IntervalSeconds: 3600, DownloadLocation: archive, Enabled: true, MaxNew: 20})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	writeIndex(t, filepath.Join(archive, "one"), provider.WordIndex{
		Title: "Desk notes", WordCount: 11, UniqueCount: 2,
		Words: []provider.WordCount{{Word: "guardian", Count: 9}, {Word: "archives", Count: 2}},
	})
	id, err := st.InsertItem(store.Item{
		SourceID: sourceID, RunID: run.ID, IdentityKey: "one", Kind: "article",
		Title: "Desk notes", URL: "https://www.theguardian.com/uk/story", Status: store.StatusSucceeded,
		DownloadLocation: filepath.Join(archive, "one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	otherArchive := filepath.Join(dir, "other")
	otherSource, otherSchedule, err := st.CreateSourceWithSchedule(store.Source{
		Name: "Desk", URL: "https://example.com", Provider: "webpage",
	}, store.Schedule{IntervalSeconds: 3600, DownloadLocation: otherArchive, Enabled: true, MaxNew: 20})
	if err != nil {
		t.Fatal(err)
	}
	otherRun, err := st.CreateRun(otherSchedule)
	if err != nil {
		t.Fatal(err)
	}
	writeIndex(t, filepath.Join(otherArchive, "mention"), provider.WordIndex{
		Title: "A mention", WordCount: 1, UniqueCount: 1,
		Words: []provider.WordCount{{Word: "guardian", Count: 1}},
	})
	if _, err := st.InsertItem(store.Item{
		SourceID: otherSource, RunID: otherRun.ID, IdentityKey: "mention", Kind: "snapshot",
		Title: "A mention", URL: "https://example.com/notes", Status: store.StatusSucceeded,
		DownloadLocation: filepath.Join(otherArchive, "mention"),
	}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st}
	cloud, err := svc.ItemWords(id, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if countOf(cloud.Words, "guardian") != 0 || countOf(cloud.Words, "archives") != 2 || cloud.WordCount != 2 {
		t.Fatalf("filtered %+v", cloud)
	}
	kept, err := svc.ItemWords(id, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if countOf(kept.Words, "guardian") != 9 {
		t.Fatalf("kept %+v", kept)
	}
	line, err := svc.WordTimeline(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if countOf(line.Words, "guardian") != 1 || line.Captures[0].Counts["guardian"] != 0 || line.Captures[0].Counts["archives"] != 2 {
		t.Fatalf("timeline %+v counts %+v", line.Words, line.Captures)
	}
	if len(line.Captures) != 2 || line.Captures[1].Counts["guardian"] != 1 {
		t.Fatalf("other site %+v", line.Captures)
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
