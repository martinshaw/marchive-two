package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/paths"
	"github.com/martinshaw/marchive-two/internal/store"
	"github.com/martinshaw/marchive-two/internal/ytdlp"
)

// YouTube archives one video page: description, subtitles, picture, sound, and comments.
type YouTube struct{}

func (YouTube) Kind() string { return KindYouTube }

func (YouTube) Run(ctx context.Context, req Request) (Summary, error) {
	id, err := youtubeID(req.Source.URL)
	if err != nil {
		return Summary{}, err
	}
	page := youtubeWatchURL(id)
	seen, err := req.Store.ItemSeen(req.Source.ID, page, strings.TrimSpace(req.Source.URL))
	if err != nil {
		return Summary{}, err
	}
	if seen {
		return Summary{Note: "0 new, 1 already archived, 0 failed"}, nil
	}
	mode := Resolve(req.Source.Provider, req.Source.Capture)
	needsTool := mode.Metadata || mode.Subtitles || mode.Video || mode.Audio || mode.Comments
	bin := ""
	if needsTool {
		bin, err = youtubeBinary(req.DataDir)
		if err != nil {
			return Summary{}, errors.New("yt-dlp is not installed. Install it from Settings, then run this capture again.")
		}
	}
	root, err := sourceCaptureDir(req)
	if err != nil {
		return Summary{}, err
	}
	dir := filepath.Join(root, SafeFileName("YouTube "+id, page))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Summary{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(dir)
		}
	}()

	var shot []byte
	if mode.Screenshot && !needsTool && req.Fetch == nil {
		return Summary{}, errors.New("this capture needs Chrome")
	}
	if mode.Screenshot && req.Fetch != nil {
		_, ferr := withRetry(ctx, 1, func(ctx context.Context) error {
			doc, fetchErr := req.Fetch.Fetch(ctx, page, browser.Options{Screenshot: true, Scroll: true})
			if fetchErr != nil {
				return fetchErr
			}
			if doc != nil {
				shot = doc.Screenshot
			}
			return nil
		})
		if ferr != nil && ctx.Err() != nil {
			return Summary{}, ctx.Err()
		}
	}

	var info ytInfo
	if needsTool {
		info, err = downloadYouTube(ctx, bin, youtubeProfile(req.DataDir), page, dir, mode)
		if err != nil {
			return Summary{}, err
		}
	}
	title := strings.TrimSpace(info.Title)
	if title == "" {
		title = "YouTube " + id
	}
	if mode.Metadata {
		if err := writeYouTubeText(dir, title, info.Description); err != nil {
			return Summary{}, err
		}
		if err := writeYouTubeMeta(dir, page, title, info); err != nil {
			return Summary{}, err
		}
	}
	if mode.Comments {
		if err := writeYouTubeComments(dir, info.Comments); err != nil {
			return Summary{}, err
		}
	}
	if mode.Subtitles {
		if err := keepSubtitle(dir); err != nil {
			return Summary{}, err
		}
	} else {
		removeSubtitles(dir)
	}
	if mode.Screenshot && len(shot) > 0 {
		if err := os.WriteFile(filepath.Join(dir, "screenshot.jpg"), shot, 0o644); err != nil {
			return Summary{}, err
		}
	}
	removeInfoDump(dir)
	if !mode.Video {
		removePrefixed(dir, "video.")
	}
	if !mode.Audio {
		removePrefixed(dir, "audio.")
	}
	hash := ContentHash(title + "\n" + info.Description)
	if _, err := req.Store.InsertItem(store.Item{
		SourceID:         req.Source.ID,
		RunID:            req.RunID,
		IdentityKey:      page,
		Kind:             "video",
		Title:            title,
		URL:              page,
		Status:           store.StatusSucceeded,
		DownloadLocation: dir,
		ContentHash:      hash,
	}); err != nil {
		return Summary{}, err
	}
	cleanup = false
	maybeRenameSource(req, title)
	return Summary{Note: "1 new, 0 already archived, 0 failed"}, nil
}

func checkYouTube(ctx context.Context, rawURL string) (string, error) {
	id, err := youtubeID(rawURL)
	if err != nil {
		return "", err
	}
	if title := youtubeTitle(ctx, id); title != "" {
		return title, nil
	}
	return "YouTube video", nil
}

func youtubeWatchURL(id string) string {
	return "https://www.youtube.com/watch?v=" + id
}

func youtubeID(raw string) (string, error) {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (ref.Scheme != "http" && ref.Scheme != "https") {
		return "", errNotYouTube
	}
	host := strings.TrimPrefix(strings.ToLower(ref.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	var id string
	switch host {
	case "youtu.be":
		id = strings.Trim(ref.Path, "/")
		if cut := strings.Index(id, "/"); cut >= 0 {
			id = id[:cut]
		}
	case "youtube.com", "music.youtube.com", "youtube-nocookie.com":
		if v := ref.Query().Get("v"); v != "" {
			id = v
			break
		}
		parts := strings.Split(strings.Trim(ref.Path, "/"), "/")
		if len(parts) >= 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live" || parts[0] == "v") {
			id = parts[1]
		}
	}
	if !youtubeIDOK(id) {
		return "", errNotYouTube
	}
	return id, nil
}

func youtubeIDOK(id string) bool {
	if len(id) != 11 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

var youtubeTitle = func(ctx context.Context, id string) string {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	endpoint := "https://www.youtube.com/oembed?format=json&url=" + url.QueryEscape(youtubeWatchURL(id))
	body, err := getBytes(ctx, mediaClient, endpoint, 1<<20)
	if err != nil {
		return ""
	}
	var doc struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return ""
	}
	return strings.TrimSpace(doc.Title)
}

var youtubeBinary = ytdlp.Executable

func youtubeProfile(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		resolved, err := paths.Resolve("")
		if err != nil {
			return ""
		}
		dataDir = resolved
	}
	return paths.Profile(dataDir)
}

type ytInfo struct {
	Title       string
	Description string
	Channel     string
	ChannelURL  string
	Duration    int
	UploadDate  string
	ViewCount   int
	LikeCount   int
	Comments    []ytComment
}

type ytComment struct {
	Author string `json:"author,omitempty"`
	Text   string `json:"text"`
	Likes  int    `json:"likes,omitempty"`
	Time   string `json:"time,omitempty"`
	Parent string `json:"parent,omitempty"`
}

type ytDump struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Channel     string `json:"channel"`
	ChannelURL  string `json:"channel_url"`
	Duration    int    `json:"duration"`
	UploadDate  string `json:"upload_date"`
	ViewCount   int    `json:"view_count"`
	LikeCount   int    `json:"like_count"`
	Comments    []struct {
		Author    string `json:"author"`
		Text      string `json:"text"`
		LikeCount int    `json:"like_count"`
		TimeText  string `json:"time_text"`
		Parent    string `json:"parent"`
	} `json:"comments"`
}

var runYtDlp = func(ctx context.Context, bin string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	return cmd.CombinedOutput()
}

var ffmpegAvailable = func() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func downloadYouTube(ctx context.Context, bin, profile, page, dir string, mode Capture) (ytInfo, error) {
	cookies := cookieArgs(profile)
	var info ytInfo
	if mode.Metadata || mode.Subtitles || mode.Comments || mode.Video || mode.Audio {
		args := infoArgs(page, dir, cookies, mode.Subtitles, mode.Comments, mode.CommentLimit)
		if err := runTool(ctx, bin, args, cookies); err != nil {
			return ytInfo{}, err
		}
		parsed, err := readInfoDump(dir)
		if err != nil {
			return ytInfo{}, err
		}
		info = parsed
	}
	if mode.Video {
		args := mediaArgs(page, dir, cookies, "video.%(ext)s", videoFormat(mode.VideoQuality, ffmpegAvailable()))
		if err := runTool(ctx, bin, args, cookies); err != nil {
			return ytInfo{}, err
		}
	}
	if mode.Audio {
		args := mediaArgs(page, dir, cookies, "audio.%(ext)s", audioFormat(mode.AudioFormat))
		if err := runTool(ctx, bin, args, cookies); err != nil {
			return ytInfo{}, err
		}
	}
	return info, nil
}

func runTool(ctx context.Context, bin string, args []string, cookies []string) error {
	err := runOnce(ctx, bin, args)
	if err == nil || len(cookies) == 0 || !cookieFailure(err) {
		return err
	}
	return runOnce(ctx, bin, withoutCookies(args))
}

func runOnce(ctx context.Context, bin string, args []string) error {
	_, err := withRetry(ctx, 1, func(ctx context.Context) error {
		out, runErr := runYtDlp(ctx, bin, args)
		if runErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = runErr.Error()
		}
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return errors.New(msg)
	})
	return err
}

func cookieArgs(profile string) []string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return nil
	}
	cookies := filepath.Join(profile, "Default", "Cookies")
	if _, err := os.Stat(cookies); err != nil {
		return nil
	}
	return []string{"--cookies-from-browser", "chrome:" + profile}
}

func cookieFailure(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "cookie") || strings.Contains(text, "decrypt")
}

func withoutCookies(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--cookies-from-browser" {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func baseArgs(dir string, cookies []string) []string {
	args := []string{"--no-playlist", "--no-warnings", "--no-progress", "-P", dir}
	return append(args, cookies...)
}

func infoArgs(page, dir string, cookies []string, subs, comments bool, limit string) []string {
	args := baseArgs(dir, cookies)
	args = append(args, "--skip-download", "--write-info-json", "-o", "meta.%(ext)s")
	if subs {
		args = append(args, "--write-subs", "--write-auto-subs", "--sub-langs", "en.*,en", "--sub-format", "vtt/best")
	}
	if comments {
		args = append(args, "--write-comments", "--extractor-args", commentArg(limit))
	}
	return append(args, page)
}

func mediaArgs(page, dir string, cookies []string, template, format string) []string {
	args := baseArgs(dir, cookies)
	return append(args, "-f", format, "-o", template, page)
}

func commentArg(limit string) string {
	limit = strings.TrimSpace(limit)
	if limit == "" || limit == "all" {
		return "youtube:max_comments=all"
	}
	return "youtube:max_comments=" + limit
}

func videoFormat(quality string, ffmpeg bool) string {
	switch quality {
	case "low":
		if ffmpeg {
			return "bv*[height<=480]+ba/b[height<=480]/worst"
		}
		return "best[height<=480]/worst"
	case "mid":
		if ffmpeg {
			return "bv*[height<=720]+ba/b[height<=720]/best"
		}
		return "best[height<=720]/best"
	default:
		if ffmpeg {
			return "bv*+ba/b"
		}
		return "best"
	}
}

func audioFormat(kind string) string {
	switch kind {
	case "m4a":
		return "bestaudio[ext=m4a]/bestaudio"
	case "opus":
		return "bestaudio[ext=webm]/bestaudio"
	default:
		return "bestaudio/best"
	}
}

func readInfoDump(dir string) (ytInfo, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.info.json"))
	if err != nil {
		return ytInfo{}, err
	}
	if len(matches) == 0 {
		return ytInfo{}, errors.New("yt-dlp did not write video details")
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return ytInfo{}, err
	}
	var dump ytDump
	if err := json.Unmarshal(raw, &dump); err != nil {
		return ytInfo{}, err
	}
	info := ytInfo{
		Title:       strings.TrimSpace(dump.Title),
		Description: strings.TrimSpace(dump.Description),
		Channel:     strings.TrimSpace(dump.Channel),
		ChannelURL:  strings.TrimSpace(dump.ChannelURL),
		Duration:    dump.Duration,
		UploadDate:  formatUploadDate(dump.UploadDate),
		ViewCount:   dump.ViewCount,
		LikeCount:   dump.LikeCount,
	}
	for _, comment := range dump.Comments {
		text := strings.TrimSpace(comment.Text)
		if text == "" {
			continue
		}
		parent := strings.TrimSpace(comment.Parent)
		if parent == "root" {
			parent = ""
		}
		info.Comments = append(info.Comments, ytComment{
			Author: strings.TrimSpace(comment.Author),
			Text:   text,
			Likes:  comment.LikeCount,
			Time:   strings.TrimSpace(comment.TimeText),
			Parent: parent,
		})
	}
	return info, nil
}

func formatUploadDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) != 8 {
		return raw
	}
	parsed, err := time.Parse("20060102", raw)
	if err != nil {
		return raw
	}
	return parsed.Format("2006-01-02")
}

func writeYouTubeText(dir, title, description string) error {
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(description)
	b.WriteString("\n")
	return os.WriteFile(filepath.Join(dir, "text.txt"), []byte(b.String()), 0o644)
}

func writeYouTubeMeta(dir, page, title string, info ytInfo) error {
	meta := SavedMeta{
		Title:        title,
		URL:          page,
		CanonicalURL: page,
		Description:  info.Description,
		Published:    info.UploadDate,
		SiteName:     "YouTube",
		Provider:     KindYouTube,
		Extractor:    "yt-dlp",
		ContentHash:  ContentHash(title + "\n" + info.Description),
		CapturedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	body := map[string]any{
		"title":        meta.Title,
		"url":          meta.URL,
		"canonicalUrl": meta.CanonicalURL,
		"description":  meta.Description,
		"channel":      strings.TrimSpace(info.Channel),
		"channelUrl":   info.ChannelURL,
		"duration":     info.Duration,
		"published":    info.UploadDate,
		"views":        info.ViewCount,
		"likes":        info.LikeCount,
		"siteName":     meta.SiteName,
		"provider":     meta.Provider,
		"extractor":    meta.Extractor,
		"contentHash":  meta.ContentHash,
		"capturedAt":   meta.CapturedAt,
	}
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), append(raw, '\n'), 0o644)
}

func writeYouTubeComments(dir string, comments []ytComment) error {
	if comments == nil {
		comments = []ytComment{}
	}
	raw, err := json.MarshalIndent(comments, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "comments.json"), append(raw, '\n'), 0o644)
}

func keepSubtitle(dir string) error {
	best := ""
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".vtt") && !strings.HasSuffix(lower, ".srt") && !strings.HasSuffix(lower, ".srv3") {
			continue
		}
		if best == "" || subtitleRank(lower) < subtitleRank(strings.ToLower(best)) {
			best = name
		}
	}
	if best == "" {
		return nil
	}
	ext := filepath.Ext(best)
	dest := filepath.Join(dir, "subtitles"+ext)
	if err := os.Rename(filepath.Join(dir, best), dest); err != nil {
		return err
	}
	removeSubtitlesExcept(dir, filepath.Base(dest))
	return nil
}

func subtitleRank(name string) int {
	switch {
	case strings.Contains(name, ".en."):
		return 0
	case strings.HasSuffix(name, ".vtt"):
		return 1
	case strings.HasSuffix(name, ".srt"):
		return 2
	default:
		return 3
	}
}

func removeSubtitles(dir string) {
	removeSubtitlesExcept(dir, "")
}

func removeSubtitlesExcept(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == keep {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".vtt") || strings.HasSuffix(lower, ".srt") || strings.HasSuffix(lower, ".srv3") {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

func removeInfoDump(dir string) {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.info.json"))
	for _, name := range matches {
		_ = os.Remove(name)
	}
}

func removePrefixed(dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}
