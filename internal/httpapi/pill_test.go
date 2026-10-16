package httpapi

import (
	"testing"

	"github.com/martinshaw/marchive-two/internal/store"
)

func TestItemMeter(t *testing.T) {
	queued := itemMeter(store.StatusQueued)
	if queued.Percent != 0 || queued.Busy {
		t.Fatalf("queued %+v", queued)
	}
	running := itemMeter(store.StatusRunning)
	if !running.Busy {
		t.Fatalf("running %+v", running)
	}
	done := itemMeter(store.StatusSucceeded)
	if done.Percent != 100 || done.Busy {
		t.Fatalf("succeeded %+v", done)
	}
}

func TestRunMeter(t *testing.T) {
	waiting := runMeter(store.StatusRunning, nil)
	if !waiting.Busy {
		t.Fatalf("empty run %+v", waiting)
	}
	partial := runMeter(store.StatusRunning, []store.Item{
		{Status: store.StatusSucceeded},
		{Status: store.StatusRunning},
		{Status: store.StatusQueued},
		{Status: store.StatusFailed},
	})
	if partial.Busy || partial.Percent != 50 {
		t.Fatalf("partial %+v", partial)
	}
	finished := runMeter(store.StatusSucceeded, []store.Item{{Status: store.StatusSucceeded}})
	if finished.Percent != 100 || finished.Busy {
		t.Fatalf("finished %+v", finished)
	}
}
