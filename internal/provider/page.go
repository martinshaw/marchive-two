package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/store"
)

// Webpage stores a new snapshot when the page text changes.
type Webpage struct{}

func (Webpage) Kind() string { return KindWebpage }

func (Webpage) Run(ctx context.Context, req Request) (Summary, error) {
	capture := Resolve(req.Source.Provider, req.Source.Capture)
	doc, hash, err := fetchHashed(ctx, req, browser.Options{Screenshot: capture.Screenshot, MHTML: capture.MHTML, Scroll: true})
	if err != nil {
		return Summary{}, err
	}
	latest, ok, err := req.Store.LatestContentHash(req.Source.ID, "snapshot")
	if err != nil {
		return Summary{}, err
	}
	if ok && latest == hash {
		return Summary{Note: "Unchanged since the last snapshot."}, nil
	}
	seen, err := req.Store.ItemSeen(req.Source.ID, hash)
	if err != nil {
		return Summary{}, err
	}
	if seen {
		return Summary{Note: "Already archived an identical snapshot."}, nil
	}
	item, err := saveSnapshot(req, doc, "snapshot", hash, doc.URL, capture.mode())
	if errors.Is(err, store.ErrExists) {
		return Summary{Note: "Already archived an identical snapshot."}, nil
	}
	if err != nil {
		return Summary{}, err
	}
	maybeRenameSource(req, item.Title)
	return Summary{Note: "Saved a new snapshot."}, nil
}

// Text stores readable text and metadata when the page changes.
type Text struct{}

func (Text) Kind() string { return KindText }

func (Text) Run(ctx context.Context, req Request) (Summary, error) {
	capture := Resolve(req.Source.Provider, req.Source.Capture)
	doc, hash, err := fetchHashed(ctx, req, browser.Options{Scroll: true})
	if err != nil {
		return Summary{}, err
	}
	latest, ok, err := req.Store.LatestContentHash(req.Source.ID, "text")
	if err != nil {
		return Summary{}, err
	}
	if ok && latest == hash {
		return Summary{Note: "Unchanged since the last extract."}, nil
	}
	seen, err := req.Store.ItemSeen(req.Source.ID, hash)
	if err != nil {
		return Summary{}, err
	}
	if seen {
		return Summary{Note: "Already archived an identical extract."}, nil
	}
	item, err := saveSnapshot(req, doc, "text", hash, doc.URL, capture.mode())
	if errors.Is(err, store.ErrExists) {
		return Summary{Note: "Already archived an identical extract."}, nil
	}
	if err != nil {
		return Summary{}, err
	}
	maybeRenameSource(req, item.Title)
	return Summary{Note: "Saved a new text extract."}, nil
}

func fetchHashed(ctx context.Context, req Request, opt browser.Options) (*browser.Document, string, error) {
	if err := requireFetch(req.Fetch); err != nil {
		return nil, "", err
	}
	var doc *browser.Document
	_, err := withRetry(ctx, 1, func(ctx context.Context) error {
		var fetchErr error
		doc, fetchErr = req.Fetch.Fetch(ctx, req.Source.URL, opt)
		return fetchErr
	})
	if err != nil {
		return nil, "", err
	}
	_, text, _, _ := Describe(doc.HTML, doc.URL)
	if text == "" {
		text = doc.HTML
	}
	if text == "" {
		return nil, "", fmt.Errorf("no content at %s", req.Source.URL)
	}
	return doc, ContentHash(text), nil
}
