package browser

import "testing"

func TestExternalLocation(t *testing.T) {
	if got := externalLocation("https://www.google.com/goto?url=abc", "https://en.wikipedia.org/wiki/Donald_Trump"); got != "https://en.wikipedia.org/wiki/Donald_Trump" {
		t.Fatalf("direct %s", got)
	}
	if got := externalLocation("https://www.google.com/goto?url=abc", "https://www.google.com/url?q=https://news.example/item&sa=U"); got != "https://news.example/item" {
		t.Fatalf("wrapped %s", got)
	}
	if got := externalLocation("https://www.google.com/goto?url=abc", "https://www.google.com/search?q=related"); got != "" {
		t.Fatalf("search %s", got)
	}
}

func TestMetaRefresh(t *testing.T) {
	body := `<html><head><meta http-equiv="refresh" content="0;url=https://www.bbc.co.uk/news/articles/abc"></head></html>`
	if got := metaRefresh("https://www.google.com/goto?url=abc", body); got != "https://www.bbc.co.uk/news/articles/abc" {
		t.Fatalf("refresh %s", got)
	}
}
