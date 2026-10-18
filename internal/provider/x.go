package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/store"
	"golang.org/x/net/html"
)

// XTrending archives posts shown on an X or Twitter page, one post at a time.
type XTrending struct{}

func (XTrending) Kind() string { return KindTrending }

func (XTrending) Run(ctx context.Context, req Request) (Summary, error) {
	if err := requireFetch(req.Fetch); err != nil {
		return Summary{}, err
	}
	capture := Resolve(req.Source.Provider, req.Source.Capture)
	var doc *browser.Document
	// A visible window is required. X answers HTTP 403 to the headless one.
	_, err := withRetry(ctx, 3, func() error {
		var fetchErr error
		doc, fetchErr = req.Fetch.Fetch(ctx, req.Source.URL, browser.Options{Scroll: true, Headed: true})
		return fetchErr
	})
	if err != nil {
		return Summary{}, err
	}
	pageURL := firstNonEmpty(doc.URL, req.Source.URL)
	posts := extractPosts(doc.HTML, pageURL)
	opened := 0
	if len(posts) == 0 {
		topics := topicPages(doc.HTML, pageURL)
		if len(topics) > topicPageLimit {
			topics = topics[:topicPageLimit]
		}
		for _, topic := range topics {
			if err := ctx.Err(); err != nil {
				return Summary{}, err
			}
			topicDoc, ferr := req.Fetch.Fetch(ctx, topic, browser.Options{Scroll: true, Headed: true})
			if ferr != nil {
				continue
			}
			opened++
			posts = append(posts, extractPosts(topicDoc.HTML, firstNonEmpty(topicDoc.URL, topic))...)
		}
		posts = uniquePosts(posts)
	}
	if len(posts) == 0 {
		if opened > 0 {
			return Summary{}, errors.New("X listed topics but did not show any posts. Sign in with Chrome on the Marchive profile, then run this source again.")
		}
		return Summary{}, errors.New("X did not show any posts. Sign in with Chrome on the Marchive profile, then run this source again.")
	}
	maxNew := req.Schedule.MaxNew
	var saved, skipped, failed, attempted int
	for _, post := range posts {
		if err := ctx.Err(); err != nil {
			return Summary{}, err
		}
		if maxNew > 0 && attempted >= maxNew {
			break
		}
		seen, err := req.Store.ItemSeen(req.Source.ID, post.URL)
		if err != nil {
			return Summary{}, err
		}
		if seen {
			skipped++
			continue
		}
		attempted++
		if capture.Media && post.NeedsRemote {
			post.Media = mergeXMedia(post.Media, tweetMedia(ctx, post.URL))
		}
		if err := savePost(ctx, req, post, capture.mode()); err != nil {
			if errors.Is(err, store.ErrExists) {
				skipped++
				continue
			}
			if ctx.Err() != nil {
				return Summary{}, ctx.Err()
			}
			failed++
			continue
		}
		saved++
	}
	note := fmt.Sprintf("%d new, %d already archived, %d failed", saved, skipped, failed)
	if opened > 0 {
		note = fmt.Sprintf("Opened %d topic pages. %s", opened, note)
	}
	if maxNew > 0 && attempted >= maxNew {
		note += fmt.Sprintf(". Stopped after %d new items; raise “new items per run” to take more.", maxNew)
	}
	return Summary{Note: note, Failed: failed > 0 && saved == 0 && skipped == 0}, nil
}

// topicPageLimit is how many topic pages to open when the chosen page lists topics instead of posts.
const topicPageLimit = 4

type xPost struct {
	Author      string
	Handle      string
	Text        string
	URL         string
	Hashtags    []string
	Media       []xMedia
	NeedsRemote bool
}

func extractPosts(pageHTML, pageURL string) []xPost {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil
	}
	var posts []xPost
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && attr(node, "data-testid") == "tweet" {
			if post, ok := readPost(node, pageURL); ok && !seen[post.URL] {
				seen[post.URL] = true
				posts = append(posts, post)
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return posts
}

func readPost(node *html.Node, pageURL string) (xPost, bool) {
	post := xPost{Text: strings.TrimSpace(textOfTestID(node, "tweetText"))}
	if post.Text == "" {
		return xPost{}, false
	}
	name := strings.TrimSpace(textOfTestID(node, "User-Name"))
	post.Author, post.Handle = splitXName(name)
	post.URL = firstStatusURL(node, pageURL)
	if post.URL == "" {
		return xPost{}, false
	}
	post.Hashtags = hashtags(node, post.Text)
	post.Media = mediaFrom(node)
	post.NeedsRemote = remoteMedia(node, post.Media)
	return post, true
}

func textOfTestID(root *html.Node, id string) string {
	var found string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != "" {
			return
		}
		if node.Type == html.ElementNode && attr(node, "data-testid") == id {
			found = strings.TrimSpace(nodeText(node))
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func splitXName(raw string) (author, handle string) {
	lines := strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' })
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "·" {
			continue
		}
		if strings.HasPrefix(line, "@") && handle == "" {
			handle = strings.Fields(line)[0]
			continue
		}
		if author == "" {
			author = line
		}
	}
	return author, handle
}

func firstStatusURL(root *html.Node, pageURL string) string {
	base, _ := url.Parse(pageURL)
	var found string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "a" {
			if status := canonicalStatus(attr(node, "href"), base); status != "" {
				found = status
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func canonicalStatus(href string, base *url.URL) string {
	if strings.TrimSpace(href) == "" {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if base != nil {
		ref = base.ResolveReference(ref)
	}
	parts := strings.Split(strings.Trim(ref.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] != "status" || !digits(parts[i+1]) {
			continue
		}
		user := "i"
		if i > 0 && parts[i-1] != "" {
			user = parts[i-1]
		}
		return "https://x.com/" + user + "/status/" + parts[i+1]
	}
	return ""
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func hashtags(root *html.Node, text string) []string {
	seen := map[string]bool{}
	var tags []string
	add := func(tag string) {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" {
			return
		}
		key := strings.ToLower(tag)
		if seen[key] {
			return
		}
		seen[key] = true
		tags = append(tags, tag)
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			if tag := hashtagFromHref(attr(node, "href")); tag != "" {
				add(tag)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	for _, field := range strings.Fields(text) {
		if strings.HasPrefix(field, "#") {
			add(strings.Trim(field, ".,!?:;"))
		}
	}
	return tags
}

func hashtagFromHref(href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(ref.Path, "/"), "/")
	if len(parts) >= 2 && strings.EqualFold(parts[0], "hashtag") {
		tag, err := url.PathUnescape(parts[1])
		if err != nil {
			return parts[1]
		}
		return tag
	}
	return ""
}

func uniquePosts(posts []xPost) []xPost {
	seen := map[string]bool{}
	out := make([]xPost, 0, len(posts))
	for _, post := range posts {
		if post.URL == "" || seen[post.URL] {
			continue
		}
		seen[post.URL] = true
		out = append(out, post)
	}
	return out
}

func topicPages(pageHTML, pageURL string) []string {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil
	}
	base, _ := url.Parse(pageURL)
	seen := map[string]bool{}
	var topics []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			if topic := topicURL(attr(node, "href"), base); topic != "" && !seen[topicKey(topic)] && topic != pageURL {
				seen[topicKey(topic)] = true
				topics = append(topics, topic)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return topics
}

func topicURL(href string, base *url.URL) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(strings.ToLower(href), "javascript:") || strings.HasPrefix(strings.ToLower(href), "mailto:") {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if base != nil {
		ref = base.ResolveReference(ref)
	}
	if canonicalStatus(ref.String(), nil) != "" || !trendingHost(ref.String()) {
		return ""
	}
	path := strings.Trim(ref.Path, "/")
	if !topicPath(path, ref.Query()) {
		return ""
	}
	ref.Scheme = "https"
	ref.Host = "x.com"
	ref.Fragment = ""
	ref.RawPath = ""
	return ref.String()
}

func topicPath(path string, query url.Values) bool {
	path = strings.ToLower(path)
	switch {
	case path == "search":
		return strings.TrimSpace(query.Get("q")) != ""
	case strings.HasPrefix(path, "hashtag/"):
		return strings.Trim(path, "/") != "hashtag"
	case strings.HasPrefix(path, "i/events/"):
		return true
	case strings.HasPrefix(path, "i/topics/"):
		return true
	case strings.HasPrefix(path, "i/trending/"):
		return true
	default:
		return false
	}
}

func topicKey(raw string) string {
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if strings.EqualFold(strings.Trim(ref.Path, "/"), "search") {
		return "search\n" + ref.Query().Get("q")
	}
	return strings.ToLower(strings.Trim(ref.Path, "/"))
}

func trendingHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	switch strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") {
	case "x.com", "twitter.com", "mobile.x.com", "mobile.twitter.com":
		return true
	default:
		return false
	}
}

func savePost(ctx context.Context, req Request, post xPost, mode saveMode) error {
	title := postTitle(post)
	hash := ContentHash(post.URL + "\n" + post.Text)
	root, err := sourceCaptureDir(req)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, SafeFileName(title, post.URL))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(dir)
		}
	}()
	meta := SavedMeta{
		Title:        title,
		URL:          post.URL,
		CanonicalURL: post.URL,
		Description:  post.Text,
		Authors:      nonEmpty(post.Author),
		Handle:       post.Handle,
		Hashtags:     post.Hashtags,
		Media:        mediaURLs(post.Media),
		SiteName:     "X",
		Provider:     req.Source.Provider,
		Extractor:    "x-post",
		ContentHash:  hash,
		CapturedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if mode.Text {
		if err := os.WriteFile(filepath.Join(dir, "text.txt"), []byte(post.Text), 0o644); err != nil {
			return err
		}
	}
	if mode.Metadata {
		raw, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	if mode.Media && len(post.Media) > 0 {
		if err := saveTweetMedia(ctx, dir, post.Media); err != nil {
			return err
		}
	}
	if mode.Words {
		index := BuildWordIndex(title, post.Text)
		raw, err := json.MarshalIndent(index, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "words.json"), append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	_, err = req.Store.InsertItem(store.Item{
		SourceID:         req.Source.ID,
		RunID:            req.RunID,
		IdentityKey:      post.URL,
		Kind:             "tweet",
		Title:            title,
		URL:              post.URL,
		Status:           store.StatusSucceeded,
		DownloadLocation: dir,
		ContentHash:      hash,
	})
	if err != nil {
		return err
	}
	cleanup = false
	return nil
}

func postTitle(post xPost) string {
	text := strings.Join(strings.Fields(post.Text), " ")
	runes := []rune(text)
	if len(runes) > 80 {
		text = string(runes[:80]) + "…"
	}
	if post.Author != "" {
		return post.Author + ": " + text
	}
	return text
}

func nonEmpty(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}
