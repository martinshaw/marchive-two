package provider

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/store"
)

type saveMode struct {
	HTML       bool
	Text       bool
	Screenshot bool
	MHTML      bool
	Metadata   bool
	Words      bool
}

func saveSnapshot(req Request, doc *browser.Document, kind, identity, itemURL string, mode saveMode) (store.Item, error) {
	meta, text, readableHTML, extractor := Describe(doc.HTML, doc.URL)
	hashSource := text
	if strings.TrimSpace(hashSource) == "" {
		hashSource = doc.HTML
	}
	hash := ContentHash(hashSource)
	if identity == "" {
		identity = hash
	}
	if itemURL == "" {
		itemURL = firstNonEmpty(meta.CanonicalURL, doc.URL)
	}
	meta.Provider = req.Source.Provider
	meta.Extractor = extractor
	meta.ContentHash = hash
	meta.CapturedAt = time.Now().UTC().Format(time.RFC3339)
	if meta.CanonicalURL == "" {
		meta.CanonicalURL = doc.URL
	}
	if meta.Title == "" {
		meta.Title = identity
	}

	dirName := time.Now().UTC().Format("2006-01-02T15-04-05Z") + "-" + ShortHash(identity)
	if kind == "article" || kind == "episode" {
		dirName = SafeFileName(meta.Title, identity)
	}
	dir := filepath.Join(req.Schedule.DownloadLocation, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return store.Item{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(dir)
		}
	}()

	if mode.HTML {
		body := doc.HTML
		name := "page.html"
		if readableHTML != "" && kind != "snapshot" {
			body = readableHTML
			name = "article.html"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return store.Item{}, err
		}
	}
	if mode.Text {
		if err := os.WriteFile(filepath.Join(dir, "text.txt"), []byte(text), 0o644); err != nil {
			return store.Item{}, err
		}
	}
	if mode.Screenshot && len(doc.Screenshot) > 0 {
		if err := os.WriteFile(filepath.Join(dir, "screenshot.jpg"), doc.Screenshot, 0o644); err != nil {
			return store.Item{}, err
		}
	}
	if mode.MHTML && doc.MHTML != "" {
		if err := os.WriteFile(filepath.Join(dir, "snapshot.mhtml"), []byte(doc.MHTML), 0o644); err != nil {
			return store.Item{}, err
		}
	}
	if mode.Metadata {
		raw, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return store.Item{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), append(raw, '\n'), 0o644); err != nil {
			return store.Item{}, err
		}
	}
	if mode.Words && (kind == "snapshot" || kind == "article") {
		index := BuildWordIndex(meta.Title, BodyText(doc.HTML, doc.URL))
		raw, err := json.MarshalIndent(index, "", "  ")
		if err != nil {
			return store.Item{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, "words.json"), append(raw, '\n'), 0o644); err != nil {
			return store.Item{}, err
		}
	}

	item := store.Item{
		SourceID:         req.Source.ID,
		RunID:            req.RunID,
		IdentityKey:      identity,
		Kind:             kind,
		Title:            meta.Title,
		URL:              itemURL,
		Status:           store.StatusSucceeded,
		DownloadLocation: dir,
		ContentHash:      hash,
	}
	id, err := req.Store.InsertItem(item)
	if errors.Is(err, store.ErrExists) {
		return store.Item{}, store.ErrExists
	}
	if err != nil {
		return store.Item{}, err
	}
	item.ID = id
	cleanup = false
	return item, nil
}

func maybeRenameSource(req Request, title string) {
	title = strings.TrimSpace(title)
	if title == "" || req.Store == nil {
		return
	}
	host := hostKey(req.Source.URL)
	name := strings.TrimSpace(req.Source.Name)
	if name != "" && name != host && name != "www."+host {
		return
	}
	_ = req.Store.SetSourceName(req.Source.ID, title)
}
