package browser

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
)

// Chrome's Page.captureSnapshot stores pictures as JPEG XL and styles as cid:
// links. Those parts do not display when the file is opened. freezePage builds
// an MHTML document from the rendered page and the original files Chrome loaded.

const frozenJS = `(function () {
  var root = document.documentElement.cloneNode(true);
  var live = document.querySelectorAll("img");
  var copy = root.querySelectorAll("img");
  for (var i = 0; i < live.length && i < copy.length; i++) {
    var src = live[i].currentSrc || live[i].getAttribute("src") || "";
    if (src && src.indexOf("data:") !== 0) copy[i].setAttribute("src", src);
    copy[i].removeAttribute("srcset");
    copy[i].removeAttribute("sizes");
  }
  root.querySelectorAll("picture source, video source, audio source").forEach(function (node) { node.remove(); });
  root.querySelectorAll("script, noscript, iframe, link[rel='preload'], link[rel='modulepreload'], link[rel='preconnect'], link[rel='dns-prefetch'], link[rel='prefetch']").forEach(function (node) { node.remove(); });
  var liveLinks = document.querySelectorAll("link[rel~='stylesheet']");
  var copyLinks = root.querySelectorAll("link[rel~='stylesheet']");
  for (var j = 0; j < liveLinks.length && j < copyLinks.length; j++) {
    if (liveLinks[j].href) copyLinks[j].setAttribute("href", liveLinks[j].href);
  }
  var css = [];
  var collect = function (node) {
    if (!node) return;
    var sheets = node.adoptedStyleSheets || [];
    for (var s = 0; s < sheets.length; s++) {
      try {
        css.push(Array.prototype.map.call(sheets[s].cssRules, function (rule) { return rule.cssText; }).join("\n"));
      } catch (err) {}
    }
    var all = node.querySelectorAll ? node.querySelectorAll("*") : [];
    for (var n = 0; n < all.length; n++) {
      if (all[n].shadowRoot) collect(all[n].shadowRoot);
    }
  };
  collect(document);
  return { html: "<!DOCTYPE html>" + root.outerHTML, css: css };
})()`

const (
	archiveResourceLimit = 8 << 20
	archiveTotalLimit    = 48 << 20
)

type archiveAsset struct {
	URL  string
	MIME string
	Body []byte
}

type assetIndex struct {
	list []archiveAsset
	by   map[string]int
}

func freezePage(ctx context.Context, pageURL string) (string, error) {
	var frozen struct {
		HTML string   `json:"html"`
		CSS  []string `json:"css"`
	}
	if err := chromedp.Evaluate(frozenJS, &frozen).Do(ctx); err != nil {
		return "", err
	}
	if strings.TrimSpace(frozen.HTML) == "" {
		return "", fmt.Errorf("chrome returned an empty page")
	}
	tree, err := page.GetResourceTree().Do(ctx)
	if err != nil {
		return "", err
	}
	var assets []archiveAsset
	var total int
	var walk func(*page.FrameResourceTree)
	walk = func(node *page.FrameResourceTree) {
		if node == nil || node.Frame == nil {
			return
		}
		for _, res := range node.Resources {
			if ctx.Err() != nil || res == nil || !wantedResource(res) {
				continue
			}
			if res.ContentSize > archiveResourceLimit {
				continue
			}
			body, err := page.GetResourceContent(node.Frame.ID, res.URL).Do(ctx)
			if err != nil || len(body) == 0 || len(body) > archiveResourceLimit {
				continue
			}
			if total+len(body) > archiveTotalLimit {
				continue
			}
			total += len(body)
			assets = append(assets, archiveAsset{URL: res.URL, MIME: res.MimeType, Body: body})
		}
		for _, child := range node.ChildFrames {
			walk(child)
		}
	}
	walk(tree)
	return assembleArchive(pageURL, frozen.HTML, frozen.CSS, assets)
}

func wantedResource(res *page.FrameResource) bool {
	if res.Failed || res.Canceled {
		return false
	}
	u := strings.ToLower(res.URL)
	if u == "" || strings.HasPrefix(u, "data:") || strings.HasPrefix(u, "blob:") || strings.HasPrefix(u, "about:") {
		return false
	}
	switch res.Type {
	case network.ResourceTypeImage, network.ResourceTypeStylesheet, network.ResourceTypeFont:
		return true
	default:
		mime := strings.ToLower(res.MimeType)
		return strings.Contains(mime, "image/") || strings.Contains(mime, "text/css") || strings.Contains(mime, "font")
	}
}

func assembleArchive(pageURL, dom string, adopted []string, assets []archiveAsset) (string, error) {
	idx := newAssetIndex(assets)
	doc, err := html.Parse(strings.NewReader(dom))
	if err != nil {
		return "", err
	}
	base := documentBase(doc, pageURL)
	rewriteTree(doc, base, idx)
	if head := findNode(doc, "head"); head != nil {
		for _, sheet := range adopted {
			if strings.TrimSpace(sheet) == "" {
				continue
			}
			head.AppendChild(styleNode(prepareCSS(sheet, base, idx, 0, map[string]bool{})))
		}
	}
	var body bytes.Buffer
	if err := html.Render(&body, doc); err != nil {
		return "", err
	}
	return wrapMHTML(pageURL, documentTitle(doc), body.String()), nil
}

func newAssetIndex(assets []archiveAsset) *assetIndex {
	idx := &assetIndex{by: map[string]int{}}
	for _, asset := range assets {
		if asset.URL == "" || len(asset.Body) == 0 {
			continue
		}
		asset.MIME = archiveMIME(asset.MIME, asset.Body)
		idx.list = append(idx.list, asset)
		at := len(idx.list) - 1
		for _, key := range archiveKeys(asset.URL) {
			if _, ok := idx.by[key]; !ok {
				idx.by[key] = at
			}
		}
	}
	return idx
}

func (idx *assetIndex) get(raw, base string) (archiveAsset, bool) {
	if idx == nil || strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.ToLower(raw), "data:") {
		return archiveAsset{}, false
	}
	for _, key := range archiveKeys(raw) {
		if at, ok := idx.by[key]; ok {
			return idx.list[at], true
		}
	}
	abs := resolveArchiveURL(base, raw)
	if abs == raw {
		return archiveAsset{}, false
	}
	for _, key := range archiveKeys(abs) {
		if at, ok := idx.by[key]; ok {
			return idx.list[at], true
		}
	}
	return archiveAsset{}, false
}

func archiveKeys(raw string) []string {
	keys := []string{raw}
	u, err := url.Parse(raw)
	if err != nil {
		return keys
	}
	u.Fragment = ""
	keys = append(keys, u.String())
	if u.RawQuery == "" {
		return keys
	}
	if plain, err := url.QueryUnescape(u.RawQuery); err == nil && plain != u.RawQuery {
		clone := *u
		clone.RawQuery = plain
		keys = append(keys, clone.String())
	}
	return keys
}

func resolveArchiveURL(base, ref string) string {
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

func rewriteTree(n *html.Node, base string, idx *assetIndex) {
	next := n.FirstChild
	for c := next; c != nil; c = next {
		next = c.NextSibling
		if c.Type != html.ElementNode {
			continue
		}
		if c.Data == "script" || c.Data == "noscript" || c.Data == "iframe" || c.Data == "source" || isCSP(c) || isPrefetch(c) {
			n.RemoveChild(c)
			continue
		}
		if c.Data == "link" && stylesheetLink(c) {
			if style := stylesheetNode(c, base, idx); style != nil {
				n.InsertBefore(style, c)
			}
			n.RemoveChild(c)
			continue
		}
		rewriteTree(c, base, idx)
		rewriteElement(c, base, idx)
	}
}

func rewriteElement(n *html.Node, base string, idx *assetIndex) {
	if n.Type != html.ElementNode {
		return
	}
	switch n.Data {
	case "img", "video", "audio", "source", "embed", "track":
		embedAttr(n, "src", base, idx)
		embedAttr(n, "poster", base, idx)
		dropAttr(n, "srcset")
		dropAttr(n, "sizes")
	case "image", "use":
		embedAttr(n, "href", base, idx)
		embedAttr(n, "xlink:href", base, idx)
	case "object":
		embedAttr(n, "data", base, idx)
	}
	if style := attrValue(n, "style"); style != "" {
		setAttr(n, "style", rewriteCSSURLs(style, base, idx))
	}
	if n.Data == "style" {
		if text := nodeText(n); text != "" {
			setText(n, safeCSS(prepareCSS(text, base, idx, 0, map[string]bool{})))
		}
	}
}

func embedAttr(n *html.Node, key, base string, idx *assetIndex) {
	raw := attrValue(n, key)
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "data:") {
		return
	}
	asset, ok := idx.get(raw, base)
	if !ok || isCSS(asset) {
		return
	}
	setAttr(n, key, dataURI(asset.MIME, asset.Body))
}

func stylesheetNode(link *html.Node, base string, idx *assetIndex) *html.Node {
	asset, ok := idx.get(attrValue(link, "href"), base)
	if !ok || !isCSS(asset) {
		return nil
	}
	node := styleNode(prepareCSS(string(asset.Body), asset.URL, idx, 0, map[string]bool{}))
	if media := attrValue(link, "media"); media != "" {
		node.Attr = append(node.Attr, html.Attribute{Key: "media", Val: media})
	}
	return node
}

func safeCSS(css string) string {
	return strings.ReplaceAll(css, "</style", "<\\/style")
}

func styleNode(css string) *html.Node {
	node := &html.Node{Type: html.ElementNode, Data: "style"}
	node.AppendChild(&html.Node{Type: html.TextNode, Data: safeCSS(css)})
	return node
}

var importRE = regexp.MustCompile(`(?i)@import\s+(?:url\(\s*)?(?:"([^"]+)"|'([^']+)'|([^\s"');]+))\s*\)?\s*;`)

func prepareCSS(css, base string, idx *assetIndex, depth int, seen map[string]bool) string {
	if depth > 6 {
		return rewriteCSSURLs(css, base, idx)
	}
	css = importRE.ReplaceAllStringFunc(css, func(match string) string {
		parts := importRE.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		ref := firstNonEmpty(parts[1], parts[2], parts[3])
		abs := resolveArchiveURL(base, ref)
		if seen[abs] {
			return ""
		}
		asset, ok := idx.get(abs, base)
		if !ok || !isCSS(asset) {
			return match
		}
		seen[abs] = true
		return prepareCSS(string(asset.Body), asset.URL, idx, depth+1, seen)
	})
	return rewriteCSSURLs(css, base, idx)
}

func rewriteCSSURLs(css, base string, idx *assetIndex) string {
	var b strings.Builder
	i := 0
	for {
		j := indexURL(css, i)
		if j < 0 {
			b.WriteString(css[i:])
			return b.String()
		}
		inner, end, ok := urlCall(css, j)
		if !ok {
			b.WriteString(css[i:])
			return b.String()
		}
		b.WriteString(css[i:j])
		raw := strings.TrimSpace(inner)
		raw = strings.Trim(raw, `"'`)
		if raw == "" || strings.HasPrefix(strings.ToLower(raw), "data:") {
			b.WriteString(css[j:end])
		} else if asset, ok := idx.get(raw, base); ok && !isCSS(asset) {
			b.WriteString(`url("` + dataURI(asset.MIME, asset.Body) + `")`)
		} else {
			b.WriteString(css[j:end])
		}
		i = end
	}
}

func indexURL(css string, from int) int {
	for i := from; i+4 <= len(css); i++ {
		if strings.EqualFold(css[i:i+4], "url(") {
			return i
		}
	}
	return -1
}

func urlCall(css string, i int) (inner string, end int, ok bool) {
	rest := css[i+4:]
	quote := byte(0)
	for n := 0; n < len(rest); n++ {
		ch := rest[n]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == ')' {
			return rest[:n], i + 4 + n + 1, true
		}
	}
	return "", 0, false
}

func dataURI(mime string, body []byte) string {
	return "data:" + archiveMIME(mime, body) + ";base64," + base64.StdEncoding.EncodeToString(body)
}

func isCSS(asset archiveAsset) bool {
	return strings.Contains(strings.ToLower(asset.MIME), "text/css")
}

func archiveMIME(mime string, body []byte) string {
	mime = strings.TrimSpace(strings.Split(mime, ";")[0])
	if mime != "" && mime != "application/octet-stream" {
		return mime
	}
	switch {
	case bytes.HasPrefix(body, []byte{0x89, 'P', 'N', 'G'}):
		return "image/png"
	case bytes.HasPrefix(body, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(body, []byte("GIF8")):
		return "image/gif"
	case len(body) >= 12 && bytes.HasPrefix(body, []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP")):
		return "image/webp"
	case len(body) > 4 && bytes.Equal(body[:4], []byte{'w', 'O', 'F', '2'}):
		return "font/woff2"
	default:
		if mime == "" {
			return "application/octet-stream"
		}
		return mime
	}
}

func wrapMHTML(pageURL, title, doc string) string {
	boundary := archiveBoundary(doc)
	var htmlPart bytes.Buffer
	qp := quotedprintable.NewWriter(&htmlPart)
	_, _ = qp.Write([]byte(doc))
	_ = qp.Close()

	var out strings.Builder
	fmt.Fprintf(&out, "From: <Saved by Marchive>\r\n")
	fmt.Fprintf(&out, "Snapshot-Content-Location: %s\r\n", oneLine(pageURL))
	fmt.Fprintf(&out, "Subject: %s\r\n", headerValue(title))
	fmt.Fprintf(&out, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123))
	out.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&out, "Content-Type: multipart/related;\r\n\ttype=\"text/html\";\r\n\tboundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&out, "--%s\r\n", boundary)
	out.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	fmt.Fprintf(&out, "Content-Location: %s\r\n", oneLine(pageURL))
	out.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	out.Write(htmlPart.Bytes())
	out.WriteString("\r\n")
	fmt.Fprintf(&out, "--%s--\r\n", boundary)
	return out.String()
}

func archiveBoundary(doc string) string {
	for {
		buf := make([]byte, 12)
		_, _ = rand.Read(buf)
		boundary := "----MarchiveBoundary--" + hex.EncodeToString(buf) + "----"
		if !strings.Contains(doc, boundary) {
			return boundary
		}
	}
}

func headerValue(value string) string {
	value = oneLine(value)
	if value == "" {
		return "Saved page"
	}
	for _, r := range value {
		if r > 127 {
			return mime.QEncoding.Encode("utf-8", value)
		}
	}
	return value
}

func oneLine(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
}

func extractArchiveHTML(mhtml string) (string, error) {
	marker := `boundary="`
	at := strings.Index(mhtml, marker)
	if at < 0 {
		return "", fmt.Errorf("missing boundary")
	}
	rest := mhtml[at+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", fmt.Errorf("missing boundary")
	}
	boundary := "--" + rest[:end]
	parts := strings.Split(mhtml, boundary)
	for _, part := range parts[1:] {
		head, body, ok := strings.Cut(part, "\r\n\r\n")
		if !ok || !strings.Contains(strings.ToLower(head), "text/html") {
			continue
		}
		reader := quotedprintable.NewReader(strings.NewReader(strings.TrimSuffix(body, "\r\n")))
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(reader); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return "", fmt.Errorf("missing html part")
}

func documentBase(doc *html.Node, pageURL string) string {
	if base := findNode(doc, "base"); base != nil {
		if href := attrValue(base, "href"); href != "" {
			return resolveArchiveURL(pageURL, href)
		}
	}
	return pageURL
}

func documentTitle(doc *html.Node) string {
	if title := findNode(doc, "title"); title != nil {
		if text := strings.TrimSpace(nodeText(title)); text != "" {
			return text
		}
	}
	return "Saved page"
}

func findNode(n *html.Node, name string) *html.Node {
	if n.Type == html.ElementNode && n.Data == name {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNode(c, name); found != nil {
			return found
		}
	}
	return nil
}

func stylesheetLink(n *html.Node) bool {
	rel := strings.ToLower(attrValue(n, "rel"))
	for _, part := range strings.Fields(rel) {
		if part == "stylesheet" {
			return true
		}
	}
	return false
}

func isPrefetch(n *html.Node) bool {
	if n.Data != "link" {
		return false
	}
	switch strings.ToLower(attrValue(n, "rel")) {
	case "preload", "modulepreload", "preconnect", "dns-prefetch", "prefetch":
		return true
	default:
		return false
	}
}

func isCSP(n *html.Node) bool {
	return n.Data == "meta" && strings.EqualFold(attrValue(n, "http-equiv"), "content-security-policy")
}

func attrValue(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, value string) {
	for i := range n.Attr {
		if strings.EqualFold(n.Attr[i].Key, key) {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
}

func dropAttr(n *html.Node, key string) {
	kept := n.Attr[:0]
	for _, attr := range n.Attr {
		if !strings.EqualFold(attr.Key, key) {
			kept = append(kept, attr)
		}
	}
	n.Attr = kept
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func setText(n *html.Node, text string) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		c = next
	}
	n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
