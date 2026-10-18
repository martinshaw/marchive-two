package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinshaw/marchive-two/internal/browser"
)

func TestOmitCommonWordsKeepsContent(t *testing.T) {
	index := OmitCommonWords(WordIndex{
		Title: "Desk", WordCount: 12, UniqueCount: 3,
		Words: []WordCount{{Word: "the", Count: 9}, {Word: "archives", Count: 2}, {Word: "it's", Count: 1}},
	})
	if index.WordCount != 2 || index.UniqueCount != 1 || len(index.Words) != 1 || index.Words[0].Word != "archives" {
		t.Fatalf("index %+v", index)
	}
	kept := OmitCommonWords(WordIndex{Words: []WordCount{{Word: "The", Count: 4}, {Word: "Guardian", Count: 2}}})
	if countIn(kept, "The") != 0 || countIn(kept, "Guardian") != 2 {
		t.Fatalf("kept %+v", kept)
	}
	dropped := OmitListedWords(kept, URLWords("https://www.theguardian.com/uk"))
	if countIn(dropped, "Guardian") != 0 {
		t.Fatalf("dropped %+v", dropped)
	}
}

func TestURLWordsSplitsSiteHost(t *testing.T) {
	words := URLWords("https://www.theguardian.com/uk/world/story")
	if !words["theguardian"] || !words["guardian"] || words["www"] || words["com"] || words["world"] || words["story"] {
		t.Fatalf("words %v", words)
	}
	bbc := URLWords("https://www.bbc.co.uk/news")
	if !bbc["bbc"] || bbc["co"] || bbc["uk"] || bbc["news"] {
		t.Fatalf("bbc %v", bbc)
	}
	if URLWords("") != nil || URLWords("not a url") != nil {
		t.Fatal("expected no words")
	}
}

func TestMergeWordIndexesSumsCounts(t *testing.T) {
	merged := MergeWordIndexes("All",
		WordIndex{Words: []WordCount{{Word: "Archives", Count: 2}, {Word: "desk", Count: 1}}},
		WordIndex{Words: []WordCount{{Word: "archives", Count: 1}}},
	)
	if merged.Title != "All" || merged.WordCount != 4 || merged.UniqueCount != 3 {
		t.Fatalf("merged %+v", merged)
	}
	if countIn(merged, "Archives") != 2 || countIn(merged, "archives") != 1 || countIn(merged, "desk") != 1 {
		t.Fatalf("order %+v", merged.Words)
	}
}

func TestBuildWordIndexKeepsCase(t *testing.T) {
	index := BuildWordIndex("Guardian", "The Guardian and the guardian")
	if countIn(index, "Guardian") != 2 || countIn(index, "guardian") != 1 || countIn(index, "The") != 1 || countIn(index, "the") != 1 {
		t.Fatalf("index %+v", index)
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
	for _, word := range []string{"Readable", "sentence", "archives", "Desk", "notes"} {
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
	if len(entries) != 1 {
		t.Fatalf("dirs %d", len(entries))
	}
	captures, err := os.ReadDir(filepath.Join(req.Schedule.DownloadLocation, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if len(captures) != 2 {
		t.Fatalf("captures %d", len(captures))
	}
	indexes := 0
	for _, entry := range captures {
		raw, err := os.ReadFile(filepath.Join(req.Schedule.DownloadLocation, entries[0].Name(), entry.Name(), "words.json"))
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
	var raw []byte
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "words.json" || raw != nil {
			return err
		}
		raw, err = os.ReadFile(path)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if raw == nil {
		t.Fatal("words.json not found")
	}
	var index WordIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	return index
}

func countIn(index WordIndex, word string) int {
	for _, item := range index.Words {
		if item.Word == word {
			return item.Count
		}
	}
	return 0
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
	if index.Title != "Desk notes" || !hasWord(index, "Readable") {
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
