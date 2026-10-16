package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinshaw/marchive-two/internal/browser"
)

func TestMergeWordIndexesSumsCounts(t *testing.T) {
	merged := MergeWordIndexes("All",
		WordIndex{Words: []WordCount{{Word: "Archives", Count: 2}, {Word: "desk", Count: 1}}},
		WordIndex{Words: []WordCount{{Word: "archives", Count: 1}}},
	)
	if merged.Title != "All" || merged.WordCount != 4 || merged.UniqueCount != 2 {
		t.Fatalf("merged %+v", merged)
	}
	if merged.Words[0].Word != "archives" || merged.Words[0].Count != 3 {
		t.Fatalf("order %+v", merged.Words)
	}
}

func TestWordIndexSkipsChrome(t *testing.T) {
	page := `<html><head><title>Desk notes</title></head><body>
	  <header class="masthead">Site chrome</header>
	  <nav>Home About</nav>
	  <aside class="sidebar">Related posts</aside>
	  <div class="cookie popup" role="dialog">Accept cookies</div>
	  <footer>Copyright</footer>
	  <article>
	    <h1>Desk notes</h1>
	    <p>Readable sentence about archives.</p>
	  </article>
	</body></html>`
	body := BodyText(page, "https://example.com/desk-notes")
	index := BuildWordIndex("Desk notes", body)
	if index.Title != "Desk notes" {
		t.Fatalf("title %q", index.Title)
	}
	got := map[string]int{}
	for _, word := range index.Words {
		got[word.Word] = word.Count
	}
	for _, word := range []string{"readable", "sentence", "archives", "desk", "notes"} {
		if got[word] == 0 {
			t.Fatalf("missing %s in %#v body %q", word, got, body)
		}
	}
	if got["about"] != 1 {
		t.Fatalf("about count = %d body %q", got["about"], body)
	}
	for _, word := range []string{"home", "related", "posts", "accept", "cookies", "copyright", "chrome"} {
		if got[word] != 0 {
			t.Fatalf("chrome word %s counted in %#v", word, got)
		}
	}
	if index.WordCount == 0 || index.UniqueCount != len(index.Words) {
		t.Fatalf("counts %+v", index)
	}
}

func TestSnapshotWritesWordIndex(t *testing.T) {
	_, req := testReq(t, KindWebpage)
	req.Fetch = staticFetch{
		html:  `<html><head><title>Desk notes</title></head><body><article><p>Readable sentence about archives.</p></article></body></html>`,
		shot:  []byte("img"),
		mhtml: "snap",
	}
	if _, err := (Webpage{}).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	index := readWordIndex(t, req.Schedule.DownloadLocation)
	if index.Title != "Desk notes" {
		t.Fatalf("title %q", index.Title)
	}
	if !hasWord(index, "archives") {
		t.Fatalf("index %+v", index)
	}

	req.Source.Capture = `{"html":true}`
	req.Fetch = staticFetch{html: `<html><head><title>Other</title></head><body><article><p>Second capture keeps html only.</p></article></body></html>`}
	if _, err := (Webpage{}).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(req.Schedule.DownloadLocation)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("dirs %d", len(entries))
	}
	indexes := 0
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(req.Schedule.DownloadLocation, entry.Name(), "words.json"))
		if err != nil {
			continue
		}
		indexes++
		var saved WordIndex
		if err := json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.Title == "Other" {
			t.Fatal("html-only capture wrote a word index")
		}
	}
	if indexes != 1 {
		t.Fatalf("word indexes %d", indexes)
	}
}

func readWordIndex(t *testing.T, dir string) WordIndex {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dirs %d", len(entries))
	}
	raw, err := os.ReadFile(filepath.Join(dir, entries[0].Name(), "words.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index WordIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	return index
}

func hasWord(index WordIndex, word string) bool {
	for _, item := range index.Words {
		if item.Word == word {
			return item.Count > 0
		}
	}
	return false
}

func TestArticleCaptureWritesWordIndex(t *testing.T) {
	_, req := testReq(t, KindArticles)
	page := `<html><head><title>Desk notes</title><link rel="canonical" href="https://example.com/desk"></head><body><article><p>` +
		repeatSentence() + `</p></article></body></html>`
	item, err := saveSnapshot(req, &browser.Document{URL: "https://example.com/desk", HTML: page}, "article", "https://example.com/desk", "https://example.com/desk", Resolve(KindArticles, "").mode())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(item.DownloadLocation, "words.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index WordIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	if index.Title != "Desk notes" || !hasWord(index, "readable") {
		t.Fatalf("index %+v", index)
	}
}

func repeatSentence() string {
	out := ""
	for i := 0; i < 40; i++ {
		out += "Readable sentence about archives. "
	}
	return out
}
