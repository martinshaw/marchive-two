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
	bad := 70000
	if err := svc.UpdateSettings(SettingsInput{ListenPort: &bad}); err == nil {
		t.Fatal("expected an invalid port error")
	}
	zero := 0
	if err := svc.UpdateSettings(SettingsInput{ListenPort: &zero}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.RunNow(scheduleID)
	var se *Error
	if !errors.As(err, &se) || se.Status != 409 {
		t.Fatalf("run now err = %v", err)
	}
}
