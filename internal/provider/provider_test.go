package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/store"
)

func TestNormalizeAndLinks(t *testing.T) {
	got, err := NormalizeURL("HTTPS://Example.com/a/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/a" {
		t.Fatalf("normalized %s", got)
	}
	name := SafeFileName("Hello, World!", "abc")
	if !strings.HasPrefix(name, "Hello-World-") {
		t.Fatalf("filename %s", name)
	}

	html := `<html><body>
	  <a href="/2024/01/hello-world">Story</a>
	  <a href="/about">About</a>
	  <a href="https://other.example/2024/01/nope">Away</a>
	  <a href="/section/story">Section</a>
	</body></html>`
	links, err := ExtractLinks(html, "https://news.example/", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("links %#v", links)
	}
	selected, err := ExtractLinks(`<div class="posts"><a href="/short">Short</a></div><a href="/2024/01/other">Other</a>`, "https://news.example/", ".posts a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0] != "https://news.example/short" {
		t.Fatalf("selected %#v", selected)
	}
}

func TestDescribeFindsCanonical(t *testing.T) {
	html := `<html><head><title>Hello</title><link rel="canonical" href="https://news.example/2024/01/hello"></head><body><p>` + strings.Repeat("Readable sentence. ", 40) + `</p></body></html>`
	meta, text, _, extractor := Describe(html, "https://news.example/2024/01/hello")
	if meta.CanonicalURL != "https://news.example/2024/01/hello" || meta.Title != "Hello" {
		t.Fatalf("meta %+v", meta)
	}
	if strings.TrimSpace(text) == "" || extractor == "" {
		t.Fatalf("text %q extractor %s", text, extractor)
	}
}

func TestWebpageSkipsUnchanged(t *testing.T) {
	st, req := testReq(t, KindWebpage)
	fetch := staticFetch{html: "<html><head><title>Desk</title></head><body><p>Hello archive</p></body></html>", shot: []byte("img"), mhtml: "snap"}
	req.Fetch = fetch
	first, err := Webpage{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Note != "Saved a new snapshot." {
		t.Fatalf("first %s", first.Note)
	}
	second, err := Webpage{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Note != "Unchanged since the last snapshot." {
		t.Fatalf("second %s", second.Note)
	}
	entries, err := os.ReadDir(req.Schedule.DownloadLocation)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dirs %d", len(entries))
	}
	if _, err := os.Stat(filepath.Join(req.Schedule.DownloadLocation, entries[0].Name(), "snapshot.mhtml")); err != nil {
		t.Fatal(err)
	}
	_ = st
}

func TestPodcastDownloadsOnlyNewEpisodes(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".mp3") {
			w.Write([]byte("audio"))
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Show</title>
		<item><title>One</title><guid>ep-1</guid><enclosure url="` + srv.URL + `/one.mp3" type="audio/mpeg"/></item>
		<item><title>Two</title><guid>ep-2</guid><enclosure url="` + srv.URL + `/two.mp3" type="audio/mpeg"/></item>
		</channel></rss>`))
	}))
	defer srv.Close()

	st, req := testReq(t, KindPodcast)
	req.Source.URL = srv.URL
	_ = st.SetSourceName(req.Source.ID, "")
	req.Source.Name = ""
	req.Schedule.MaxNew = 1
	req.PodcastConcurrency = 2
	first, err := Podcast{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.Note, "1 new") {
		t.Fatalf("first note %s", first.Note)
	}
	req.Schedule.MaxNew = 20
	second, err := Podcast{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(second.Note, "1 new") || !strings.Contains(second.Note, "1 already archived") {
		t.Fatalf("second note %s", second.Note)
	}
	src, err := st.GetSource(req.Source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if src.Name != "Show" {
		t.Fatalf("name %s", src.Name)
	}
}

type staticFetch struct {
	html  string
	shot  []byte
	mhtml string
}

func (f staticFetch) Fetch(context.Context, string, browser.Options) (*browser.Document, error) {
	return &browser.Document{URL: "https://example.com/page", HTML: f.html, Screenshot: f.shot, MHTML: f.mhtml}, nil
}

func testReq(t *testing.T, kind string) (*store.Store, Request) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	archive := filepath.Join(dir, "archive")
	sourceID, scheduleID, err := st.CreateSourceWithSchedule(store.Source{
		Name: "example.com", URL: "https://example.com/page", Provider: kind,
	}, store.Schedule{
		IntervalSeconds: 3600, DownloadLocation: archive, Enabled: true, MaxNew: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := st.GetSchedule(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	detail.Source.ID = sourceID
	return st, Request{Source: detail.Source, Schedule: detail.Schedule, RunID: 0, Store: st}
}
