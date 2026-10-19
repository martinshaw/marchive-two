package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinshaw/marchive-two/internal/browser"
)

type shotFetch struct {
	html  string
	shot  []byte
	fail  map[string]bool
	urls  []string
	shots int
}

func (f *shotFetch) Fetch(_ context.Context, raw string, opt browser.Options) (*browser.Document, error) {
	f.urls = append(f.urls, raw)
	if opt.Screenshot {
		f.shots++
	}
	if f.fail[raw] {
		return nil, os.ErrNotExist
	}
	var shot []byte
	if opt.Screenshot {
		shot = f.shot
	}
	return &browser.Document{URL: raw, HTML: f.html, Screenshot: shot}, nil
}

const googleHTML = `<html><body>
<div class="g">
  <a href="https://example.com/story?utm_source=google">
    <h3>Example story</h3>
  </a>
  <div class="VwiC3b">A short snippet about the story.</div>
</div>
<div class="g">
  <a href="/url?q=https://news.example/item&amp;sa=U&amp;ved=1">
    <h3>News item</h3>
  </a>
  <div class="VwiC3b">Another snippet.</div>
</div>
<div class="g">
  <a href="https://www.googleadservices.com/pagead/aclk?q=ad">
    <h3>Paid listing</h3>
  </a>
</div>
<div class="g">
  <a href="https://www.google.com/search?q=related">
    <h3>Related search</h3>
  </a>
</div>
</body></html>`

const ddgHTML = `<html><body>
<div class="result results_links">
  <a class="result__a" href="https://example.com/story?utm_source=ddg">Example story</a>
  <a class="result__snippet" href="https://example.com/story">A short snippet about the story.</a>
</div>
<div class="result">
  <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fnews.example%2Fitem&amp;rut=1">News item</a>
  <a class="result__snippet">Another snippet.</a>
</div>
<div class="result">
  <a class="result__a" href="https://example.com/story">Example story again</a>
  <a class="result__snippet">Duplicate.</a>
</div>
</body></html>`

func TestGoogleHitsSkipAdsAndUnwrapLinks(t *testing.T) {
	hits := parseSearch(KindGoogle, googleHTML, "https://www.google.com/search?q=city+lights")
	if len(hits) != 2 {
		t.Fatalf("hits %d %+v", len(hits), hits)
	}
	if hits[0].Title != "Example story" || hits[0].URL != "https://example.com/story" || hits[0].Snippet != "A short snippet about the story." || hits[0].Rank != 1 {
		t.Fatalf("first %+v", hits[0])
	}
	if hits[1].Title != "News item" || hits[1].URL != "https://news.example/item" || hits[1].Rank != 2 {
		t.Fatalf("second %+v", hits[1])
	}
}

func TestGoogleHitsReadHeadingLayouts(t *testing.T) {
	const page = `<html><body>
<div id="tads"><div class="g"><a href="https://ads.example/buy"><h3>Buy now</h3></a></div></div>
<div class="g">
  <h3><a href="https://example.com/old?utm_source=google">Old title</a></h3>
  <div class="VwiC3b">Old snippet.</div>
</div>
<div class="MjjYud">
  <a href="https://example.com/role">
    <div role="heading" aria-level="3">Role title</div>
  </a>
  <div class="VwiC3b">Role snippet.</div>
</div>
</body></html>`
	hits := parseSearch(KindGoogle, page, "https://www.google.com/search?q=trump")
	if len(hits) != 2 {
		t.Fatalf("hits %d %+v", len(hits), hits)
	}
	if hits[0].Title != "Old title" || hits[0].URL != "https://example.com/old" || hits[0].Snippet != "Old snippet." {
		t.Fatalf("old %+v", hits[0])
	}
	if hits[1].Title != "Role title" || hits[1].URL != "https://example.com/role" || hits[1].Snippet != "Role snippet." {
		t.Fatalf("role %+v", hits[1])
	}
}

func TestGoogleGotoUsesSavedAddressOrCite(t *testing.T) {
	const page = `<html><body>
<div class="g">
  <a href="/goto?url=abc" data-marchive-url="https://en.wikipedia.org/wiki/Donald_Trump?utm_source=google">
    <h3>Donald Trump</h3>
    <cite>https://en.wikipedia.org › wiki › Donald_Trump</cite>
  </a>
  <div class="VwiC3b">A biography.</div>
</div>
<div class="g">
  <a href="/goto?url=def">
    <h3>Reuters topic</h3>
    <cite>https://www.reuters.com › world › donald-trump</cite>
  </a>
  <div class="VwiC3b">News.</div>
</div>
<div class="g">
  <a href="/goto?url=ghi">
    <h3>Cut off</h3>
    <cite>https://example.com › people › donald...</cite>
  </a>
</div>
</body></html>`
	hits := parseSearch(KindGoogle, page, "https://www.google.com/search?q=Trump")
	if len(hits) != 2 {
		t.Fatalf("hits %d %+v", len(hits), hits)
	}
	if hits[0].URL != "https://en.wikipedia.org/wiki/Donald_Trump" || hits[0].Snippet != "A biography." {
		t.Fatalf("saved %+v", hits[0])
	}
	if hits[1].URL != "https://www.reuters.com/world/donald-trump" || hits[1].Title != "Reuters topic" {
		t.Fatalf("cite %+v", hits[1])
	}
}

func TestGoogleBlockedPage(t *testing.T) {
	sorry := &browser.Document{URL: "https://www.google.com/sorry/index?continue=1", HTML: `<html><body>unusual traffic</body></html>`}
	if !googleBlocked(sorry) {
		t.Fatal("sorry page should be blocked")
	}
	results := &browser.Document{URL: "https://www.google.com/search?q=Trump", HTML: googleHTML}
	if googleBlocked(results) {
		t.Fatal("results page should not be blocked")
	}
}

func TestDuckDuckGoHitsDecodeRedirects(t *testing.T) {
	hits := parseSearch(KindDDG, ddgHTML, "https://html.duckduckgo.com/html/?q=city+lights")
	if len(hits) != 2 {
		t.Fatalf("hits %d %+v", len(hits), hits)
	}
	if hits[0].URL != "https://example.com/story" || hits[0].Snippet != "A short snippet about the story." {
		t.Fatalf("first %+v", hits[0])
	}
	if hits[1].URL != "https://news.example/item" || hits[1].Title != "News item" {
		t.Fatalf("second %+v", hits[1])
	}
	if got := ddgFetchURL("https://duckduckgo.com/?q=city+lights&kl=uk-en"); got != "https://html.duckduckgo.com/html/?kl=uk-en&q=city+lights" {
		t.Fatalf("fetch url %s", got)
	}
}

func TestSearchAddressCheck(t *testing.T) {
	title, err := CheckAddress(context.Background(), KindGoogle, "https://www.google.co.uk/search?q=city+lights")
	if err != nil || title != "city lights" {
		t.Fatalf("google %q %v", title, err)
	}
	if _, err := CheckAddress(context.Background(), KindGoogle, "https://www.google.com/"); err == nil {
		t.Fatal("expected google homepage to fail")
	}
	title, err = CheckAddress(context.Background(), KindDDG, "https://duckduckgo.com/?q=city+lights")
	if err != nil || title != "city lights" {
		t.Fatalf("ddg %q %v", title, err)
	}
	if _, err := CheckAddress(context.Background(), KindDDG, "https://example.com/?q=news"); err == nil {
		t.Fatal("expected a non-search address to fail")
	}
}

func TestGoogleRunSavesEachNewResult(t *testing.T) {
	st, req := testReq(t, KindGoogle)
	req.Source.URL = "https://www.google.com/search?q=city+lights"
	req.Source.Name = "www.google.com"
	req.Fetch = staticFetch{html: googleHTML}
	summary, err := GoogleSearch{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Note != "2 new, 0 already archived, 0 failed" {
		t.Fatalf("note %q", summary.Note)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(located) != 2 {
		t.Fatalf("items %d", len(located))
	}
	item, err := st.GetItem(located[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != "result" || item.URL != "https://example.com/story" || item.Title != "Example story" {
		t.Fatalf("item %+v", item)
	}
	raw, err := os.ReadFile(filepath.Join(item.DownloadLocation, "text.txt"))
	if err != nil || !strings.Contains(string(raw), "short snippet") || !strings.Contains(string(raw), "https://example.com/story") {
		t.Fatalf("text %s %v", raw, err)
	}
	meta, err := os.ReadFile(filepath.Join(item.DownloadLocation, "metadata.json"))
	if err != nil || !strings.Contains(string(meta), `"query": "city lights"`) || !strings.Contains(string(meta), `"rank": 1`) || !strings.Contains(string(meta), `"siteName": "Google"`) {
		t.Fatalf("meta %s %v", meta, err)
	}
	words, err := os.ReadFile(filepath.Join(item.DownloadLocation, "words.json"))
	if err != nil || !strings.Contains(string(words), "snippet") {
		t.Fatalf("words %s %v", words, err)
	}
	again, err := GoogleSearch{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if again.Note != "0 new, 2 already archived, 0 failed" {
		t.Fatalf("again %q", again.Note)
	}
}

func TestSearchSavesResultsPageAndLinkedPage(t *testing.T) {
	st, req := testReq(t, KindGoogle)
	req.Source.URL = "https://www.google.com/search?q=city+lights"
	fetch := &shotFetch{html: googleHTML, shot: []byte("jpeg-bytes")}
	req.Fetch = fetch
	if _, err := (GoogleSearch{}).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(fetch.urls) != 3 || fetch.urls[0] != req.Source.URL || fetch.shots != 3 {
		t.Fatalf("fetches %+v shots %d", fetch.urls, fetch.shots)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.GetItem(located[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	results, err := os.ReadFile(filepath.Join(item.DownloadLocation, "results.jpg"))
	if err != nil || string(results) != "jpeg-bytes" {
		t.Fatalf("results page %q %v", results, err)
	}
	linked, err := os.ReadFile(filepath.Join(item.DownloadLocation, "screenshot.jpg"))
	if err != nil || string(linked) != "jpeg-bytes" {
		t.Fatalf("linked page %q %v", linked, err)
	}
	if err := os.Remove(filepath.Join(item.DownloadLocation, "screenshot.jpg")); err != nil {
		t.Fatal(err)
	}
	fetch.shot = []byte("newer")
	before := len(fetch.urls)
	again, err := GoogleSearch{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if again.Note != "0 new, 2 already archived, 0 failed" {
		t.Fatalf("again %q", again.Note)
	}
	if len(fetch.urls)-before != 2 {
		t.Fatalf("backfill fetches %v", fetch.urls[before:])
	}
	linked, err = os.ReadFile(filepath.Join(item.DownloadLocation, "screenshot.jpg"))
	if err != nil || string(linked) != "newer" {
		t.Fatalf("restored %q %v", linked, err)
	}
	results, err = os.ReadFile(filepath.Join(item.DownloadLocation, "results.jpg"))
	if err != nil || string(results) != "jpeg-bytes" {
		t.Fatalf("results page changed %q %v", results, err)
	}
}

func TestSearchKeepsResultWhenLinkedPageHasNoScreenshot(t *testing.T) {
	st, req := testReq(t, KindDDG)
	req.Source.URL = "https://duckduckgo.com/?q=city+lights"
	fetch := &shotFetch{html: ddgHTML, shot: []byte("serp"), fail: map[string]bool{"https://news.example/item": true}}
	req.Fetch = fetch
	summary, err := DuckDuckGo{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Note != "2 new, 0 already archived, 0 failed" {
		t.Fatalf("note %q", summary.Note)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	var missed bool
	for _, loc := range located {
		item, err := st.GetItem(loc.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(item.DownloadLocation, "results.jpg")); err != nil {
			t.Fatalf("results page missing for %s", item.URL)
		}
		_, shotErr := os.Stat(filepath.Join(item.DownloadLocation, "screenshot.jpg"))
		if item.URL == "https://news.example/item" {
			missed = true
			if !os.IsNotExist(shotErr) {
				t.Fatal("failed page still saved a screenshot")
			}
		} else if shotErr != nil {
			t.Fatalf("screenshot %s %v", item.URL, shotErr)
		}
	}
	if !missed {
		t.Fatal("missing failed result")
	}
}

func TestDuckDuckGoRunSavesResults(t *testing.T) {
	st, req := testReq(t, KindDDG)
	req.Source.URL = "https://duckduckgo.com/?q=city+lights"
	req.Fetch = staticFetch{html: ddgHTML}
	summary, err := DuckDuckGo{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Note != "2 new, 0 already archived, 0 failed" {
		t.Fatalf("note %q", summary.Note)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(located) != 2 {
		t.Fatalf("items %d", len(located))
	}
	item, err := st.GetItem(located[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := os.ReadFile(filepath.Join(item.DownloadLocation, "metadata.json"))
	if err != nil || !strings.Contains(string(meta), "DuckDuckGo") || !strings.Contains(string(meta), "news.example/item") {
		t.Fatalf("meta %s %v", meta, err)
	}
	if _, err := os.Stat(filepath.Join(item.DownloadLocation, "words.json")); err != nil {
		t.Fatal(err)
	}
}
