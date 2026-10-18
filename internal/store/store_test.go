package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSourceScheduleRunAndDedup(t *testing.T) {
	st := openTest(t)
	sourceID, scheduleID, err := st.CreateSourceWithSchedule(Source{
		Name:     "Example",
		URL:      "https://example.com/feed",
		Provider: "podcast",
	}, Schedule{
		IntervalSeconds:  3600,
		DownloadLocation: t.TempDir(),
		Enabled:          true,
		MaxNew:           20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sourceID == 0 || scheduleID == 0 {
		t.Fatalf("ids source=%d schedule=%d", sourceID, scheduleID)
	}

	due, err := st.DueSchedules(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != scheduleID {
		t.Fatalf("due = %+v", due)
	}

	run, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	due, err = st.DueSchedules(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("schedule with a queued run should not be due: %+v", due)
	}

	claimed, err := st.ClaimRun(run.ID)
	if err != nil || !claimed {
		t.Fatalf("claim = %v %v", claimed, err)
	}
	claimed, err = st.ClaimRun(run.ID)
	if err != nil || claimed {
		t.Fatalf("second claim = %v %v", claimed, err)
	}

	id, err := st.InsertItem(Item{
		SourceID:    sourceID,
		RunID:       run.ID,
		IdentityKey: "ep-1",
		Kind:        "episode",
		Title:       "One",
		URL:         "https://cdn.example/one.mp3",
		Status:      StatusSucceeded,
	})
	if err != nil || id == 0 {
		t.Fatal(err)
	}
	if _, err := st.InsertItem(Item{SourceID: sourceID, IdentityKey: "ep-1", Kind: "episode", Status: StatusSucceeded}); err != ErrExists {
		t.Fatalf("duplicate err = %v", err)
	}
	seen, err := st.ItemSeen(sourceID, "https://cdn.example/one.mp3")
	if err != nil || !seen {
		t.Fatalf("seen url = %v %v", seen, err)
	}

	updated, err := st.FinishRun(run.ID, StatusSucceeded, "", "1 new")
	if err != nil || !updated {
		t.Fatalf("finish = %v %v", updated, err)
	}
	if err := st.AdvanceSchedule(scheduleID, time.Now()); err != nil {
		t.Fatal(err)
	}
	due, err = st.DueSchedules(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("advanced schedule still due: %+v", due)
	}

	if err := st.DeleteSource(sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSource(sourceID); err != ErrNotFound {
		t.Fatalf("deleted source err = %v", err)
	}
	if _, err := st.GetRun(run.ID); err != ErrNotFound {
		t.Fatalf("cascaded run err = %v", err)
	}
}

func TestFailInterruptedAndSettings(t *testing.T) {
	st := openTest(t)
	_, scheduleID, err := st.CreateSourceWithSchedule(Source{
		Name: "Page", URL: "https://example.com", Provider: "webpage",
	}, Schedule{IntervalSeconds: 60, DownloadLocation: t.TempDir(), Enabled: true, MaxNew: 1})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertItem(Item{
		SourceID: 1, RunID: run.ID, IdentityKey: "h", Kind: "snapshot", Status: StatusRunning,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.FailInterrupted(); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.Error == "" {
		t.Fatalf("run = %+v", got)
	}

	if err := st.SetRunnerEnabled(true); err != nil {
		t.Fatal(err)
	}
	on, err := st.RunnerEnabled()
	if err != nil || !on {
		t.Fatalf("runner = %v %v", on, err)
	}
	if err := st.SetPodcastConcurrency(9); err != nil {
		t.Fatal(err)
	}
	n, err := st.PodcastConcurrency()
	if err != nil || n != 6 {
		t.Fatalf("podcast concurrency = %d %v", n, err)
	}
	gotPort, err := st.ListenPort()
	if err != nil || gotPort != 0 {
		t.Fatalf("default port = %d %v", gotPort, err)
	}
	if err := st.SetListenPort(8080); err != nil {
		t.Fatal(err)
	}
	gotPort, err = st.ListenPort()
	if err != nil || gotPort != 8080 {
		t.Fatalf("port = %d %v", gotPort, err)
	}
	if err := st.SetListenPort(0); err != nil {
		t.Fatal(err)
	}
	n, err = st.PodcastConcurrency()
	if err != nil || n != 6 {
		t.Fatalf("concurrency = %d %v", n, err)
	}
}

func TestOnceScheduleTurnsOff(t *testing.T) {
	st := openTest(t)
	_, scheduleID, err := st.CreateSourceWithSchedule(Source{
		Name: "Page", URL: "https://example.com", Provider: "webpage",
	}, Schedule{IntervalSeconds: 0, DownloadLocation: t.TempDir(), Enabled: true, MaxNew: 1})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := st.CompleteRun(run.ID, scheduleID, StatusSucceeded, "", "once", time.Now())
	if err != nil || !updated {
		t.Fatalf("complete = %v %v", updated, err)
	}
	got, err := st.GetSchedule(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.NextRunAt != nil || got.EveryUnit() != "once" {
		t.Fatalf("schedule = %+v", got)
	}
	due, err := st.DueSchedules(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("once schedule still due: %+v", due)
	}
}

func TestLatestHash(t *testing.T) {
	st := openTest(t)
	sourceID, _, err := st.CreateSourceWithSchedule(Source{
		Name: "Page", URL: "https://example.com", Provider: "webpage",
	}, Schedule{IntervalSeconds: 60, DownloadLocation: t.TempDir(), Enabled: true, MaxNew: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertItem(Item{SourceID: sourceID, IdentityKey: "aaa", Kind: "snapshot", Status: StatusSucceeded, ContentHash: "aaa"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertItem(Item{SourceID: sourceID, IdentityKey: "bbb", Kind: "snapshot", Status: StatusSucceeded, ContentHash: "bbb"}); err != nil {
		t.Fatal(err)
	}
	hash, ok, err := st.LatestContentHash(sourceID, "snapshot")
	if err != nil || !ok || hash != "bbb" {
		t.Fatalf("hash = %q ok=%v err=%v", hash, ok, err)
	}
}

func TestSetItemsRead(t *testing.T) {
	st := openTest(t)
	sourceID, _, err := st.CreateSourceWithSchedule(Source{
		Name: "Desk", URL: "https://example.com", Provider: "webpage",
	}, Schedule{IntervalSeconds: 60, DownloadLocation: t.TempDir(), Enabled: true, MaxNew: 1})
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.InsertItem(Item{SourceID: sourceID, IdentityKey: "one", Kind: "snapshot", Status: StatusSucceeded})
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.GetItem(id)
	if err != nil || !item.ReadAt.IsZero() {
		t.Fatalf("new item readAt %v %v", item.ReadAt, err)
	}
	if err := st.SetItemsRead([]int64{id}, true); err != nil {
		t.Fatal(err)
	}
	item, err = st.GetItem(id)
	if err != nil || item.ReadAt.IsZero() {
		t.Fatalf("read %v %v", item.ReadAt, err)
	}
	if err := st.SetItemsRead([]int64{id}, false); err != nil {
		t.Fatal(err)
	}
	item, err = st.GetItem(id)
	if err != nil || !item.ReadAt.IsZero() {
		t.Fatalf("unread %v %v", item.ReadAt, err)
	}
}

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}
