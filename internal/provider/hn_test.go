package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinshaw/marchive-two/internal/browser"
)

func TestHackerNewsSavesCommentsMetadataScreenshotAndWords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/newstories.json":
			_, _ = w.Write([]byte(`[101,102,103]`))
		case "/v0/item/101.json":
			fmt.Fprintf(w, `{"id":101,"by":"ada","type":"story","title":"Widget launch","score":12,"descendants":2,"time":1700000000,"url":"%s/site","kids":[201]}`, srvURL(r))
		case "/v0/item/201.json":
			_, _ = w.Write([]byte(`{"id":201,"by":"grace","type":"comment","time":1700000100,"text":"<p>Brilliant launch notes</p>","kids":[202]}`))
		case "/v0/item/202.json":
			_, _ = w.Write([]byte(`{"id":202,"by":"linus","type":"comment","time":1700000200,"text":"<p>Agreed <a href=\"https://example.com/note\">fully</a></p>"}`))
		case "/v0/item/102.json":
			_, _ = w.Write([]byte(`{"id":102,"by":"ada","type":"story","title":"Ask HN: Widgets","score":3,"descendants":0,"time":1700000300,"text":"<p>How do widgets fail?</p>"}`))
		case "/v0/item/103.json":
			_, _ = w.Write([]byte(`{"id":103,"deleted":true,"type":"story"}`))
		case "/site":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>Site</title></head><body><article><p>A photonovel about widgets.</p></article></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	previous := hnAPI
	hnAPI = srv.URL + "/v0"
	t.Cleanup(func() { hnAPI = previous })

	fetch := &pageFetch{html: `<html><body><article><p>A photonovel about widgets.</p></article></body></html>`, shot: []byte("jpg-bytes")}
	st, req := testReq(t, KindHN)
	req.Source.URL = "https://news.ycombinator.com/newest"
	req.Fetch = fetch
	summary, err := HackerNews{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Note != "2 new, 0 already archived, 0 failed" {
		t.Fatalf("note %q", summary.Note)
	}
	if fetch.calls != 1 || fetch.urls[0] != srv.URL+"/site" {
		t.Fatalf("screenshot calls %v", fetch.urls)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(located) != 2 {
		t.Fatalf("items %d", len(located))
	}
	linkItem, err := st.GetItem(located[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if linkItem.Kind != "entry" || linkItem.Title != "Widget launch" || linkItem.URL != "https://news.ycombinator.com/item?id=101" {
		t.Fatalf("item %+v", linkItem)
	}
	comments, err := os.ReadFile(filepath.Join(linkItem.DownloadLocation, "comments.txt"))
	if err != nil || !strings.Contains(string(comments), "Brilliant launch notes") || !strings.Contains(string(comments), "fully (https://example.com/note)") {
		t.Fatalf("comments %s %v", comments, err)
	}
	if !strings.Contains(string(comments), "\n  linus ·") {
		t.Fatalf("reply indent %s", comments)
	}
	site, err := os.ReadFile(filepath.Join(linkItem.DownloadLocation, "site.txt"))
	if err != nil || !strings.Contains(string(site), "photonovel") {
		t.Fatalf("site %s %v", site, err)
	}
	shot, err := os.ReadFile(filepath.Join(linkItem.DownloadLocation, "screenshot.jpg"))
	if err != nil || string(shot) != "jpg-bytes" {
		t.Fatalf("shot %q %v", shot, err)
	}
	meta, err := os.ReadFile(filepath.Join(linkItem.DownloadLocation, "metadata.json"))
	if err != nil || !strings.Contains(string(meta), `"score": 12`) || !strings.Contains(string(meta), "ada") || !strings.Contains(string(meta), srv.URL+"/site") {
		t.Fatalf("meta %s %v", meta, err)
	}
	words, err := os.ReadFile(filepath.Join(linkItem.DownloadLocation, "words.json"))
	if err != nil || !strings.Contains(string(words), "photonovel") || !strings.Contains(string(words), "Brilliant") {
		t.Fatalf("words %s %v", words, err)
	}
	ask, err := st.GetItem(located[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ask.DownloadLocation, "screenshot.jpg")); !os.IsNotExist(err) {
		t.Fatal("ask story saved a screenshot")
	}
	text, err := os.ReadFile(filepath.Join(ask.DownloadLocation, "text.txt"))
	if err != nil || !strings.Contains(string(text), "How do widgets fail?") {
		t.Fatalf("ask text %s %v", text, err)
	}
	again, err := HackerNews{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if again.Note != "0 new, 2 already archived, 0 failed" {
		t.Fatalf("again %q", again.Note)
	}
	if fetch.calls != 1 {
		t.Fatalf("screenshot repeated %d", fetch.calls)
	}
}

func TestCheckAddressAcceptsHackerNews(t *testing.T) {
	title, err := CheckAddress(context.Background(), KindHN, "https://news.ycombinator.com/newest")
	if err != nil || title != "Hacker News newest" {
		t.Fatalf("newest %q %v", title, err)
	}
	title, err = CheckAddress(context.Background(), KindHN, "https://news.ycombinator.com/news")
	if err != nil || title != "Hacker News front page" {
		t.Fatalf("front %q %v", title, err)
	}
	if _, err := CheckAddress(context.Background(), KindHN, "https://example.com"); err == nil {
		t.Fatal("example.com was accepted")
	}
	if _, err := CheckAddress(context.Background(), KindHN, "https://news.ycombinator.com/item?id=0"); err == nil {
		t.Fatal("item id 0 was accepted")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/item/55.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"id":55,"type":"story","title":"Desk notes"}`))
	}))
	defer srv.Close()
	previous := hnAPI
	hnAPI = srv.URL + "/v0"
	t.Cleanup(func() { hnAPI = previous })
	title, err = CheckAddress(context.Background(), KindHN, "https://news.ycombinator.com/item?id=55")
	if err != nil || title != "Desk notes" {
		t.Fatalf("item %q %v", title, err)
	}
}

func srvURL(r *http.Request) string {
	return "http://" + r.Host
}

type pageFetch struct {
	html  string
	shot  []byte
	urls  []string
	calls int
}

func (f *pageFetch) Fetch(_ context.Context, rawURL string, opt browser.Options) (*browser.Document, error) {
	f.calls++
	f.urls = append(f.urls, rawURL)
	if !opt.Screenshot {
		return nil, fmt.Errorf("screenshot was not requested")
	}
	return &browser.Document{URL: rawURL, HTML: f.html, Screenshot: f.shot}, nil
}
