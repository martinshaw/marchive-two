package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/store"
)

const minArticleRunes = 280

// ArticleIndex archives new articles linked from a page.
type ArticleIndex struct{}

func (ArticleIndex) Kind() string { return KindArticles }

func (ArticleIndex) Run(ctx context.Context, req Request) (Summary, error) {
	if err := requireFetch(req.Fetch); err != nil {
		return Summary{}, err
	}
	var index *browser.Document
	_, err := withRetry(ctx, 3, func() error {
		var fetchErr error
		index, fetchErr = req.Fetch.Fetch(ctx, req.Source.URL, browser.Options{Scroll: true})
		return fetchErr
	})
	if err != nil {
		return Summary{}, err
	}
	links, err := ExtractLinks(index.HTML, firstNonEmpty(index.URL, req.Source.URL), req.Source.LinkSelector, 300)
	if err != nil {
		return Summary{}, err
	}
	maxNew := req.Schedule.MaxNew
	var saved, skipped, failed, attempted int
	for _, link := range links {
		if err := ctx.Err(); err != nil {
			return Summary{}, err
		}
		if maxNew > 0 && attempted >= maxNew {
			break
		}
		seen, err := req.Store.ItemSeen(req.Source.ID, link)
		if err != nil {
			return Summary{}, err
		}
		if seen {
			skipped++
			continue
		}
		attempted++
		if err := captureArticle(ctx, req, link); err != nil {
			if errors.Is(err, errNotArticle) || errors.Is(err, store.ErrExists) {
				if errors.Is(err, errNotArticle) {
					skipped++
				} else {
					skipped++
				}
				continue
			}
			failed++
		} else {
			saved++
		}
	}
	note := fmt.Sprintf("%d new, %d already archived, %d failed", saved, skipped, failed)
	if maxNew > 0 && attempted >= maxNew {
		note += fmt.Sprintf(". Stopped after %d new items; raise “new items per run” to take more.", maxNew)
	}
	return Summary{Note: note, Failed: failed > 0 && saved == 0 && skipped == 0}, nil
}

var errNotArticle = errors.New("not an article")

func captureArticle(ctx context.Context, req Request, link string) error {
	var doc *browser.Document
	attempts, err := withRetry(ctx, 3, func() error {
		var fetchErr error
		doc, fetchErr = req.Fetch.Fetch(ctx, link, browser.Options{})
		return fetchErr
	})
	if err != nil {
		var httpErr statusError
		if errors.As(err, &httpErr) && httpErr.code >= 400 && httpErr.code < 500 {
			_, _ = req.Store.InsertItem(store.Item{
				SourceID:    req.Source.ID,
				RunID:       req.RunID,
				IdentityKey: link,
				Kind:        "article",
				URL:         link,
				Status:      store.StatusFailed,
				Error:       err.Error(),
				RetryCount:  attempts - 1,
			})
		}
		return err
	}
	meta, text, _, _ := Describe(doc.HTML, doc.URL)
	identity := link
	if canon, nerr := NormalizeURL(meta.CanonicalURL); nerr == nil && canon != "" {
		identity = canon
	}
	seen, err := req.Store.ItemSeen(req.Source.ID, identity, link)
	if err != nil {
		return err
	}
	if seen {
		return store.ErrExists
	}
	threshold := minArticleRunes
	if strings.TrimSpace(req.Source.LinkSelector) != "" {
		threshold = 40
	}
	if len([]rune(strings.TrimSpace(text))) < threshold {
		_, insErr := req.Store.InsertItem(store.Item{
			SourceID:    req.Source.ID,
			RunID:       req.RunID,
			IdentityKey: identity,
			Kind:        "article",
			Title:       meta.Title,
			URL:         link,
			Status:      store.StatusSkipped,
			Error:       "Not enough article text to archive.",
		})
		if insErr != nil && !errors.Is(insErr, store.ErrExists) {
			return insErr
		}
		return errNotArticle
	}
	_, err = saveSnapshot(req, doc, "article", identity, link, saveMode{HTML: true, Text: true})
	return err
}
