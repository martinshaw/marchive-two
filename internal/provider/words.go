package provider

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

// WordCount is one body word and how often it appears.
type WordCount struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

// WordIndex is the title plus the words of the page body, saved as words.json.
type WordIndex struct {
	Title       string      `json:"title"`
	WordCount   int         `json:"wordCount"`
	UniqueCount int         `json:"uniqueCount"`
	Words       []WordCount `json:"words"`
}

// LoadWordIndex reads words.json from a capture directory. A missing file is an empty index.
func LoadWordIndex(dir string) (WordIndex, error) {
	empty := WordIndex{Words: []WordCount{}}
	path := filepath.Join(filepath.Clean(strings.TrimSpace(dir)), "words.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	if len(raw) > 2<<20 {
		return empty, errors.New("word index is too large")
	}
	var index WordIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return empty, err
	}
	if index.Words == nil {
		index.Words = []WordCount{}
	}
	return index, nil
}

// MergeWordIndexes adds word counts from each index. The result is sorted by count, then word.
func MergeWordIndexes(title string, parts ...WordIndex) WordIndex {
	counts := map[string]int{}
	for _, part := range parts {
		for _, item := range part.Words {
			word := strings.Trim(strings.ToLower(strings.TrimSpace(item.Word)), "'’-")
			if word == "" || item.Count < 1 || !hasLetterOrNumber(word) {
				continue
			}
			counts[word] += item.Count
		}
	}
	words := make([]WordCount, 0, len(counts))
	total := 0
	for word, count := range counts {
		words = append(words, WordCount{Word: word, Count: count})
		total += count
	}
	sortWordCounts(words)
	return WordIndex{
		Title:       strings.TrimSpace(title),
		WordCount:   total,
		UniqueCount: len(words),
		Words:       words,
	}
}

// BuildWordIndex counts title words and body words. Body text should already
// exclude navigation, sidebars, popups, and other site chrome.
func BuildWordIndex(title, body string) WordIndex {
	counts := map[string]int{}
	add := func(s string) {
		for _, word := range tokenize(s) {
			counts[word]++
		}
	}
	add(title)
	add(body)
	words := make([]WordCount, 0, len(counts))
	total := 0
	for word, count := range counts {
		words = append(words, WordCount{Word: word, Count: count})
		total += count
	}
	sortWordCounts(words)
	return WordIndex{
		Title:       strings.TrimSpace(title),
		WordCount:   total,
		UniqueCount: len(words),
		Words:       words,
	}
}

// BodyText returns the readable page body. Site chrome is removed before
// readability scores the page, and the fallback walks the article or main
// element rather than the whole document.
func BodyText(pageHTML, pageURL string) string {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return ""
	}
	stripChrome(doc)
	var buf strings.Builder
	if err := html.Render(&buf, doc); err == nil {
		if parsed, err := parseURL(pageURL); err == nil {
			article, err := readability.FromReader(strings.NewReader(buf.String()), parsed)
			if err == nil {
				if text := strings.TrimSpace(article.TextContent); text != "" {
					return text
				}
			}
		}
	}
	return strings.TrimSpace(visibleText(contentRoot(doc)))
}

func sortWordCounts(words []WordCount) {
	sort.Slice(words, func(i, j int) bool {
		if words[i].Count != words[j].Count {
			return words[i].Count > words[j].Count
		}
		return words[i].Word < words[j].Word
	})
}

func tokenize(s string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) == 0 {
			return
		}
		word := strings.Trim(strings.ToLower(string(cur)), "'’-")
		cur = cur[:0]
		if word == "" || !hasLetterOrNumber(word) {
			return
		}
		words = append(words, word)
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '\'' || r == '’' || r == '-' {
			cur = append(cur, r)
			continue
		}
		flush()
	}
	flush()
	return words
}

func hasLetterOrNumber(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

func stripChrome(n *html.Node) {
	var drop []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if isChrome(node) {
			drop = append(drop, node)
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	for _, node := range drop {
		if node.Parent != nil {
			node.Parent.RemoveChild(node)
		}
	}
}

func isChrome(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	switch n.Data {
	case "script", "style", "noscript", "svg", "nav", "aside", "dialog", "iframe", "template", "form", "button":
		return true
	case "header", "footer":
		if !insideArticle(n) {
			return true
		}
	}
	switch strings.ToLower(attr(n, "role")) {
	case "navigation", "banner", "contentinfo", "complementary", "dialog", "alertdialog", "search", "menu", "menubar":
		return true
	}
	if hasAttr(n, "hidden") || strings.EqualFold(attr(n, "aria-hidden"), "true") {
		return true
	}
	for _, tok := range classTokens(attr(n, "class") + " " + attr(n, "id")) {
		if chromeTokens[tok] {
			return true
		}
	}
	return false
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return true
		}
	}
	return false
}

func insideArticle(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "article" {
			return true
		}
	}
	return false
}

func classTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

var chromeTokens = map[string]bool{
	"nav": true, "navbar": true, "navigation": true, "menu": true, "menubar": true,
	"sidebar": true, "popup": true, "modal": true, "dialog": true, "overlay": true,
	"cookie": true, "cookiebar": true, "consent": true, "gdpr": true,
	"breadcrumb": true, "pagination": true, "pager": true,
	"masthead": true, "banner": true,
	"advert": true, "advertisement": true, "ads": true,
	"newsletter": true,
}

func contentRoot(doc *html.Node) *html.Node {
	if n := findElement(doc, func(n *html.Node) bool { return n.Data == "article" }); n != nil {
		return n
	}
	if n := findElement(doc, func(n *html.Node) bool {
		return n.Data == "main" || strings.EqualFold(attr(n, "role"), "main")
	}); n != nil {
		return n
	}
	if n := findElement(doc, func(n *html.Node) bool { return n.Data == "body" }); n != nil {
		return n
	}
	return doc
}

func findElement(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, match); found != nil {
			return found
		}
	}
	return nil
}
