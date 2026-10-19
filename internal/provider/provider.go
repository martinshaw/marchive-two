// Package provider captures webpages, article indexes, podcasts, text, X posts, Hacker News, and search results.
package provider

import (
	"context"
	"errors"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/store"
)

const (
	KindWebpage  = "webpage"
	KindArticles = "article_index"
	KindPodcast  = "podcast"
	KindText     = "text_extract"
	KindTrending = "x_trending"
	KindHN       = "hn_entry"
	KindGoogle   = "google_search"
	KindDDG      = "ddg_search"
	KindYouTube  = "youtube_video"
)

// KindHint is the message shown when a source kind is missing or unknown.
func KindHint() string {
	return "choose a webpage, article index, podcast, text extract, X trending, Hacker News entry, Google search, DuckDuckGo search, or YouTube video"
}

// Summary is the outcome of one capture, even when individual items fail.
type Summary struct {
	Note   string
	Failed bool
}

// Request is everything a provider needs for one run.
type Request struct {
	Source             store.Source
	Schedule           store.Schedule
	RunID              int64
	Store              *store.Store
	Fetch              browser.Fetcher
	PodcastConcurrency int
	// DataDir is the app data directory. YouTube captures keep yt-dlp there.
	DataDir string
}

// Provider captures one kind of source.
type Provider interface {
	Kind() string
	Run(ctx context.Context, req Request) (Summary, error)
}

// Registry maps provider ids to implementations.
type Registry struct {
	all map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{all: map[string]Provider{}}
}

func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(Webpage{})
	r.Register(ArticleIndex{})
	r.Register(Podcast{})
	r.Register(Text{})
	r.Register(XTrending{})
	r.Register(HackerNews{})
	r.Register(GoogleSearch{})
	r.Register(DuckDuckGo{})
	r.Register(YouTube{})
	return r
}

func (r *Registry) Register(p Provider) {
	if r.all == nil {
		r.all = map[string]Provider{}
	}
	r.all[p.Kind()] = p
}

func (r *Registry) Get(kind string) (Provider, bool) {
	p, ok := r.all[kind]
	return p, ok
}

func ValidKind(kind string) bool {
	switch kind {
	case KindWebpage, KindArticles, KindPodcast, KindText, KindTrending, KindHN, KindGoogle, KindDDG, KindYouTube:
		return true
	default:
		return false
	}
}

func Label(kind string) string {
	switch kind {
	case KindWebpage:
		return "Webpage"
	case KindArticles:
		return "Article index"
	case KindPodcast:
		return "Podcast"
	case KindText:
		return "Text extract"
	case KindTrending:
		return "X trending"
	case KindHN:
		return "Hacker News entry"
	case KindGoogle:
		return "Google search"
	case KindDDG:
		return "DuckDuckGo"
	case KindYouTube:
		return "YouTube video"
	default:
		return kind
	}
}

func requireFetch(fetch browser.Fetcher) error {
	if fetch == nil {
		return errors.New("this capture needs Chrome")
	}
	return nil
}
