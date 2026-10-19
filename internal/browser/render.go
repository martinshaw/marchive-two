package browser

import (
	"context"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	renderQuiet = time.Second
	renderLimit = 15 * time.Second
)

// pageLoad tracks requests that can still change the document.
type pageLoad struct {
	mu       sync.Mutex
	inflight map[network.RequestID]struct{}
}

func (p *pageLoad) listen(ctx context.Context) {
	p.mu.Lock()
	if p.inflight == nil {
		p.inflight = map[network.RequestID]struct{}{}
	}
	p.mu.Unlock()
	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			if skipResource(ev.Type) {
				return
			}
			p.mu.Lock()
			p.inflight[ev.RequestID] = struct{}{}
			p.mu.Unlock()
		case *network.EventLoadingFinished:
			p.done(ev.RequestID)
		case *network.EventLoadingFailed:
			p.done(ev.RequestID)
		}
	})
}

func (p *pageLoad) done(id network.RequestID) {
	p.mu.Lock()
	delete(p.inflight, id)
	p.mu.Unlock()
}

func (p *pageLoad) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.inflight)
}

func skipResource(kind network.ResourceType) bool {
	switch kind {
	case network.ResourceTypeWebSocket, network.ResourceTypeEventSource, network.ResourceTypePing, network.ResourceTypeMedia, network.ResourceTypePrefetch:
		return true
	default:
		return false
	}
}

// waitReady polls script until it returns true, clicking through a consent
// dialog between checks. A page that stays blocked is returned as it is.
func waitReady(ctx context.Context, script string) error {
	deadline := time.Now().Add(45 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var ready bool
		if err := chromedp.Evaluate(script, &ready).Do(ctx); err != nil {
			return err
		}
		if ready || !time.Now().Before(deadline) {
			return nil
		}
		_ = chromedp.Evaluate(dismissCookieJS, nil).Do(ctx)
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// wait until the loaded page has stopped changing, so markup built by the
// page's own scripts is present before the snapshot.
func (p *pageLoad) wait(ctx context.Context) error {
	deadline := time.Now().Add(renderLimit)
	var state renderWait
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var snap renderSample
		if err := chromedp.Evaluate(renderProbeJS, &snap).Do(ctx); err != nil {
			return err
		}
		snap.Inflight = p.count()
		now := time.Now()
		if state.settled(snap, now) || !now.Before(deadline) {
			return nil
		}
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

const renderProbeJS = `(function () {
  var watch = window.__marchiveRender;
  if (!watch) {
    watch = window.__marchiveRender = { n: 0 };
    var obs = new MutationObserver(function () { watch.n++; });
    if (document.documentElement) {
      obs.observe(document.documentElement, { subtree: true, childList: true, characterData: true });
    }
  }
  var images = false;
  var list = document.images || [];
  for (var i = 0; i < list.length; i++) {
    if (!list[i].complete) { images = true; break; }
  }
  return { mutations: watch.n, images: images, ready: document.readyState === "complete" };
})()`

// renderSample is one look at the page while waiting for it to finish.
type renderSample struct {
	Mutations     int  `json:"mutations"`
	ImagesPending bool `json:"images"`
	Ready         bool `json:"ready"`
	Inflight      int  `json:"-"`
}

// renderWait remembers the previous look so a quiet stretch can be measured.
type renderWait struct {
	seen       bool
	mutations  int
	quietSince time.Time
}

func (w *renderWait) settled(sample renderSample, now time.Time) bool {
	stable := sample.Ready && !sample.ImagesPending && sample.Inflight == 0 && w.seen && sample.Mutations == w.mutations
	w.seen = true
	w.mutations = sample.Mutations
	if !stable {
		w.quietSince = time.Time{}
		return false
	}
	if w.quietSince.IsZero() {
		w.quietSince = now
		return false
	}
	return !now.Before(w.quietSince.Add(renderQuiet))
}
