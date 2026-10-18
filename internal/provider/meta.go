package provider

import (
	"net/url"
	"strings"

	"github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

func parseURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

// SavedMeta is written next to a captured page.
type SavedMeta struct {
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	CanonicalURL string   `json:"canonicalUrl,omitempty"`
	Description  string   `json:"description,omitempty"`
	Authors      []string `json:"authors,omitempty"`
	Handle       string   `json:"handle,omitempty"`
	Hashtags     []string `json:"hashtags,omitempty"`
	Media        []string `json:"media,omitempty"`
	Published    string   `json:"published,omitempty"`
	SiteName     string   `json:"siteName,omitempty"`
	Query        string   `json:"query,omitempty"`
	Rank         int      `json:"rank,omitempty"`
	Provider     string   `json:"provider"`
	Extractor    string   `json:"extractor"`
	ContentHash  string   `json:"contentHash"`
	CapturedAt   string   `json:"capturedAt"`
}

// Describe pulls metadata and readable text out of HTML.
func Describe(pageHTML, pageURL string) (meta SavedMeta, text, readableHTML, extractor string) {
	meta.URL = pageURL
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err == nil {
		fillMeta(doc, &meta)
	}
	if parsed, err := parseURL(pageURL); err == nil {
		article, err := readability.FromReader(strings.NewReader(pageHTML), parsed)
		if err == nil && strings.TrimSpace(article.TextContent) != "" {
			text = strings.TrimSpace(article.TextContent)
			readableHTML = article.Content
			extractor = "readability"
			if meta.Title == "" {
				meta.Title = strings.TrimSpace(article.Title)
			}
			if meta.Description == "" {
				meta.Description = strings.TrimSpace(article.Excerpt)
			}
			if meta.SiteName == "" {
				meta.SiteName = strings.TrimSpace(article.SiteName)
			}
			if len(meta.Authors) == 0 && strings.TrimSpace(article.Byline) != "" {
				meta.Authors = splitAuthors(article.Byline)
			}
			if meta.Published == "" && article.PublishedTime != nil {
				meta.Published = article.PublishedTime.UTC().Format("2006-01-02T15:04:05Z")
			}
		}
	}
	if strings.TrimSpace(text) == "" && doc != nil {
		text = strings.TrimSpace(visibleText(doc))
		extractor = "text"
	}
	if extractor == "" {
		extractor = "text"
	}
	if meta.Title == "" {
		meta.Title = meta.SiteName
	}
	return meta, text, readableHTML, extractor
}

func fillMeta(n *html.Node, meta *SavedMeta) {
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "title":
				if meta.Title == "" {
					meta.Title = strings.TrimSpace(nodeText(node))
				}
			case "meta":
				key := strings.ToLower(firstNonEmpty(attr(node, "property"), attr(node, "name")))
				content := strings.TrimSpace(attr(node, "content"))
				if content == "" {
					break
				}
				switch key {
				case "description", "og:description":
					if meta.Description == "" || key == "og:description" && meta.Description == "" {
						meta.Description = content
					}
				case "og:title":
					if meta.Title == "" {
						meta.Title = content
					}
				case "og:url":
					if meta.CanonicalURL == "" {
						meta.CanonicalURL = content
					}
				case "og:site_name":
					meta.SiteName = content
				case "author", "article:author":
					meta.Authors = append(meta.Authors, splitAuthors(content)...)
				case "article:published_time", "date", "pubdate":
					if meta.Published == "" {
						meta.Published = content
					}
				}
			case "link":
				if strings.EqualFold(attr(node, "rel"), "canonical") && meta.CanonicalURL == "" {
					meta.CanonicalURL = strings.TrimSpace(attr(node, "href"))
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	meta.Authors = uniqueStrings(meta.Authors)
}

func visibleText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "noscript", "svg":
				return
			}
		}
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
			b.WriteByte(' ')
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
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

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func splitAuthors(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		key := strings.ToLower(s)
		if s == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
