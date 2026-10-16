package provider

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

var skipPath = regexp.MustCompile(`(?i)^/(tag|tags|author|authors|category|categories|login|signin|signup|subscribe|privacy|terms|about|contact|search|account|cart|wp-admin|feed)(/|$)`)
var datePath = regexp.MustCompile(`/\d{4}/\d{2}/`)
var skipExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true,
	".pdf": true, ".zip": true, ".mp3": true, ".mp4": true, ".css": true, ".js": true,
	".xml": true, ".json": true,
}

// ValidateSelector reports whether a CSS selector compiles.
func ValidateSelector(selector string) error {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil
	}
	_, err := cascadia.Compile(selector)
	return err
}

// ExtractLinks returns same-site article candidates from HTML.
func ExtractLinks(pageHTML, base, selector string, limit int) ([]string, error) {
	if limit < 1 {
		limit = 300
	}
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, err
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	var hrefs []string
	if strings.TrimSpace(selector) != "" {
		sel, err := cascadia.Compile(selector)
		if err != nil {
			return nil, err
		}
		for _, node := range cascadia.QueryAll(doc, sel) {
			hrefs = append(hrefs, hrefsUnder(node)...)
		}
	} else {
		hrefs = hrefsUnder(doc)
	}

	seen := map[string]bool{}
	var out []string
	for _, href := range hrefs {
		resolved, ok := resolveLink(baseURL, href, selector == "")
		if !ok || seen[resolved] {
			continue
		}
		seen[resolved] = true
		out = append(out, resolved)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func hrefsUnder(n *html.Node) []string {
	var out []string
	if n.Type == html.ElementNode && n.Data == "a" {
		if href := strings.TrimSpace(attr(n, "href")); href != "" {
			out = append(out, href)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, hrefsUnder(c)...)
	}
	return out
}

func resolveLink(base *url.URL, href string, strict bool) (string, bool) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") || strings.HasPrefix(strings.ToLower(href), "mailto:") || strings.HasPrefix(strings.ToLower(href), "tel:") {
		return "", false
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", false
	}
	if hostKey(resolved.String()) == "" || hostKey(resolved.String()) != hostKey(base.String()) {
		return "", false
	}
	ext := strings.ToLower(path.Ext(resolved.Path))
	if skipExt[ext] {
		return "", false
	}
	if skipPath.MatchString(resolved.Path) {
		return "", false
	}
	normalized, err := NormalizeURL(resolved.String())
	if err != nil {
		return "", false
	}
	if strict && !articleLike(resolved.Path) {
		return "", false
	}
	baseNorm, err := NormalizeURL(base.String())
	if err == nil && normalized == baseNorm {
		return "", false
	}
	return normalized, true
}

func articleLike(p string) bool {
	if datePath.MatchString(p) {
		return true
	}
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return false
	}
	segments := strings.Split(trimmed, "/")
	if len(segments) >= 2 {
		return true
	}
	return false
}
