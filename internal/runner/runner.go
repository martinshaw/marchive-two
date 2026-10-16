// Package runner schedules captures and keeps a failed job from stopping the process.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/store"
)

// Runner pulls due schedules and runs them one at a time.
type Runner struct {
	store *store.Store
	reg   *provider.Registry
	fetch browser.Fetcher

	jobs chan job
	kick chan struct{}

	mu     sync.Mutex
	active map[int64]context.CancelFunc
	wg     sync.WaitGroup
}

type job struct {
	RunID      int64
	ScheduleID int64
}

func New(st *store.Store, reg *provider.Registry, fetch browser.Fetcher) *Runner {
	return &Runner{
		store:  st,
		reg:    reg,
		fetch:  fetch,
		jobs:   make(chan job, 32),
		kick:   make(chan struct{}, 1),
		active: map[int64]context.CancelFunc{},
	}
}

// Start runs the scheduler and the worker until ctx is cancelled.
func (r *Runner) Start(ctx context.Context) {
	r.wg.Add(2)
	go r.loop(ctx)
	go r.worker(ctx)
}

// Shutdown waits for the scheduler and worker to leave.
func (r *Runner) Shutdown() {
	r.wg.Wait()
}

// Kick asks the scheduler to look for due work.
func (r *Runner) Kick() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// SetEnabled persists the runner flag. Stopping cancels queued runs and leaves
// the active run to finish or be cancelled on its own.
func (r *Runner) SetEnabled(on bool) error {
	if err := r.store.SetRunnerEnabled(on); err != nil {
		return err
	}
	if !on {
		ids, err := r.store.ListRunIDsByStatus(store.StatusQueued)
		if err != nil {
			return err
		}
		for _, id := range ids {
			_ = r.Cancel(id)
		}
	}
	r.Kick()
	return nil
}

// RunNow queues a schedule even when the scheduler is stopped.
func (r *Runner) RunNow(scheduleID int64) (int64, error) {
	if _, err := r.store.GetSchedule(scheduleID); err != nil {
		return 0, err
	}
	active, err := r.store.HasActiveRun(scheduleID)
	if err != nil {
		return 0, err
	}
	if active {
		return 0, errors.New("this schedule already has a queued or running capture")
	}
	run, err := r.store.CreateRun(scheduleID)
	if err != nil {
		return 0, err
	}
	select {
	case r.jobs <- job{RunID: run.ID, ScheduleID: scheduleID}:
		return run.ID, nil
	default:
		_, _ = r.store.CompleteRun(run.ID, scheduleID, store.StatusFailed, "runner queue is full", "", time.Now())
		return 0, errors.New("runner queue is full")
	}
}

// Cancel stops a queued or running capture.
func (r *Runner) Cancel(id int64) error {
	run, err := r.store.GetRun(id)
	if err != nil {
		return err
	}
	switch run.Status {
	case store.StatusQueued:
		updated, err := r.store.CompleteRun(id, run.ScheduleID, store.StatusCancelled, "", "Cancelled before it started.", time.Now())
		if err != nil {
			return err
		}
		if updated {
			r.Kick()
			return nil
		}
		return r.cancelActive(id)
	case store.StatusRunning:
		return r.cancelActive(id)
	default:
		return fmt.Errorf("run %d is already %s", id, run.Status)
	}
}

func (r *Runner) cancelActive(id int64) error {
	r.mu.Lock()
	cancel := r.active[id]
	r.mu.Unlock()
	if cancel == nil {
		return errors.New("that capture is no longer running")
	}
	cancel()
	return nil
}

func (r *Runner) loop(ctx context.Context) {
	defer r.wg.Done()
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		enabled, err := r.store.RunnerEnabled()
		if err != nil {
			slog.Error("read runner flag", "err", err)
		} else if enabled {
			r.enqueueDue()
		}
		delay := 24 * time.Hour
		if enabled {
			if wait, ok := r.store.NextWake(time.Now()); ok {
				// An overdue schedule that could not be queued must not spin the loop.
				// A finished capture kicks the scheduler immediately.
				if wait <= 0 {
					wait = 2 * time.Second
				}
				delay = wait
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(delay)
		select {
		case <-ctx.Done():
			return
		case <-r.kick:
		case <-timer.C:
		}
	}
}

func (r *Runner) enqueueDue() {
	due, err := r.store.DueSchedules(time.Now())
	if err != nil {
		slog.Error("list due schedules", "err", err)
		return
	}
	for _, detail := range due {
		run, err := r.store.CreateRun(detail.ID)
		if err != nil {
			slog.Error("create run", "schedule", detail.ID, "err", err)
			continue
		}
		select {
		case r.jobs <- job{RunID: run.ID, ScheduleID: detail.ID}:
		default:
			_, _ = r.store.CompleteRun(run.ID, detail.ID, store.StatusFailed, "runner queue is full", "", time.Now())
		}
	}
}

func (r *Runner) worker(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-r.jobs:
			r.execute(ctx, job)
		}
	}
}

func (r *Runner) execute(processCtx context.Context, job job) {
	finished := false
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("capture panicked", "run", job.RunID, "panic", rec)
			if !finished {
				r.finish(job, store.StatusFailed, fmt.Sprintf("panic: %v", rec), "")
			}
		}
	}()

	claimed, err := r.store.ClaimRun(job.RunID)
	if err != nil {
		slog.Error("claim run", "run", job.RunID, "err", err)
		return
	}
	if !claimed {
		return
	}

	runCtx, userCancel := context.WithCancel(processCtx)
	runCtx, timeoutCancel := context.WithTimeout(runCtx, 30*time.Minute)
	defer timeoutCancel()
	defer userCancel()
	r.track(job.RunID, userCancel)
	defer r.untrack(job.RunID)

	detail, err := r.store.GetSchedule(job.ScheduleID)
	if err != nil {
		finished = true
		r.finish(job, store.StatusFailed, err.Error(), "")
		return
	}
	impl, ok := r.reg.Get(detail.Source.Provider)
	if !ok {
		finished = true
		r.finish(job, store.StatusFailed, "unknown provider "+detail.Source.Provider, "")
		return
	}
	slots, _ := r.store.PodcastConcurrency()
	slog.Info("capture started", "run", job.RunID, "source", detail.Source.Name, "provider", detail.Source.Provider)
	summary, err := impl.Run(runCtx, provider.Request{
		Source:             detail.Source,
		Schedule:           detail.Schedule,
		RunID:              job.RunID,
		Store:              r.store,
		Fetch:              r.fetch,
		PodcastConcurrency: slots,
	})
	finished = true
	switch {
	case errors.Is(err, context.Canceled):
		_ = r.store.CancelOpenItems(job.RunID)
		r.finish(job, store.StatusCancelled, "", noteOr(summary.Note, "Cancelled."))
	case err != nil:
		_ = r.store.FailOpenItems(job.RunID, err.Error())
		r.finish(job, store.StatusFailed, err.Error(), summary.Note)
	case summary.Failed:
		r.finish(job, store.StatusFailed, summary.Note, summary.Note)
	default:
		r.finish(job, store.StatusSucceeded, "", summary.Note)
	}
}

func (r *Runner) finish(job job, status, errMsg, note string) {
	updated, err := r.store.CompleteRun(job.RunID, job.ScheduleID, status, errMsg, note, time.Now())
	if err != nil {
		slog.Error("finish run", "run", job.RunID, "err", err)
		return
	}
	if !updated {
		return
	}
	slog.Info("capture finished", "run", job.RunID, "status", status)
	r.Kick()
}

func (r *Runner) track(id int64, cancel context.CancelFunc) {
	r.mu.Lock()
	r.active[id] = cancel
	r.mu.Unlock()
}

func (r *Runner) untrack(id int64) {
	r.mu.Lock()
	delete(r.active, id)
	r.mu.Unlock()
}

func noteOr(note, fallback string) string {
	if note == "" {
		return fallback
	}
	return note
}
