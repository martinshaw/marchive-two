package service

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/provider"
)

const wordCloudLimit = 80

// WordCloud is a word index for one item, one capture, or every capture.
type WordCloud struct {
	Title       string               `json:"title"`
	RunID       int64                `json:"runId,omitempty"`
	ItemID      int64                `json:"itemId,omitempty"`
	ItemCount   int                  `json:"itemCount,omitempty"`
	WordCount   int                  `json:"wordCount"`
	UniqueCount int                  `json:"uniqueCount"`
	Words       []provider.WordCount `json:"words"`
	Captures    []WordCapture        `json:"captures,omitempty"`
}

// WordCapture is one capture's contribution to the timeline.
type WordCapture struct {
	RunID      int64          `json:"runId"`
	At         string         `json:"at"`
	SourceName string         `json:"sourceName"`
	Provider   string         `json:"provider"`
	WordCount  int            `json:"wordCount"`
	Counts     map[string]int `json:"counts"`
}

// ItemWords reads the word index saved with one capture item.
func (s *Service) ItemWords(id int64) (WordCloud, error) {
	item, err := s.Store.GetItem(id)
	if err != nil {
		return WordCloud{}, err
	}
	index, err := s.loadWords(item.DownloadLocation, item.Title)
	if err != nil {
		return WordCloud{}, err
	}
	return cloudFrom(index, item.RunID, item.ID, 0), nil
}

// RunWords adds up words.json for every item in one capture.
func (s *Service) RunWords(id int64) (WordCloud, error) {
	detail, err := s.Store.GetRunDetail(id)
	if err != nil {
		return WordCloud{}, err
	}
	items, err := s.Store.ListItemsByRunIDs([]int64{id})
	if err != nil {
		return WordCloud{}, err
	}
	var parts []provider.WordIndex
	for _, item := range items {
		index, err := s.loadWords(item.DownloadLocation, item.Title)
		if err != nil || len(index.Words) == 0 {
			continue
		}
		parts = append(parts, index)
	}
	title := detail.SourceName
	if title == "" {
		title = "Capture"
	}
	merged := provider.MergeWordIndexes(title, parts...)
	cloud := cloudFrom(merged, id, 0, len(parts))
	return cloud, nil
}

// WordTimeline adds up every saved word index and keeps each capture's counts over time.
func (s *Service) WordTimeline() (WordCloud, error) {
	items, err := s.Store.ListWordItems()
	if err != nil {
		return WordCloud{}, err
	}
	type group struct {
		at       time.Time
		name     string
		provider string
		parts    []provider.WordIndex
	}
	order := []int64{}
	groups := map[int64]*group{}
	for _, item := range items {
		index, err := s.loadWords(item.DownloadLocation, item.Title)
		if err != nil || len(index.Words) == 0 {
			continue
		}
		current := groups[item.RunID]
		if current == nil {
			current = &group{at: item.RunAt, name: item.SourceName, provider: item.Provider}
			groups[item.RunID] = current
			order = append(order, item.RunID)
		}
		current.parts = append(current.parts, index)
	}
	var all []provider.WordIndex
	captures := make([]WordCapture, 0, len(order))
	mergedByRun := make([]provider.WordIndex, 0, len(order))
	for _, id := range order {
		current := groups[id]
		name := current.name
		if name == "" {
			name = "Capture"
		}
		merged := provider.MergeWordIndexes(name, current.parts...)
		if merged.WordCount == 0 {
			continue
		}
		all = append(all, merged)
		mergedByRun = append(mergedByRun, merged)
		at := ""
		if !current.at.IsZero() {
			at = current.at.UTC().Format(time.RFC3339)
		}
		captures = append(captures, WordCapture{
			RunID:      id,
			At:         at,
			SourceName: name,
			Provider:   current.provider,
			WordCount:  merged.WordCount,
		})
	}
	combined := provider.MergeWordIndexes("All captures", all...)
	tracked := map[string]bool{}
	for _, word := range topWords(combined.Words, wordCloudLimit) {
		tracked[word.Word] = true
	}
	for i := range captures {
		counts := map[string]int{}
		for _, word := range mergedByRun[i].Words {
			if tracked[word.Word] {
				counts[word.Word] = word.Count
			}
		}
		captures[i].Counts = counts
	}
	cloud := cloudFrom(combined, 0, 0, 0)
	cloud.Captures = captures
	return cloud, nil
}

func (s *Service) loadWords(dir, title string) (provider.WordIndex, error) {
	empty := provider.WordIndex{Title: title, Words: []provider.WordCount{}}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return empty, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return empty, nil
	}
	roots, err := s.Store.DownloadRoots()
	if err != nil {
		return empty, err
	}
	if !insideAny(roots, abs) {
		return empty, nil
	}
	index, err := provider.LoadWordIndex(abs)
	if err != nil {
		return empty, err
	}
	if index.Title == "" {
		index.Title = title
	}
	if index.Words == nil {
		index.Words = []provider.WordCount{}
	}
	return index, nil
}

func cloudFrom(index provider.WordIndex, runID, itemID int64, items int) WordCloud {
	words := topWords(index.Words, wordCloudLimit)
	if words == nil {
		words = []provider.WordCount{}
	}
	title := index.Title
	if title == "" {
		title = "Words"
	}
	return WordCloud{
		Title:       title,
		RunID:       runID,
		ItemID:      itemID,
		ItemCount:   items,
		WordCount:   index.WordCount,
		UniqueCount: index.UniqueCount,
		Words:       words,
	}
}

func topWords(words []provider.WordCount, limit int) []provider.WordCount {
	if len(words) <= limit {
		return words
	}
	out := make([]provider.WordCount, limit)
	copy(out, words[:limit])
	return out
}
