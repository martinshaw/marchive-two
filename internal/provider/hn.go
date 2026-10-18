package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/store"
	nethtml "golang.org/x/net/html"
)

const (
	hnCommentLimit = 400
	hnCommentDepth = 10
)

// hnAPI is the Hacker News item API. Tests point it at a local server.
var hnAPI = "https://hacker-news.firebaseio.com/v0"

// HackerNews archives new stories from a Hacker News list, or one story address.
type HackerNews struct{}

func (HackerNews) Kind() string { return KindHN }

func (HackerNews) Run(ctx context.Context, req Request) (Summary, error) {
	capture := Resolve(req.Source.Provider, req.Source.Capture)
	if capture.Screenshot {
		if err := requireFetch(req.Fetch); err != nil {
			return Summary{}, errors.New("a linked site screenshot needs Chrome")
		}
	}
	api, listTitle, onlyID, ok := hnSource(req.Source.URL)
	if !ok {
		return Summary{}, errors.New("use a Hacker News list, such as https://news.ycombinator.com/newest, or a story address")
	}
	ids := []int{onlyID}
	if onlyID == 0 {
		var err error
		ids, err = fetchHNList(ctx, api)
		if err != nil {
			return Summary{}, err
		}
		if listTitle != "" {
			maybeRenameSource(req, listTitle)
		}
	}
	if len(ids) == 0 {
		return Summary{}, errors.New("Hacker News did not list any stories")
	}
	maxNew := req.Schedule.MaxNew
	var saved, skipped, failed, attempted int
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return Summary{}, err
		}
		if id < 1 {
			continue
		}
		itemURL := hnItemPage(id)
		seen, err := req.Store.ItemSeen(req.Source.ID, itemURL)
		if err != nil {
			return Summary{}, err
		}
		if seen {
			skipped++
			continue
		}
		if maxNew > 0 && attempted >= maxNew {
			break
		}
		item, err := fetchHNItem(ctx, id)
		if err != nil {
			if ctx.Err() != nil {
				return Summary{}, ctx.Err()
			}
			failed++
			continue
		}
		if item.ID == 0 || item.Deleted || item.Dead || !hnStory(item.Type) {
			continue
		}
		attempted++
		if err := saveHN(ctx, req, item, capture); err != nil {
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
	if maxNew > 0 && attempted >= maxNew {
		note += fmt.Sprintf(". Stopped after %d new stories.", maxNew)
	}
	return Summary{Note: note, Failed: failed > 0 && saved == 0 && skipped == 0}, nil
}

func checkHackerNews(ctx context.Context, rawURL string) (string, error) {
	_, title, id, ok := hnSource(rawURL)
	if !ok {
		return "", errNotHN
	}
	if id == 0 {
		return title, nil
	}
	item, err := fetchHNItem(ctx, id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(item.Title) == "" {
		return "Hacker News", nil
	}
	return strings.TrimSpace(item.Title), nil
}

func hnSource(raw string) (api, title string, itemID int, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", 0, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	switch host {
	case "news.ycombinator.com":
		path := strings.Trim(u.Path, "/")
		if path == "item" {
			id, err := strconv.Atoi(u.Query().Get("id"))
			if err != nil || id < 1 {
				return "", "", 0, false
			}
			return "", "Hacker News", id, true
		}
		switch path {
		case "", "news":
			return hnAPI + "/topstories.json", "Hacker News front page", 0, true
		case "newest":
			return hnAPI + "/newstories.json", "Hacker News newest", 0, true
		case "best":
			return hnAPI + "/beststories.json", "Hacker News best", 0, true
		case "ask":
			return hnAPI + "/askstories.json", "Hacker News ask", 0, true
		case "show":
			return hnAPI + "/showstories.json", "Hacker News show", 0, true
		case "jobs":
			return hnAPI + "/jobstories.json", "Hacker News jobs", 0, true
		default:
			return "", "", 0, false
		}
	case "hacker-news.firebaseio.com":
		path := strings.Trim(u.Path, "/")
		switch path {
		case "v0/topstories.json":
			return u.String(), "Hacker News front page", 0, true
		case "v0/newstories.json":
			return u.String(), "Hacker News newest", 0, true
		case "v0/beststories.json":
			return u.String(), "Hacker News best", 0, true
		case "v0/askstories.json":
			return u.String(), "Hacker News ask", 0, true
		case "v0/showstories.json":
			return u.String(), "Hacker News show", 0, true
		case "v0/jobstories.json":
			return u.String(), "Hacker News jobs", 0, true
		}
		if id, ok := firebaseItemID(path); ok {
			return "", "Hacker News", id, true
		}
		return "", "", 0, false
	default:
		return "", "", 0, false
	}
}

func firebaseItemID(path string) (int, bool) {
	const prefix = "v0/item/"
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(path, prefix), ".json")
	id, err := strconv.Atoi(rest)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

func hnItemPage(id int) string {
	return "https://news.ycombinator.com/item?id=" + strconv.Itoa(id)
}

func hnStory(kind string) bool {
	switch kind {
	case "story", "job", "poll":
		return true
	default:
		return false
	}
}

type hnItem struct {
	ID          int    `json:"id"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Kids        []int  `json:"kids"`
	Dead        bool   `json:"dead"`
	Deleted     bool   `json:"deleted"`
}

func fetchHNList(ctx context.Context, api string) ([]int, error) {
	var body []byte
	_, err := withRetry(ctx, 3, func() error {
		var getErr error
		body, getErr = getBytes(ctx, mediaClient, api, 1<<20)
		return getErr
	})
	if err != nil {
		return nil, err
	}
	var ids []int
	if err := json.Unmarshal(body, &ids); err != nil {
		return nil, errors.New("Hacker News did not return a story list")
	}
	return ids, nil
}

func fetchHNItem(ctx context.Context, id int) (hnItem, error) {
	var body []byte
	_, err := withRetry(ctx, 3, func() error {
		var getErr error
		body, getErr = getBytes(ctx, mediaClient, fmt.Sprintf("%s/item/%d.json", hnAPI, id), 1<<20)
		return getErr
	})
	if err != nil {
		return hnItem{}, err
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "null" {
		return hnItem{}, nil
	}
	var item hnItem
	if err := json.Unmarshal(body, &item); err != nil {
		return hnItem{}, errors.New("Hacker News returned an unreadable story")
	}
	return item, nil
}

type hnComment struct {
	Author string
	Text   string
	At     time.Time
	Depth  int
}

func loadComments(ctx context.Context, ids []int, depth int, out *[]hnComment) bool {
	truncated := false
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return truncated
		}
		if len(*out) >= hnCommentLimit {
			return true
		}
		item, err := fetchHNItem(ctx, id)
		if err != nil || item.ID == 0 {
			if ctx.Err() != nil {
				return truncated
			}
			continue
		}
		if item.Deleted {
			if len(item.Kids) == 0 {
				continue
			}
			if depth+1 >= hnCommentDepth {
				truncated = true
				continue
			}
			if loadComments(ctx, item.Kids, depth+1, out) {
				truncated = true
			}
			continue
		}
		if item.Dead {
			continue
		}
		text := htmlToPlain(item.Text)
		if text != "" {
			comment := hnComment{Author: item.By, Text: text, Depth: depth}
			if item.Time > 0 {
				comment.At = time.Unix(item.Time, 0).UTC()
			}
			*out = append(*out, comment)
		}
		if len(item.Kids) == 0 {
			continue
		}
		if depth+1 >= hnCommentDepth || len(*out) >= hnCommentLimit {
			truncated = true
			continue
		}
		if loadComments(ctx, item.Kids, depth+1, out) {
			truncated = true
		}
	}
	return truncated
}

func htmlToPlain(fragment string) string {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return ""
	}
	doc, err := nethtml.Parse(strings.NewReader(fragment))
	if err != nil {
		return strings.Join(strings.Fields(html.UnescapeString(fragment)), " ")
	}
	var b strings.Builder
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.TextNode {
			b.WriteString(node.Data)
			return
		}
		if node.Type == nethtml.ElementNode && node.Data == "a" {
			href := strings.TrimSpace(attr(node, "href"))
			label := strings.TrimSpace(nodeText(node))
			if href != "" && href != label {
				if label != "" {
					b.WriteString(label)
					b.WriteString(" (")
					b.WriteString(href)
					b.WriteString(")")
				} else {
					b.WriteString(href)
				}
				return
			}
		}
		block := node.Type == nethtml.ElementNode && (node.Data == "p" || node.Data == "div" || node.Data == "br" || node.Data == "li")
		if block && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if block && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
	}
	walk(doc)
	var lines []string
	blank := false
	for _, line := range strings.Split(b.String(), "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if blank || len(lines) == 0 {
				continue
			}
			blank = true
			lines = append(lines, "")
			continue
		}
		blank = false
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

type hnMeta struct {
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	ItemURL     string `json:"itemUrl"`
	Author      string `json:"author,omitempty"`
	Score       int    `json:"score"`
	Comments    int    `json:"comments"`
	PostedAt    string `json:"postedAt,omitempty"`
	SiteName    string `json:"siteName"`
	Provider    string `json:"provider"`
	Extractor   string `json:"extractor"`
	ContentHash string `json:"contentHash"`
	CapturedAt  string `json:"capturedAt"`
}

func saveHN(ctx context.Context, req Request, item hnItem, capture Capture) error {
	title := strings.TrimSpace(html.UnescapeString(item.Title))
	if title == "" {
		title = hnItemPage(item.ID)
	}
	plain := htmlToPlain(item.Text)
	var comments []hnComment
	truncated := false
	if capture.Comments || capture.Words {
		truncated = loadComments(ctx, item.Kids, 0, &comments)
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	link := pageLink(item.URL)
	var pageText string
	var shot []byte
	var shotErr error
	if capture.Screenshot && link != "" {
		var doc *browser.Document
		_, shotErr = withRetry(ctx, 2, func() error {
			var ferr error
			doc, ferr = req.Fetch.Fetch(ctx, link, browser.Options{Screenshot: true, Scroll: true})
			return ferr
		})
		if shotErr == nil && doc != nil {
			shot = doc.Screenshot
			if capture.Words || capture.Text {
				pageText = BodyText(doc.HTML, firstNonEmpty(doc.URL, link))
			}
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if !capture.Text && !capture.Comments && !capture.Metadata && !capture.Words && (link == "" || shotErr != nil || len(shot) == 0) {
		if shotErr != nil {
			return shotErr
		}
		if link == "" {
			return errors.New("this story has no website to screenshot")
		}
		return errors.New("the linked site did not return a screenshot")
	}

	itemURL := hnItemPage(item.ID)
	var wordBody strings.Builder
	wordBody.WriteString(plain)
	wordBody.WriteByte('\n')
	for _, comment := range comments {
		wordBody.WriteString(comment.Text)
		wordBody.WriteByte('\n')
	}
	wordBody.WriteString(pageText)
	hash := ContentHash(title + "\n" + wordBody.String())
	root, err := sourceCaptureDir(req)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, SafeFileName(title, itemURL))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(dir)
		}
	}()
	if capture.Text {
		if err := os.WriteFile(filepath.Join(dir, "text.txt"), []byte(storyText(title, link, plain)), 0o644); err != nil {
			return err
		}
		if strings.TrimSpace(pageText) != "" {
			if err := os.WriteFile(filepath.Join(dir, "site.txt"), []byte(strings.TrimSpace(pageText)+"\n"), 0o644); err != nil {
				return err
			}
		}
	}
	if capture.Comments {
		if err := os.WriteFile(filepath.Join(dir, "comments.txt"), []byte(commentText(comments, truncated)), 0o644); err != nil {
			return err
		}
	}
	if capture.Screenshot && len(shot) > 0 {
		if err := os.WriteFile(filepath.Join(dir, "screenshot.jpg"), shot, 0o644); err != nil {
			return err
		}
	}
	if capture.Metadata {
		meta := hnMeta{
			Title:       title,
			URL:         link,
			ItemURL:     itemURL,
			Author:      item.By,
			Score:       item.Score,
			Comments:    item.Descendants,
			SiteName:    "Hacker News",
			Provider:    req.Source.Provider,
			Extractor:   "hn-item",
			ContentHash: hash,
			CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		}
		if item.Time > 0 {
			meta.PostedAt = time.Unix(item.Time, 0).UTC().Format(time.RFC3339)
		}
		raw, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	if capture.Words {
		index := BuildWordIndex(title, wordBody.String())
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
		IdentityKey:      itemURL,
		Kind:             "entry",
		Title:            title,
		URL:              itemURL,
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

func storyText(title, link, plain string) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	if link != "" {
		b.WriteByte('\n')
		b.WriteString(link)
		b.WriteByte('\n')
	}
	if plain != "" {
		b.WriteByte('\n')
		b.WriteString(plain)
		b.WriteByte('\n')
	}
	return b.String()
}

func commentText(comments []hnComment, truncated bool) string {
	if len(comments) == 0 {
		if truncated {
			return "Some replies were left out.\n"
		}
		return "No comments.\n"
	}
	var b strings.Builder
	for _, comment := range comments {
		pad := strings.Repeat("  ", comment.Depth)
		when := ""
		if !comment.At.IsZero() {
			when = " · " + comment.At.Format("2006-01-02 15:04 UTC")
		}
		author := comment.Author
		if author == "" {
			author = "unknown"
		}
		fmt.Fprintf(&b, "%s%s%s\n\n", pad, author, when)
		for _, line := range strings.Split(comment.Text, "\n") {
			fmt.Fprintf(&b, "%s%s\n", pad, line)
		}
		b.WriteByte('\n')
	}
	if truncated {
		if len(comments) >= hnCommentLimit {
			fmt.Fprintf(&b, "Stopped after %d comments.\n", hnCommentLimit)
		} else {
			b.WriteString("Some replies were left out.\n")
		}
	}
	return b.String()
}

func pageLink(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}
