package service

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/martinshaw/marchive-two/internal/store"
)

func TestCreateSourceValidates(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := New(st, nil, nil)
	_, _, err = svc.CreateSource(SourceInput{
		URL:              "example.com/feed",
		Provider:         "nope",
		IntervalSeconds:  60,
		DownloadLocation: t.TempDir(),
		MaxNew:           5,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	enabled := true
	id, scheduleID, err := svc.CreateSource(SourceInput{
		URL:              "podcasts.example/feed",
		Provider:         "podcast",
		IntervalSeconds:  3600,
		DownloadLocation: t.TempDir(),
		MaxNew:           5,
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 || scheduleID == 0 {
		t.Fatalf("ids %d %d", id, scheduleID)
	}
	src, err := st.GetSource(id)
	if err != nil {
		t.Fatal(err)
	}
	if src.URL != "https://podcasts.example/feed" {
		t.Fatalf("url %s", src.URL)
	}
	port := 8080
	if err := svc.UpdateSettings(SettingsInput{ListenPort: &port}); err != nil {
		t.Fatal(err)
	}
	state, err := svc.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.ListenPort != 8080 {
		t.Fatalf("listen port %d", state.ListenPort)
	}
	if state.ItemTimeoutSeconds != 0 || state.CaptureTimeoutSeconds != 0 || state.ItemTries != 1 || state.CaptureTries != 1 {
		t.Fatalf("default limits %+v", state)
	}
	bad := 70000
	if err := svc.UpdateSettings(SettingsInput{ListenPort: &bad}); err == nil {
		t.Fatal("expected an invalid port error")
	}
	zero := 0
	if err := svc.UpdateSettings(SettingsInput{ListenPort: &zero}); err != nil {
		t.Fatal(err)
	}
	selector := `div[data-testid="newswire-blocks-container"] a`
	_, articleSchedule, err := svc.CreateSource(SourceInput{
		URL:              "https://www.rockstargames.com/newswire",
		Provider:         "article_index",
		LinkSelector:     "main a",
		IntervalSeconds:  3600,
		DownloadLocation: t.TempDir(),
		MaxNew:           5,
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateSchedule(articleSchedule, ScheduleInput{LinkSelector: &selector}); err != nil {
		t.Fatal(err)
	}
	detail, err := st.GetSchedule(articleSchedule)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Source.LinkSelector != selector {
		t.Fatalf("selector %q", detail.Source.LinkSelector)
	}
	badSelector := "div[a"
	if err := svc.UpdateSchedule(articleSchedule, ScheduleInput{LinkSelector: &badSelector}); err == nil {
		t.Fatal("expected an invalid selector error")
	}
	itemTimeout, captureTimeout, itemTries, captureTries := 30, 600, 4, 2
	if err := svc.UpdateSettings(SettingsInput{
		ItemTimeoutSeconds:    &itemTimeout,
		CaptureTimeoutSeconds: &captureTimeout,
		ItemTries:             &itemTries,
		CaptureTries:          &captureTries,
	}); err != nil {
		t.Fatal(err)
	}
	state, err = svc.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.ItemTimeoutSeconds != 30 || state.CaptureTimeoutSeconds != 600 || state.ItemTries != 4 || state.CaptureTries != 2 {
		t.Fatalf("limits %+v", state)
	}
	tooMany := 11
	if err := svc.UpdateSettings(SettingsInput{CaptureTries: &tooMany}); err == nil {
		t.Fatal("expected an invalid capture attempt count")
	}
	negative := -1
	if err := svc.UpdateSettings(SettingsInput{ItemTimeoutSeconds: &negative}); err == nil {
		t.Fatal("expected an invalid item time limit")
	}
	_, err = svc.RunNow(scheduleID)
	var se *Error
	if !errors.As(err, &se) || se.Status != 409 {
		t.Fatalf("run now err = %v", err)
	}
}
