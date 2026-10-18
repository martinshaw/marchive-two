package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckAddressMatchesKind(t *testing.T) {
	const page = `<!doctype html><html><head><title>Desk &amp; Co</title></head><body><p>Hello</p></body></html>`
	const feed = `<?xml version="1.0"?><rss version="2.0"><channel><title>Desk Feed</title><item><title>One</title></item></channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(page))
		case "/feed":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(feed))
		case "/data":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	title, err := CheckAddress(context.Background(), KindWebpage, srv.URL+"/page")
	if err != nil || title != "Desk & Co" {
		t.Fatalf("webpage title %q err %v", title, err)
	}
	if _, err := CheckAddress(context.Background(), KindArticles, srv.URL+"/page"); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckAddress(context.Background(), KindText, srv.URL+"/page"); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckAddress(context.Background(), KindPodcast, srv.URL+"/page"); err == nil || !strings.Contains(err.Error(), "RSS or Atom") {
		t.Fatalf("podcast on html: %v", err)
	}

	title, err = CheckAddress(context.Background(), KindPodcast, srv.URL+"/feed")
	if err != nil || title != "Desk Feed" {
		t.Fatalf("feed title %q err %v", title, err)
	}
	if _, err := CheckAddress(context.Background(), KindWebpage, srv.URL+"/feed"); err == nil || !strings.Contains(err.Error(), "Podcast") {
		t.Fatalf("webpage on feed: %v", err)
	}
	if _, err := CheckAddress(context.Background(), KindWebpage, srv.URL+"/data"); err == nil {
		t.Fatal("json page was accepted")
	}
	if _, err := CheckAddress(context.Background(), KindPodcast, srv.URL+"/missing"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("missing: %v", err)
	}
}
