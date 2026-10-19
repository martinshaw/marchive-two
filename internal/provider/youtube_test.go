package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestYouTubeID(t *testing.T) {
	cases := []struct {
		raw string
		id  string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://m.youtube.com/watch?v=dQw4w9WgXcQ&list=PL", "dQw4w9WgXcQ"},
		{"https://music.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
	}
	for _, tc := range cases {
		id, err := youtubeID(tc.raw)
		if err != nil || id != tc.id {
			t.Fatalf("%s id %s %v", tc.raw, id, err)
		}
	}
	for _, raw := range []string{
		"https://www.youtube.com/@channel",
		"https://www.youtube.com/playlist?list=PL123",
		"https://example.com/watch?v=dQw4w9WgXcQ",
	} {
		if _, err := youtubeID(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestYouTubeFormatsAndCommentLimit(t *testing.T) {
	if videoFormat("low", false) != "best[height<=480]/worst" {
		t.Fatal(videoFormat("low", false))
	}
	if videoFormat("high", true) != "bv*+ba/b" {
		t.Fatal(videoFormat("high", true))
	}
	if audioFormat("opus") != "bestaudio[ext=webm]/bestaudio" {
		t.Fatal(audioFormat("opus"))
	}
	if commentArg("all") != "youtube:max_comments=all" || commentArg("40") != "youtube:max_comments=40" {
		t.Fatal(commentArg("all"))
	}
	raw, err := NormalizeStored(KindYouTube, `{"all":true,"videoQuality":"low","audioFormat":"m4a","commentLimit":"40"}`)
	if err != nil {
		t.Fatal(err)
	}
	got := Resolve(KindYouTube, raw)
	if !got.Video || !got.Audio || !got.Comments || !got.Subtitles || !got.Metadata || got.VideoQuality != "low" || got.AudioFormat != "m4a" || got.CommentLimit != "40" {
		t.Fatalf("resolved %+v from %s", got, raw)
	}
	if _, err := NormalizeStored(KindYouTube, `{"all":true,"commentLimit":"0"}`); err == nil {
		t.Fatal("expected an invalid comment count")
	}
}

func TestYouTubeRunSavesSelectedParts(t *testing.T) {
	origRun, origBin, origFFmpeg := runYtDlp, youtubeBinary, ffmpegAvailable
	t.Cleanup(func() {
		runYtDlp, youtubeBinary, ffmpegAvailable = origRun, origBin, origFFmpeg
	})
	youtubeBinary = func(string) (string, error) { return "yt-dlp", nil }
	ffmpegAvailable = func() bool { return false }
	var calls [][]string
	runYtDlp = func(_ context.Context, _ string, args []string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		dir := ""
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-P" {
				dir = args[i+1]
			}
		}
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "--write-info-json"):
			raw := []byte(`{"title":"City lights","description":"A night drive.","channel":"Roads","upload_date":"20200102","duration":65,"view_count":3,"comments":[{"author":"Ada","text":"Nice","like_count":2,"parent":"root"}]}`)
			if err := os.WriteFile(filepath.Join(dir, "meta.info.json"), raw, 0o644); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(dir, "meta.en.vtt"), []byte("WEBVTT\n"), 0o644); err != nil {
				return nil, err
			}
		case strings.Contains(joined, "video.%(ext)s"):
			if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("video"), 0o644); err != nil {
				return nil, err
			}
		case strings.Contains(joined, "audio.%(ext)s"):
			if err := os.WriteFile(filepath.Join(dir, "audio.m4a"), []byte("audio"), 0o644); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}

	st, req := testReq(t, KindYouTube)
	req.Source.URL = "https://youtu.be/dQw4w9WgXcQ"
	req.Source.Name = ""
	req.Source.Capture = `{"all":true,"videoQuality":"low","audioFormat":"m4a","commentLimit":"40"}`
	req.Fetch = staticFetch{shot: []byte("jpeg")}
	summary, err := (YouTube{}).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Note != "1 new, 0 already archived, 0 failed" {
		t.Fatalf("note %q", summary.Note)
	}
	if len(calls) != 3 {
		t.Fatalf("calls %d", len(calls))
	}
	if !strings.Contains(strings.Join(calls[0], " "), "max_comments=40") || !strings.Contains(strings.Join(calls[1], " "), "height<=480") || !strings.Contains(strings.Join(calls[2], " "), "ext=m4a") {
		t.Fatalf("args %#v", calls)
	}
	located, err := st.ListLocatedItems()
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.GetItem(located[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "City lights" || item.URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("item %+v", item)
	}
	for _, name := range []string{"text.txt", "metadata.json", "comments.json", "subtitles.vtt", "video.mp4", "audio.m4a", "screenshot.jpg"} {
		if _, err := os.Stat(filepath.Join(item.DownloadLocation, name)); err != nil {
			t.Fatalf("missing %s", name)
		}
	}
	meta, err := os.ReadFile(filepath.Join(item.DownloadLocation, "metadata.json"))
	if err != nil || !strings.Contains(string(meta), "A night drive.") || !strings.Contains(string(meta), "2020-01-02") {
		t.Fatalf("metadata %s %v", meta, err)
	}
	if _, err := os.Stat(filepath.Join(item.DownloadLocation, "meta.info.json")); !os.IsNotExist(err) {
		t.Fatal("raw info json was kept")
	}
	again, err := (YouTube{}).Run(context.Background(), req)
	if err != nil || again.Note != "0 new, 1 already archived, 0 failed" {
		t.Fatalf("again %q %v", again.Note, err)
	}
}

func TestYouTubeCheckAddress(t *testing.T) {
	orig := youtubeTitle
	t.Cleanup(func() { youtubeTitle = orig })
	youtubeTitle = func(context.Context, string) string { return "City lights" }
	title, err := CheckAddress(context.Background(), KindYouTube, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil || title != "City lights" {
		t.Fatalf("title %q %v", title, err)
	}
	if _, err := CheckAddress(context.Background(), KindYouTube, "https://www.youtube.com/feed/subscriptions"); err == nil {
		t.Fatal("expected a video address")
	}
}

func TestYouTubeScreenshotUsesChrome(t *testing.T) {
	orig := runYtDlp
	t.Cleanup(func() { runYtDlp = orig })
	runYtDlp = func(context.Context, string, []string) ([]byte, error) {
		t.Fatal("screenshot-only should not run yt-dlp")
		return nil, nil
	}
	_, req := testReq(t, KindYouTube)
	req.Source.URL = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	req.Source.Capture = `{"screenshot":true}`
	req.Fetch = staticFetch{shot: []byte("jpeg")}
	if _, err := (YouTube{}).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}
