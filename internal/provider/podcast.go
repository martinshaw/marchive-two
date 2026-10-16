package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/martinshaw/marchive-two/internal/store"
)

// Podcast archives new enclosures from an RSS or Atom feed.
type Podcast struct{}

func (Podcast) Kind() string { return KindPodcast }

func (Podcast) Run(ctx context.Context, req Request) (Summary, error) {
	client := &http.Client{}
	var body []byte
	_, err := withRetry(ctx, 3, func() error {
		var getErr error
		body, getErr = getBytes(ctx, client, req.Source.URL, 20<<20)
		return getErr
	})
	if err != nil {
		return Summary{}, err
	}
	feed, err := gofeed.NewParser().ParseString(string(body))
	if err != nil {
		return Summary{}, fmt.Errorf("read feed: %w", err)
	}
	if feed == nil {
		return Summary{}, fmt.Errorf("empty feed")
	}
	if strings.TrimSpace(feed.Title) != "" {
		maybeRenameSource(req, feed.Title)
	}

	slots := req.PodcastConcurrency
	if slots < 1 {
		slots = 2
	}
	if slots > 6 {
		slots = 6
	}
	sem := make(chan struct{}, slots)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var saved, skipped, failed, attempted int
	maxNew := req.Schedule.MaxNew

	for _, item := range feed.Items {
		if err := ctx.Err(); err != nil {
			break
		}
		if item == nil {
			continue
		}
		identity, enclosure := episodeIdentity(item)
		if identity == "" {
			continue
		}
		seen, err := req.Store.ItemSeen(req.Source.ID, identity, enclosure)
		if err != nil {
			return Summary{}, err
		}
		if seen {
			mu.Lock()
			skipped++
			mu.Unlock()
			continue
		}
		if maxNew > 0 && attempted >= maxNew {
			break
		}
		attempted++
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = identity
		}
		record := store.Item{
			SourceID:    req.Source.ID,
			RunID:       req.RunID,
			IdentityKey: identity,
			Kind:        "episode",
			Title:       title,
			URL:         enclosure,
			Status:      store.StatusRunning,
		}
		id, err := req.Store.InsertItem(record)
		if errors.Is(err, store.ErrExists) {
			mu.Lock()
			skipped++
			mu.Unlock()
			continue
		}
		if err != nil {
			return Summary{}, err
		}
		record.ID = id
		if enclosure == "" {
			record.Status = store.StatusFailed
			record.Error = "Episode has no media file."
			_ = req.Store.UpdateItem(record)
			mu.Lock()
			failed++
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func(item *gofeed.Item, record store.Item, enclosure string) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					record.Status = store.StatusFailed
					record.Error = fmt.Sprintf("panic: %v", rec)
					_ = req.Store.UpdateItem(record)
					mu.Lock()
					failed++
					mu.Unlock()
				}
			}()
			select {
			case <-ctx.Done():
				record.Status = store.StatusCancelled
				record.Error = "Cancelled."
				_ = req.Store.UpdateItem(record)
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			err := saveEpisode(ctx, client, req, item, feed.Title, &record, enclosure)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if ctx.Err() != nil {
					record.Status = store.StatusCancelled
					record.Error = "Cancelled."
				} else {
					record.Status = store.StatusFailed
					record.Error = err.Error()
					failed++
				}
				_ = req.Store.UpdateItem(record)
				return
			}
			saved++
		}(item, record, enclosure)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Summary{Note: fmt.Sprintf("%d new, %d already archived, %d failed", saved, skipped, failed)}, err
	}
	note := fmt.Sprintf("%d new, %d already archived, %d failed", saved, skipped, failed)
	if maxNew > 0 && attempted >= maxNew {
		note += fmt.Sprintf(". Stopped after %d new episodes.", maxNew)
	}
	return Summary{Note: note, Failed: failed > 0 && saved == 0 && skipped == 0}, nil
}

func saveEpisode(ctx context.Context, client *http.Client, req Request, item *gofeed.Item, feedTitle string, record *store.Item, enclosure string) error {
	dir := filepath.Join(req.Schedule.DownloadLocation, SafeFileName(record.Title, record.IdentityKey))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	meta := episodeMeta(item, feedTitle, enclosure)
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}
	dest := filepath.Join(dir, "episode"+extensionFrom(enclosure, ""))
	attempts, err := withRetry(ctx, 3, func() error {
		return downloadFile(ctx, client, enclosure, dest)
	})
	record.RetryCount = attempts - 1
	if err != nil {
		os.RemoveAll(dir)
		return err
	}
	record.Status = store.StatusSucceeded
	record.DownloadLocation = dir
	record.ContentHash = ShortHash(enclosure)
	return req.Store.UpdateItem(*record)
}

type episodeFile struct {
	Title        string `json:"title"`
	GUID         string `json:"guid,omitempty"`
	Link         string `json:"link,omitempty"`
	EnclosureURL string `json:"enclosureUrl"`
	Published    string `json:"published,omitempty"`
	Author       string `json:"author,omitempty"`
	Description  string `json:"description,omitempty"`
	FeedTitle    string `json:"feedTitle,omitempty"`
	CapturedAt   string `json:"capturedAt"`
}

func episodeMeta(item *gofeed.Item, feedTitle, enclosure string) episodeFile {
	meta := episodeFile{
		Title:        item.Title,
		GUID:         item.GUID,
		Link:         item.Link,
		EnclosureURL: enclosure,
		Description:  item.Description,
		FeedTitle:    feedTitle,
		CapturedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if item.PublishedParsed != nil {
		meta.Published = item.PublishedParsed.UTC().Format(time.RFC3339)
	} else if item.Published != "" {
		meta.Published = item.Published
	}
	if len(item.Authors) > 0 && item.Authors[0] != nil {
		meta.Author = item.Authors[0].Name
	}
	return meta
}

func episodeIdentity(item *gofeed.Item) (identity, enclosure string) {
	if len(item.Enclosures) > 0 && item.Enclosures[0] != nil {
		enclosure = strings.TrimSpace(item.Enclosures[0].URL)
	}
	identity = strings.TrimSpace(item.GUID)
	if identity == "" {
		identity = enclosure
	}
	return identity, enclosure
}
