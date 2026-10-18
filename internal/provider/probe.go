package provider

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/martinshaw/marchive-two/internal/version"
)

const (
	pagePeekLimit = 64 << 10
	feedPeekLimit = 4 << 20
)

// CheckAddress confirms rawURL can be captured as kind. Title is a page or feed title when one is present.
func CheckAddress(ctx context.Context, kind, rawURL string) (string, error) {
	if !ValidKind(kind) {
		return "", errBadKind
	}
	if kind == KindTrending {
		if !trendingHost(rawURL) {
			return "", errNotX
		}
		return "X trending", nil
	}
	if kind == KindHN {
		return checkHackerNews(ctx, rawURL)
	}
	if kind == KindGoogle {
		return checkGoogle(rawURL)
	}
	if kind == KindDDG {
		return checkDuckDuckGo(rawURL)
	}
	limit := int64(pagePeekLimit)
	if kind == KindPodcast {
		limit = feedPeekLimit
	}
	contentType, body, err := peekAddress(ctx, rawURL, limit)
	if err != nil {
		return "", err
	}
	if kind == KindPodcast {
		feed, ferr := gofeed.NewParser().ParseString(string(body))
		if ferr != nil || !isFeed(feed) {
			return "", errNotFeed
		}
		return strings.TrimSpace(feed.Title), nil
	}
	if feed, ferr := gofeed.NewParser().ParseString(string(body)); ferr == nil && isFeed(feed) {
		return "", errIsFeed
	}
	if !looksLikeHTML(contentType, body) {
		return "", errNotPage
	}
	return htmlTitle(body), nil
}

func isFeed(feed *gofeed.Feed) bool {
	if feed == nil {
		return false
	}
	return feed.FeedType == "rss" || feed.FeedType == "atom"
}

func looksLikeHTML(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "html") {
		return true
	}
	snippet := strings.TrimSpace(strings.ToLower(string(body)))
	if len(snippet) > 800 {
		snippet = snippet[:800]
	}
	return strings.Contains(snippet, "<html") || strings.Contains(snippet, "<!doctype html") || strings.Contains(snippet, "<head") || strings.Contains(snippet, "<body")
}

func htmlTitle(body []byte) string {
	lower := strings.ToLower(string(body))
	start := strings.Index(lower, "<title")
	if start < 0 {
		return ""
	}
	rest := body[start:]
	open := strings.IndexByte(string(rest), '>')
	if open < 0 {
		return ""
	}
	closeAt := strings.Index(strings.ToLower(string(rest[open:])), "</title>")
	if closeAt < 0 {
		return ""
	}
	title := strings.TrimSpace(html.UnescapeString(string(rest[open+1 : open+closeAt])))
	title = strings.Join(strings.Fields(title), " ")
	if len(title) > 140 {
		title = title[:140]
	}
	return title
}

func peekAddress(ctx context.Context, rawURL string, limit int64) (string, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, errCouldNotOpen
	}
	req.Header.Set("User-Agent", "Marchive/"+version.Version)
	req.Header.Set("Accept", "text/html, application/rss+xml, application/atom+xml, application/xml, text/xml, */*")
	resp, err := mediaClient.Do(req)
	if err != nil {
		return "", nil, errCouldNotOpen
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("that address returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return "", nil, errCouldNotOpen
	}
	return resp.Header.Get("Content-Type"), body, nil
}

var (
	errBadKind      = errorString(KindHint())
	errNotX         = errorString("Use an https://x.com or https://twitter.com address.")
	errNotHN        = errorString("Use a Hacker News list, such as https://news.ycombinator.com/newest, or a story address.")
	errNotGoogle    = errorString("Use a Google search address, such as https://www.google.com/search?q=news.")
	errNotDDG       = errorString("Use a DuckDuckGo search address, such as https://duckduckgo.com/?q=news.")
	errNotFeed      = errorString("This address did not return an RSS or Atom feed.")
	errIsFeed       = errorString("This address is a podcast feed. Choose Podcast to capture it.")
	errNotPage      = errorString("This address did not return a web page.")
	errCouldNotOpen = errorString("Could not open that address.")
)
