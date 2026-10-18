package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
