package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinshaw/marchive-two/internal/browser"
)

const trendingHTML = `<html><body>
<article data-testid="tweet">
  <div data-testid="User-Name">Ada Lovelace
@ada
·
1h</div>
  <div data-testid="tweetText">The city lights are up. #Night</div>
  <a href="/ada/status/1001">permalink</a>
  <a href="/hashtag/Night">#Night</a>
</article>
<article data-testid="tweet">
  <div data-testid="User-Name">Grace Hopper
@grace</div>
  <div data-testid="tweetText">Another bug in the log.</div>
  <a href="https://twitter.com/grace/status/1002/photo/1">photo</a>
</article>
<article data-testid="tweet">
  <div data-testid="User-Name">Ada Lovelace
@ada</div>
  <div data-testid="tweetText">The city lights are up. #Night</div>
  <a href="/ada/status/1001">again</a>
</article>
</body></html>`

func TestExtractPostsReadsAuthorTextURLAndHashtags(t *testing.T) {
	posts := extractPosts(trendingHTML, "https://x.com/explore/tabs/trending")
	if len(posts) != 2 {
		t.Fatalf("posts %d", len(posts))
	}
	if posts[0].Author != "Ada Lovelace" || posts[0].Handle != "@ada" || posts[0].URL != "https://x.com/ada/status/1001" {
		t.Fatalf("first %+v", posts[0])
	}
	if posts[0].Text != "The city lights are up. #Night" || len(posts[0].Hashtags) != 1 || posts[0].Hashtags[0] != "Night" {
		t.Fatalf("first text %+v", posts[0])
	}
	if posts[1].URL != "https://x.com/grace/status/1002" || posts[1].Author != "Grace Hopper" {
		t.Fatalf("second %+v", posts[1])
	}
}

func TestTrendingRunSavesEachNewPost(t *testing.T) {
	st, req := testReq(t, KindTrending)
	req.Source.URL = "https://x.com/explore/tabs/trending"
	req.Fetch = staticFetch{html: trendingHTML}
	summary, err := XTrending{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Note != "2 new, 0 already archived, 0 failed" {
		t.Fatalf("note %q", summary.Note)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(located) != 2 {
		t.Fatalf("items %d", len(located))
	}
	item, err := st.GetItem(located[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != "tweet" || item.URL != "https://x.com/ada/status/1001" {
		t.Fatalf("item %+v", item)
	}
	raw, err := os.ReadFile(filepath.Join(item.DownloadLocation, "text.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "city lights") {
		t.Fatalf("text %s", raw)
	}
	meta, err := os.ReadFile(filepath.Join(item.DownloadLocation, "metadata.json"))
	if err != nil || !strings.Contains(string(meta), "Ada Lovelace") || !strings.Contains(string(meta), "Night") {
		t.Fatalf("meta %s %v", meta, err)
	}
	words, err := os.ReadFile(filepath.Join(item.DownloadLocation, "words.json"))
	if err != nil || !strings.Contains(string(words), "city") {
		t.Fatalf("words %s %v", words, err)
	}
	again, err := XTrending{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if again.Note != "0 new, 2 already archived, 0 failed" {
		t.Fatalf("again %q", again.Note)
	}
}

func TestTopicPagesIgnoresPostsAndSectionLinks(t *testing.T) {
	page := `<html><body>
<a href="/explore/tabs/news">News</a>
<a href="/ada">Ada</a>
<a href="/ada/status/1001">a post</a>
<div data-testid="trend"><a href="/search?q=%23Night&amp;src=trend_click">#Night</a></div>
<div data-testid="trend"><a href="/search?q=%23Night&amp;src=promoted_trend_click">again</a></div>
<div data-testid="trend"><a href="/hashtag/City">City</a></div>
<a href="/i/events/99">A story</a>
<a href="https://example.com/search?q=no">elsewhere</a>
</body></html>`
	topics := topicPages(page, "https://x.com/explore/tabs/news")
	if len(topics) != 3 {
		t.Fatalf("topics %v", topics)
	}
	if topics[0] != "https://x.com/search?q=%23Night&src=trend_click" && topics[0] != "https://x.com/search?q=%23Night&src=promoted_trend_click" {
		t.Fatalf("first %s", topics[0])
	}
	if topics[1] != "https://x.com/hashtag/City" || topics[2] != "https://x.com/i/events/99" {
		t.Fatalf("rest %v", topics)
	}
}

func TestTrendingOpensAFewTopicPages(t *testing.T) {
	landing := `<html><body>`
	for i := 1; i <= 5; i++ {
		landing += `<div data-testid="trend"><a href="/search?q=Topic` + string(rune('A'+i-1)) + `&amp;src=trend_click">Topic</a></div>`
	}
	landing += `</body></html>`
	fetch := &mapFetch{pages: map[string]string{
		"https://x.com/explore/tabs/news": landing,
	}}
	topics := topicPages(landing, "https://x.com/explore/tabs/news")
	if len(topics) != 5 {
		t.Fatalf("topics %v", topics)
	}
	for i, topic := range topics[:topicPageLimit] {
		fetch.pages[topic] = `<html><body><article data-testid="tweet">
<div data-testid="User-Name">Ada Lovelace
@ada</div>
<div data-testid="tweetText">Topic note ` + string(rune('A'+i)) + `</div>
<a href="/ada/status/` + string(rune('1'+i)) + `">post</a>
</article></body></html>`
	}
	st, req := testReq(t, KindTrending)
	req.Source.URL = "https://x.com/explore/tabs/news"
	req.Fetch = fetch
	summary, err := XTrending{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary.Note, "Opened 4 topic pages") || !strings.Contains(summary.Note, "4 new") {
		t.Fatalf("note %q calls %v", summary.Note, fetch.calls)
	}
	if len(fetch.calls) != 1+topicPageLimit {
		t.Fatalf("calls %v", fetch.calls)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(located) != topicPageLimit {
		t.Fatalf("items %d", len(located))
	}
}

func TestTrendingKeepsPostsOnThePage(t *testing.T) {
	fetch := &mapFetch{pages: map[string]string{
		"https://x.com/explore/tabs/trending": trendingHTML + `<a href="/search?q=Other&amp;src=trend_click">Other</a>`,
	}}
	_, req := testReq(t, KindTrending)
	req.Source.URL = "https://x.com/explore/tabs/trending"
	req.Fetch = fetch
	summary, err := XTrending{}.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Note != "2 new, 0 already archived, 0 failed" {
		t.Fatalf("note %q", summary.Note)
	}
	if len(fetch.calls) != 1 {
		t.Fatalf("calls %v", fetch.calls)
	}
}

func TestTrendingNeedsAVisiblePost(t *testing.T) {
	_, req := testReq(t, KindTrending)
	req.Fetch = staticFetch{html: `<html><body>Sign in to X</body></html>`}
	_, err := XTrending{}.Run(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "Sign in with Chrome") {
		t.Fatalf("err %v", err)
	}
}

func TestTrendingSavesImageAudioAndVideo(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		switch r.URL.Path {
		case "/photo.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg-bytes"))
		case "/voice.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("mp3-bytes"))
		case "/clip.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("mp4-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	page := `<html><body><article data-testid="tweet">
<div data-testid="User-Name">Ada Lovelace
@ada</div>
<div data-testid="tweetText">A clip from the city. #Night</div>
<img src="https://pbs.twimg.com/profile_images/ada.jpg">
<img src="` + srv.URL + `/photo.jpg">
<audio src="` + srv.URL + `/voice.mp3"></audio>
<video src="` + srv.URL + `/clip.mp4"></video>
<a href="/ada/status/1001">permalink</a>
</article></body></html>`
	st, req := testReq(t, KindTrending)
	req.Source.URL = "https://x.com/explore/tabs/trending"
	req.Fetch = staticFetch{html: page}
	if _, err := (XTrending{}).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(located) != 1 {
		t.Fatalf("items %d", len(located))
	}
	item, err := st.GetItem(located[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"image-1.jpg", "audio-1.mp3", "video-1.mp4"} {
		raw, err := os.ReadFile(filepath.Join(item.DownloadLocation, name))
		if err != nil || len(raw) == 0 {
			t.Fatalf("%s %v %q", name, err, raw)
		}
	}
	if strings.Contains(strings.Join(hits, " "), "profile_images") {
		t.Fatalf("profile image was downloaded: %v", hits)
	}
	meta, err := os.ReadFile(filepath.Join(item.DownloadLocation, "metadata.json"))
	if err != nil || !strings.Contains(string(meta), "photo.jpg") || !strings.Contains(string(meta), "clip.mp4") {
		t.Fatalf("meta %s %v", meta, err)
	}
}

func TestTrendingLooksUpBlobVideo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("mp4-bytes"))
	}))
	defer srv.Close()
	previous := tweetMedia
	tweetMedia = func(context.Context, string) []xMedia {
		return []xMedia{{URL: srv.URL + "/from-syndication.mp4", Kind: "video"}}
	}
	t.Cleanup(func() { tweetMedia = previous })
	page := `<html><body><article data-testid="tweet">
<div data-testid="User-Name">Ada Lovelace
@ada</div>
<div data-testid="tweetText">Playing now.</div>
<div data-testid="videoPlayer"><video src="blob:https://x.com/abc"></video></div>
<a href="/ada/status/1009">permalink</a>
</article></body></html>`
	st, req := testReq(t, KindTrending)
	req.Fetch = staticFetch{html: page}
	if _, err := (XTrending{}).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.GetItem(located[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(item.DownloadLocation, "video-1.mp4"))
	if err != nil || string(raw) != "mp4-bytes" {
		t.Fatalf("video %q %v", raw, err)
	}
}

func TestParseSyndicationPicksTheLargestVideo(t *testing.T) {
	body := []byte(`{"mediaDetails":[
		{"type":"photo","media_url_https":"https://pbs.twimg.com/media/abc.jpg"},
		{"type":"video","video_info":{"variants":[
			{"bitrate":256000,"content_type":"video/mp4","url":"https://video.twimg.com/small.mp4"},
			{"content_type":"application/x-mpegURL","url":"https://video.twimg.com/clip.m3u8"},
			{"bitrate":2176000,"content_type":"video/mp4","url":"https://video.twimg.com/large.mp4"}
		]}},
		{"type":"audio","video_info":{"variants":[
			{"bitrate":64000,"content_type":"audio/mp4","url":"https://video.twimg.com/voice.m4a"}
		]}}
	]}`)
	got := parseSyndication(body)
	if len(got) != 3 || got[0].Kind != "image" || got[1].URL != "https://video.twimg.com/large.mp4" || got[2].Kind != "audio" {
		t.Fatalf("media %+v", got)
	}
	upgraded := preferOriginal("https://pbs.twimg.com/media/abc?format=jpg&name=small")
	if !strings.Contains(upgraded, "name=orig") || !strings.Contains(upgraded, "format=jpg") {
		t.Fatalf("upgraded %s", upgraded)
	}
}

func TestCheckAddressAcceptsXHostsOnly(t *testing.T) {
	title, err := CheckAddress(context.Background(), KindTrending, "https://x.com/explore/tabs/trending")
	if err != nil || title != "X trending" {
		t.Fatalf("x %q %v", title, err)
	}
	if _, err := CheckAddress(context.Background(), KindTrending, "https://twitter.com/explore"); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckAddress(context.Background(), KindTrending, "https://example.com"); err == nil {
		t.Fatal("example.com was accepted")
	}
}

type mapFetch struct {
	pages map[string]string
	calls []string
}

func (f *mapFetch) Fetch(_ context.Context, rawURL string, _ browser.Options) (*browser.Document, error) {
	f.calls = append(f.calls, rawURL)
	page := f.pages[rawURL]
	if page == "" {
		page = `<html><body></body></html>`
	}
	return &browser.Document{URL: rawURL, HTML: page}, nil
}
