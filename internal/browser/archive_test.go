package browser

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAssembleArchiveInlinesStylesAndImages(t *testing.T) {
	photo := []byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4}
	bg := []byte{0x89, 'P', 'N', 'G', 5, 6, 7, 8}
	dom := `<!DOCTYPE html><html><head>
<link rel="stylesheet" href="https://cdn.example/app.css">
<link rel="preconnect" href="https://cdn.example">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'">
<title>Frozen desk</title>
</head><body>
<img src="https://cdn.example/photo.png?crop=5%3A4" srcset="https://cdn.example/large.png 2x">
<script>alert(1)</script>
<iframe src="https://ads.example/banner"></iframe>
</body></html>`
	css := `@import "extra.css";
body{color:#123456;background:url("bg.png")}`
	extra := `.extra{font-family:Desk}`
	adopted := `.live{content:"shown"}`
	archived, err := assembleArchive("https://news.example/story", dom, []string{adopted}, []archiveAsset{
		{URL: "https://cdn.example/app.css", MIME: "text/css", Body: []byte(css)},
		{URL: "https://cdn.example/extra.css", MIME: "text/css; charset=utf-8", Body: []byte(extra)},
		{URL: "https://cdn.example/photo.png?crop=5%3A4", MIME: "image/png", Body: photo},
		{URL: "https://cdn.example/bg.png", MIME: "image/png", Body: bg},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(archived, "image/jxl") || strings.Contains(archived, "cid:") {
		t.Fatal("archive still uses a browser-only snapshot")
	}
	page, err := extractArchiveHTML(archived)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "Frozen desk") || !strings.Contains(page, "#123456") || !strings.Contains(page, ".extra{font-family:Desk}") || !strings.Contains(page, `.live{content:"shown"}`) {
		t.Fatalf("styles were not inlined: %s", page)
	}
	if strings.Contains(page, "<script") || strings.Contains(page, "Content-Security-Policy") || strings.Contains(page, "iframe") || strings.Contains(page, "srcset") {
		t.Fatalf("live page pieces remained: %s", page)
	}
	if !strings.Contains(page, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(photo)) {
		t.Fatal("photo was not inlined")
	}
	if !strings.Contains(page, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(bg)) {
		t.Fatal("stylesheet image was not inlined")
	}
	if strings.Contains(page, "https://cdn.example/photo.png") || strings.Contains(page, `url("bg.png")`) {
		t.Fatalf("archive still points at the network: %s", page)
	}
}
