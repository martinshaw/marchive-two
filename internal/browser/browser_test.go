package browser

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchExample(t *testing.T) {
	if _, err := Find(); err != nil {
		t.Skip(err)
	}
	client := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	doc, err := client.Fetch(ctx, "https://example.com/", Options{Screenshot: true, MHTML: true, Scroll: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.HTML, "Example Domain") {
		t.Fatalf("unexpected page: %.200q", doc.HTML)
	}
	if len(doc.Screenshot) == 0 {
		t.Fatal("missing screenshot")
	}
	if !strings.Contains(doc.MHTML, "Example Domain") {
		t.Fatal("missing mhtml snapshot")
	}
}

func TestFetchWaitsForClientRender(t *testing.T) {
	if _, err := Find(); err != nil {
		t.Skip(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/items" {
			time.Sleep(1500 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `["alpha","beta"]`)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><body><div id="list"></div><script>
fetch("/items").then(function (r) { return r.json(); }).then(function (items) {
  document.getElementById("list").innerHTML = items.map(function (item) {
    return '<a href="/' + item + '">' + item + '</a>';
  }).join("");
});
</script></body></html>`)
	}))
	defer srv.Close()

	client := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	doc, err := client.Fetch(ctx, srv.URL+"/", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.HTML, ">alpha<") || !strings.Contains(doc.HTML, ">beta<") {
		t.Fatalf("snapshot missed client-rendered links: %.400q", doc.HTML)
	}
}

func TestFetchArchivesImagesAndStyles(t *testing.T) {
	if _, err := Find(); err != nil {
		t.Skip(err)
	}
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x03, 0x00, 0x01, 0x00, 0x18, 0xdd, 0x8d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app.css":
			w.Header().Set("Content-Type", "text/css")
			_, _ = io.WriteString(w, `h1{color:#123456}body{background:url("/bg.png")}`)
		case "/photo.png", "/bg.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<!doctype html><html><head><link rel="stylesheet" href="/app.css"><title>Frozen desk</title></head><body><h1>Frozen desk</h1><img src="/photo.png" srcset="/photo.png 1x"></body></html>`)
		}
	}))
	defer srv.Close()

	client := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	doc, err := client.Fetch(ctx, srv.URL+"/", Options{MHTML: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.MHTML, "image/jxl") || strings.Contains(doc.MHTML, "Saved by Blink") {
		t.Fatal("capture fell back to Chrome's JPEG XL snapshot")
	}
	page, err := extractArchiveHTML(doc.MHTML)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "Frozen desk") || !strings.Contains(page, "#123456") {
		t.Fatalf("styles missing: %.500q", page)
	}
	if !strings.Contains(page, "data:image/png;base64,") {
		t.Fatalf("images missing: %.500q", page)
	}
	if strings.Contains(page, "/photo.png") || strings.Contains(page, "/bg.png") || strings.Contains(page, "srcset") {
		t.Fatalf("archive still points at the network: %.500q", page)
	}
}
