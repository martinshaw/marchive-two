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

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/store"
	"golang.org/x/net/html"
)

// GoogleSearch archives new organic results from a Google search address.
type GoogleSearch struct{}

func (GoogleSearch) Kind() string { return KindGoogle }

func (GoogleSearch) Run(ctx context.Context, req Request) (Summary, error) {
	return runSearch(ctx, req, KindGoogle)
}

// DuckDuckGo archives new organic results from a DuckDuckGo search address.
type DuckDuckGo struct{}

func (DuckDuckGo) Kind() string { return KindDDG }

func (DuckDuckGo) Run(ctx context.Context, req Request) (Summary, error) {
	return runSearch(ctx, req, KindDDG)
}

type searchHit struct {
	Title   string
	URL     string
	Snippet string
	Rank    int
}

func runSearch(ctx context.Context, req Request, kind string) (Summary, error) {
	if err := requireFetch(req.Fetch); err != nil {
		return Summary{}, err
	}
	if kind == KindGoogle && !googleSearchURL(req.Source.URL) {
		return Summary{}, errNotGoogle
	}
	if kind == KindDDG && !ddgSearchURL(req.Source.URL) {
		return Summary{}, errNotDDG
	}
	page := req.Source.URL
	if kind == KindDDG {
		page = ddgFetchURL(req.Source.URL)
	}
	var doc *browser.Document
	_, err := withRetry(ctx, 2, func() error {
		var fetchErr error
		doc, fetchErr = req.Fetch.Fetch(ctx, page, browser.Options{Scroll: true})
		return fetchErr
	})
	if err != nil {
		return Summary{}, err
	}
	hits := parseSearch(kind, doc.HTML, page)
	if len(hits) == 0 {
		return Summary{}, errors.New("this search did not list any results")
	}
	query := searchQuery(req.Source.URL)
	if query != "" {
		maybeRenameSource(req, query)
	}
	mode := Resolve(req.Source.Provider, req.Source.Capture).mode()
	engine := "Google"
	if kind == KindDDG {
		engine = "DuckDuckGo"
	}
	maxNew := req.Schedule.MaxNew
	var saved, skipped, failed, attempted int
	for _, hit := range hits {
		if err := ctx.Err(); err != nil {
			return Summary{}, err
		}
		seen, err := req.Store.ItemSeen(req.Source.ID, hit.URL)
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
		attempted++
		if err := saveSearchHit(req, hit, query, mode, engine); err != nil {
			if errors.Is(err, store.ErrExists) {
				skipped++
				continue
			}
			failed++
			continue
		}
		saved++
	}
	note := fmt.Sprintf("%d new, %d already archived, %d failed", saved, skipped, failed)
	if maxNew > 0 && attempted >= maxNew {
		note += fmt.Sprintf(". Stopped after %d new items; raise “new items per run” to take more.", maxNew)
	}
	return Summary{Note: note, Failed: failed > 0 && saved == 0 && skipped == 0}, nil
}

func checkGoogle(raw string) (string, error) {
	if !googleSearchURL(raw) {
		return "", errNotGoogle
	}
	return searchQuery(raw), nil
}

func checkDuckDuckGo(raw string) (string, error) {
	if !ddgSearchURL(raw) {
		return "", errNotDDG
	}
	return searchQuery(raw), nil
}

func googleSearchURL(raw string) bool {
	u, ok := searchPage(raw)
	if !ok || !googleHost(u.Hostname()) {
		return false
	}
	return strings.TrimRight(u.Path, "/") == "/search" && strings.TrimSpace(u.Query().Get("q")) != ""
}

func ddgSearchURL(raw string) bool {
	u, ok := searchPage(raw)
	if !ok || !ddgHost(u.Hostname()) {
		return false
	}
	path := strings.TrimRight(u.Path, "/")
	switch path {
	case "", "/html", "/lite":
	default:
		return false
	}
	return strings.TrimSpace(u.Query().Get("q")) != ""
}

func searchPage(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, false
	}
	return u, true
}

func searchQuery(raw string) string {
	u, ok := searchPage(raw)
	if !ok {
		return ""
	}
	return strings.Join(strings.Fields(u.Query().Get("q")), " ")
}

func ddgFetchURL(raw string) string {
	u, ok := searchPage(raw)
	if !ok || !ddgHost(u.Hostname()) {
		return raw
	}
	q := strings.TrimSpace(u.Query().Get("q"))
	if q == "" {
		return raw
	}
	params := url.Values{}
	params.Set("q", q)
	if kl := strings.TrimSpace(u.Query().Get("kl")); kl != "" {
		params.Set("kl", kl)
	}
	next := url.URL{Scheme: "https", Host: "html.duckduckgo.com", Path: "/html/", RawQuery: params.Encode()}
	return next.String()
}

func googleHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	if host == "google.com" {
		return true
	}
	rest, ok := strings.CutPrefix(host, "google.")
	if !ok || rest == "" || len(rest) > 10 || strings.Contains(rest, ".") && len(rest) > 7 {
		return false
	}
	for _, r := range rest {
		if (r < 'a' || r > 'z') && r != '.' {
			return false
		}
	}
	return true
}

func ddgHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	return host == "duckduckgo.com" || strings.HasSuffix(host, ".duckduckgo.com")
}

func parseSearch(kind, pageHTML, pageURL string) []searchHit {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil
	}
	if kind == KindGoogle {
		return googleHits(doc, pageURL)
	}
	return ddgHits(doc, pageURL)
}

func googleHits(doc *html.Node, pageURL string) []searchHit {
	var hits []searchHit
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(hits) >= 50 {
			return
		}
		if n.Type == html.ElementNode && n.Data == "h3" {
			title := strings.Join(strings.Fields(nodeText(n)), " ")
			link := nearestLink(n)
			raw := ""
			if link != nil {
				raw = unwrapSearchURL(attr(link, "href"), pageURL)
			}
			if title != "" && raw != "" && !seen[raw] {
				seen[raw] = true
				hits = append(hits, searchHit{
					Title:   title,
					URL:     raw,
					Snippet: snippetText(resultBlock(n), n),
					Rank:    len(hits) + 1,
				})
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return hits
}

func ddgHits(doc *html.Node, pageURL string) []searchHit {
	var hits []searchHit
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(hits) >= 50 {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" && (classHas(n, "result__a", "result-link")) {
			title := strings.Join(strings.Fields(nodeText(n)), " ")
			raw := unwrapSearchURL(attr(n, "href"), pageURL)
			if title != "" && raw != "" && !seen[raw] {
				seen[raw] = true
				hits = append(hits, searchHit{
					Title:   title,
					URL:     raw,
					Snippet: snippetText(resultBlock(n), n),
					Rank:    len(hits) + 1,
				})
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return hits
}

func nearestLink(n *html.Node) *html.Node {
	for n != nil {
		if n.Type == html.ElementNode && n.Data == "a" && strings.TrimSpace(attr(n, "href")) != "" {
			return n
		}
		n = n.Parent
	}
	return nil
}

func resultBlock(n *html.Node) *html.Node {
	block := n
	for i := 0; n != nil && i < 8; i++ {
		if n.Type == html.ElementNode && classHas(n, "g", "MjjYud", "tF2Cxc", "result", "web-result") {
			return n
		}
		block = n
		n = n.Parent
	}
	return block
}

func snippetText(block, skip *html.Node) string {
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" || n == nil || n == skip {
			return
		}
		if n.Type == html.ElementNode && (classContains(n, "VwiC3b") || classContains(n, "result__snippet") || classContains(n, "result-snippet")) {
			found = strings.Join(strings.Fields(nodeText(n)), " ")
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(block)
	return found
}

func classHas(n *html.Node, names ...string) bool {
	if n == nil {
		return false
	}
	for _, field := range strings.Fields(attr(n, "class")) {
		for _, name := range names {
			if field == name {
				return true
			}
		}
	}
	return false
}

func classContains(n *html.Node, part string) bool {
	for _, field := range strings.Fields(attr(n, "class")) {
		if strings.Contains(field, part) {
			return true
		}
	}
	return false
}

func unwrapSearchURL(raw, pageURL string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "javascript:") || strings.HasPrefix(raw, "#") {
		return ""
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if base, berr := url.Parse(pageURL); berr == nil {
		ref = base.ResolveReference(ref)
	}
	if ref.Scheme != "http" && ref.Scheme != "https" {
		return ""
	}
	if target := trackingTarget(ref); target != "" {
		return cleanResultURL(target)
	}
	return cleanResultURL(ref.String())
}

func trackingTarget(u *url.URL) string {
	host := u.Hostname()
	if googleHost(host) && strings.HasPrefix(u.Path, "/url") {
		for _, key := range []string{"q", "url"} {
			if value := u.Query().Get(key); strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
				return value
			}
		}
	}
	if ddgHost(host) {
		if value := u.Query().Get("uddg"); strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
			return value
		}
	}
	return ""
}

func cleanResultURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	if googleHost(host) || ddgHost(host) || adHost(host) {
		return ""
	}
	if strings.Contains(strings.ToLower(u.Path), "/aclk") {
		return ""
	}
	query := u.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || lower == "fbclid" || lower == "gclid" || lower == "gclsrc" || lower == "dclid" || lower == "mc_eid" || lower == "yclid" || lower == "ved" || lower == "usg" || lower == "sa" {
			query.Del(key)
		}
	}
	u.RawQuery = query.Encode()
	u.Fragment = ""
	return u.String()
}

func adHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	switch host {
	case "googleadservices.com", "doubleclick.net", "googlesyndication.com":
		return true
	default:
		return strings.HasSuffix(host, ".googleadservices.com") || strings.HasSuffix(host, ".doubleclick.net")
	}
}

func saveSearchHit(req Request, hit searchHit, query string, mode saveMode, engine string) error {
	text := hit.Title
	if hit.URL != "" {
		if text != "" {
			text += "\n\n"
		}
		text += hit.URL
	}
	if hit.Snippet != "" {
		text += "\n\n" + hit.Snippet
	}
	hash := ContentHash(hit.URL + "\n" + text)
	root, err := sourceCaptureDir(req)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, SafeFileName(hit.Title, hit.URL))
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
		Title:        hit.Title,
		URL:          hit.URL,
		CanonicalURL: hit.URL,
		Description:  hit.Snippet,
		SiteName:     engine,
		Query:        query,
		Rank:         hit.Rank,
		Provider:     req.Source.Provider,
		Extractor:    "search-result",
		ContentHash:  hash,
		CapturedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if mode.Text {
		if err := os.WriteFile(filepath.Join(dir, "text.txt"), []byte(text), 0o644); err != nil {
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
	if mode.Words {
		index := BuildWordIndex(hit.Title, hit.Snippet)
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
		IdentityKey:      hit.URL,
		Kind:             "result",
		Title:            hit.Title,
		URL:              hit.URL,
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
