package runner

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/store"
)

type scripted struct {
	kind string
	run  func(ctx context.Context, req provider.Request) (provider.Summary, error)
}

func (s scripted) Kind() string { return s.kind }

func (s scripted) Run(ctx context.Context, req provider.Request) (provider.Summary, error) {
	return s.run(ctx, req)
}

func TestRunnerIsolatesPanicAndCancel(t *testing.T) {
	st := openStore(t)
	reg := provider.NewRegistry()
	reg.Register(scripted{kind: provider.KindWebpage, run: func(context.Context, provider.Request) (provider.Summary, error) {
		panic("boom")
	}})
	reg.Register(scripted{kind: provider.KindPodcast, run: func(ctx context.Context, _ provider.Request) (provider.Summary, error) {
		<-ctx.Done()
		return provider.Summary{}, ctx.Err()
	}})
	reg.Register(scripted{kind: provider.KindText, run: func(context.Context, provider.Request) (provider.Summary, error) {
		return provider.Summary{Note: "ok"}, nil
	}})

	runr := New(st, reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runr.Start(ctx)
	defer func() {
		cancel()
		runr.Shutdown()
	}()

	panicSchedule := add(t, st, provider.KindWebpage, time.Hour)
	panicID, err := runr.RunNow(panicSchedule)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, st, panicID, store.StatusFailed)

	blockSchedule := add(t, st, provider.KindPodcast, time.Hour)
	blockID, err := runr.RunNow(blockSchedule)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, st, blockID, store.StatusRunning)
	if err := runr.Cancel(blockID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, st, blockID, store.StatusCancelled)

	okSchedule := add(t, st, provider.KindText, time.Hour)
	if err := st.SetRunnerEnabled(true); err != nil {
		t.Fatal(err)
	}
	runr.Kick()
	deadline := time.Now().Add(3 * time.Second)
	var succeeded bool
	for time.Now().Before(deadline) {
		runs, err := st.ListRunDetails(20)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			if run.ScheduleID == okSchedule && run.Status == store.StatusSucceeded && run.Note == "ok" {
				succeeded = true
			}
		}
		if succeeded {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !succeeded {
		t.Fatal("scheduled text capture did not succeed after the panic")
	}
}

func TestStopCancelsQueued(t *testing.T) {
	st := openStore(t)
	started := make(chan struct{})
	release := make(chan struct{})
	reg := provider.NewRegistry()
	reg.Register(scripted{kind: provider.KindWebpage, run: func(ctx context.Context, _ provider.Request) (provider.Summary, error) {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return provider.Summary{}, ctx.Err()
		}
		return provider.Summary{Note: "done"}, nil
	}})
	runr := New(st, reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runr.Start(ctx)
	defer func() {
		close(release)
		cancel()
		runr.Shutdown()
	}()

	first := add(t, st, provider.KindWebpage, time.Hour)
	second := add(t, st, provider.KindWebpage, time.Hour)
	if _, err := runr.RunNow(first); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first run did not start")
	}
	queuedID, err := runr.RunNow(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := runr.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, st, queuedID, store.StatusCancelled)
	got, err := st.GetRun(queuedID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusCancelled {
		t.Fatalf("queued run status %s", got.Status)
	}
}

func TestCaptureTriesOnceByDefault(t *testing.T) {
	st := openStore(t)
	calls := 0
	reg := provider.NewRegistry()
	reg.Register(scripted{kind: provider.KindWebpage, run: func(context.Context, provider.Request) (provider.Summary, error) {
		calls++
		return provider.Summary{}, errors.New("still down")
	}})
	runr := New(st, reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runr.Start(ctx)
	defer func() {
		cancel()
		runr.Shutdown()
	}()
	id, err := runr.RunNow(add(t, st, provider.KindWebpage, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, st, id, store.StatusFailed)
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestCaptureAttemptsStopAfterSuccess(t *testing.T) {
	st := openStore(t)
	if err := st.SetCaptureTries(3); err != nil {
		t.Fatal(err)
	}
	if err := st.SetItemTries(4); err != nil {
		t.Fatal(err)
	}
	if err := st.SetItemTimeoutSeconds(12); err != nil {
		t.Fatal(err)
	}
	calls := 0
	reg := provider.NewRegistry()
	reg.Register(scripted{kind: provider.KindWebpage, run: func(ctx context.Context, _ provider.Request) (provider.Summary, error) {
		calls++
		timeout, tries := provider.ItemBudget(ctx)
		if timeout != 12*time.Second || tries != 4 {
			return provider.Summary{}, errors.New("limits were not passed to the capture")
		}
		if calls < 2 {
			return provider.Summary{}, errors.New("not yet")
		}
		return provider.Summary{Note: "ok"}, nil
	}})
	runr := New(st, reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runr.Start(ctx)
	defer func() {
		cancel()
		runr.Shutdown()
	}()
	id, err := runr.RunNow(add(t, st, provider.KindWebpage, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := waitStatus(t, st, id, store.StatusSucceeded)
	if got.Note != "ok" || calls != 2 {
		t.Fatalf("note %q calls %d", got.Note, calls)
	}
}

func TestCaptureTimeoutUsesEachAttempt(t *testing.T) {
	st := openStore(t)
	if err := st.SetCaptureTimeoutSeconds(1); err != nil {
		t.Fatal(err)
	}
	if err := st.SetCaptureTries(2); err != nil {
		t.Fatal(err)
	}
	calls := 0
	reg := provider.NewRegistry()
	reg.Register(scripted{kind: provider.KindWebpage, run: func(ctx context.Context, _ provider.Request) (provider.Summary, error) {
		calls++
		<-ctx.Done()
		return provider.Summary{}, ctx.Err()
	}})
	runr := New(st, reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runr.Start(ctx)
	defer func() {
		cancel()
		runr.Shutdown()
	}()
	id, err := runr.RunNow(add(t, st, provider.KindWebpage, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Second)
	var last store.Run
	for time.Now().Before(deadline) {
		last, err = st.GetRun(id)
		if err != nil {
			t.Fatal(err)
		}
		if last.Status == store.StatusFailed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if last.Status != store.StatusFailed || calls != 2 {
		t.Fatalf("status %s calls %d err %s", last.Status, calls, last.Error)
	}
}

func TestCancelDoesNotRetryCapture(t *testing.T) {
	st := openStore(t)
	if err := st.SetCaptureTries(3); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	calls := 0
	reg := provider.NewRegistry()
	reg.Register(scripted{kind: provider.KindWebpage, run: func(ctx context.Context, _ provider.Request) (provider.Summary, error) {
		calls++
		if calls == 1 {
			close(started)
		}
		<-ctx.Done()
		return provider.Summary{}, ctx.Err()
	}})
	runr := New(st, reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runr.Start(ctx)
	defer func() {
		cancel()
		runr.Shutdown()
	}()
	id, err := runr.RunNow(add(t, st, provider.KindWebpage, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("capture did not start")
	}
	if err := runr.Cancel(id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, st, id, store.StatusCancelled)
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func add(t *testing.T, st *store.Store, kind string, every time.Duration) int64 {
	t.Helper()
	_, scheduleID, err := st.CreateSourceWithSchedule(store.Source{
		Name: kind, URL: "https://example.com/" + kind, Provider: kind,
	}, store.Schedule{
		IntervalSeconds:  int(every.Seconds()),
		DownloadLocation: t.TempDir(),
		Enabled:          true,
		MaxNew:           5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return scheduleID
}

func waitStatus(t *testing.T, st *store.Store, id int64, status string) store.Run {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last store.Run
	for time.Now().Before(deadline) {
		run, err := st.GetRun(id)
		if err != nil {
			t.Fatal(err)
		}
		last = run
		if run.Status == status {
			return run
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("run %d status %s, want %s", id, last.Status, status)
	return store.Run{}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}
