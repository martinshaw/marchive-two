package provider

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// tweetMedia looks up video and audio that the page only exposes as a blob URL.
var tweetMedia = fetchTweetMedia

type xMedia struct {
	URL  string
	Kind string
}

func fetchTweetMedia(ctx context.Context, postURL string) []xMedia {
	id := statusID(postURL)
	if id == "" {
		return nil
	}
	endpoint := "https://cdn.syndication.twimg.com/tweet-result?id=" + url.QueryEscape(id) + "&lang=en"
	var body []byte
	_, err := withRetry(ctx, 1, func(ctx context.Context) error {
		var getErr error
		body, getErr = getBytes(ctx, mediaClient, endpoint, 2<<20)
		return getErr
	})
	if err != nil {
		return nil
	}
	return parseSyndication(body)
}

func statusID(raw string) string {
	ref, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(ref.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "status" && digits(parts[i+1]) {
			return parts[i+1]
		}
	}
	return ""
}

type synMedia struct {
	Type          string `json:"type"`
	MediaURLHTTPS string `json:"media_url_https"`
	VideoInfo     struct {
		Variants []synVariant `json:"variants"`
	} `json:"video_info"`
}

type synVariant struct {
	Bitrate     int    `json:"bitrate"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}

func parseSyndication(body []byte) []xMedia {
	var payload struct {
		MediaDetails []synMedia `json:"mediaDetails"`
		Tweet        struct {
			MediaDetails []synMedia `json:"mediaDetails"`
		} `json:"tweet"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	details := payload.MediaDetails
	if len(details) == 0 {
		details = payload.Tweet.MediaDetails
	}
	var out []xMedia
	for _, item := range details {
		switch strings.ToLower(item.Type) {
		case "photo":
			if item.MediaURLHTTPS != "" {
				out = append(out, xMedia{URL: item.MediaURLHTTPS, Kind: "image"})
			}
		case "video", "animated_gif":
			if raw := bestVariant(item.VideoInfo.Variants, "video/"); raw != "" {
				out = append(out, xMedia{URL: raw, Kind: "video"})
			}
		case "audio":
			raw := bestVariant(item.VideoInfo.Variants, "audio/")
			if raw == "" {
				raw = item.MediaURLHTTPS
			}
			if raw != "" {
				out = append(out, xMedia{URL: raw, Kind: "audio"})
			}
		}
	}
	return out
}

func bestVariant(variants []synVariant, prefix string) string {
	best := ""
	rate := -1
	for _, variant := range variants {
		if !strings.HasPrefix(strings.ToLower(variant.ContentType), prefix) || variant.URL == "" {
			continue
		}
		if variant.Bitrate >= rate {
			rate = variant.Bitrate
			best = variant.URL
		}
	}
	return best
}

func mediaFrom(root *html.Node) []xMedia {
	var out []xMedia
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "img":
				if raw := bestImageURL(node); raw != "" {
					out = addXMedia(out, seen, raw, "img")
				}
			case "video":
				out = addXMedia(out, seen, attr(node, "src"), "video")
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.ElementNode && child.Data == "source" {
						out = addXMedia(out, seen, attr(child, "src"), "video")
					}
				}
			case "audio":
				out = addXMedia(out, seen, attr(node, "src"), "audio")
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.ElementNode && child.Data == "source" {
						out = addXMedia(out, seen, attr(child, "src"), "audio")
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func addXMedia(dst []xMedia, seen map[string]bool, raw, tag string) []xMedia {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "blob:") || strings.HasPrefix(raw, "data:") {
		return dst
	}
	kind := mediaKind(raw, tag)
	if kind == "" {
		return dst
	}
	raw = preferOriginal(raw)
	key := mediaKey(raw)
	if seen[key] {
		return dst
	}
	seen[key] = true
	return append(dst, xMedia{URL: raw, Kind: kind})
}

func mergeXMedia(base, extra []xMedia) []xMedia {
	seen := map[string]bool{}
	out := make([]xMedia, 0, len(base)+len(extra))
	for _, item := range append(append([]xMedia{}, base...), extra...) {
		item.URL = preferOriginal(item.URL)
		key := mediaKey(item.URL)
		if item.URL == "" || item.Kind == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func bestImageURL(node *html.Node) string {
	src := strings.TrimSpace(firstNonEmpty(attr(node, "src"), attr(node, "data-src")))
	best := src
	bestW := srcWidth(src)
	for _, part := range strings.Split(attr(node, "srcset"), ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		candidate := fields[0]
		width := 0
		if len(fields) > 1 && strings.HasSuffix(fields[1], "w") {
			width, _ = strconv.Atoi(strings.TrimSuffix(fields[1], "w"))
		}
		if best == "" || width > bestW {
			best = candidate
			bestW = width
		}
	}
	return best
}

func srcWidth(raw string) int {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	switch strings.ToLower(u.Query().Get("name")) {
	case "orig":
		return 100000
	case "large":
		return 4096
	case "medium":
		return 1200
	case "small":
		return 680
	default:
		return 0
	}
}

func mediaKind(raw, tag string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	path := strings.ToLower(u.Path)
	if strings.Contains(host, "twimg.com") {
		switch {
		case strings.Contains(path, "/profile_images"), strings.Contains(path, "/profile_banners"), strings.Contains(path, "/emoji"):
			return ""
		case strings.Contains(path, "_thumb/"):
			return ""
		case strings.Contains(path, "/tweet_video/"), strings.Contains(path, "/ext_tw_video/"), strings.Contains(path, "/amplify_video/"):
			return "video"
		case strings.Contains(path, "/media/"):
			if strings.EqualFold(u.Query().Get("format"), "mp4") {
				return "video"
			}
			return "image"
		}
	}
	switch strings.ToLower(urlPathExt(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		if tag == "video" || tag == "audio" {
			return ""
		}
		return "image"
	case ".mp4", ".webm", ".mov", ".m4v":
		return "video"
	case ".mp3", ".m4a", ".aac", ".ogg", ".wav", ".opus":
		return "audio"
	}
	if tag == "audio" {
		return "audio"
	}
	if tag == "video" {
		return "video"
	}
	return ""
}

func urlPathExt(path string) string {
	dot := strings.LastIndex(path, ".")
	if dot < 0 || strings.Contains(path[dot:], "/") {
		return ""
	}
	return path[dot:]
}

func preferOriginal(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if !strings.EqualFold(u.Hostname(), "pbs.twimg.com") || !strings.Contains(strings.ToLower(u.Path), "/media/") {
		return raw
	}
	q := u.Query()
	if q.Get("name") == "" || q.Get("name") == "orig" {
		return raw
	}
	q.Set("name", "orig")
	u.RawQuery = q.Encode()
	return u.String()
}

func mediaKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Del("name")
	u.RawQuery = q.Encode()
	return u.String()
}

func remoteMedia(root *html.Node, found []xMedia) bool {
	images, av := 0, 0
	for _, item := range found {
		if item.Kind == "image" {
			images++
		} else if item.Kind != "" {
			av++
		}
	}
	photo, player := false, false
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "video", "audio":
				player = true
			}
			switch attr(node, "data-testid") {
			case "videoPlayer", "videoComponent", "audio", "tweetVoice", "voice":
				player = true
			case "tweetPhoto":
				photo = true
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if player && av == 0 {
		return true
	}
	if photo && images == 0 {
		return true
	}
	return false
}

func mediaURLs(items []xMedia) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.URL != "" {
			out = append(out, item.URL)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func saveTweetMedia(ctx context.Context, dir string, media []xMedia) error {
	count := map[string]int{}
	for _, item := range media {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.URL == "" || item.Kind == "" {
			continue
		}
		count[item.Kind]++
		stem := item.Kind + "-" + strconv.Itoa(count[item.Kind])
		_, err := withRetry(ctx, 1, func(ctx context.Context) error {
			_, err := downloadNamed(ctx, mediaClient, item.URL, dir, stem, item.Kind)
			return err
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			count[item.Kind]--
		}
	}
	return nil
}
