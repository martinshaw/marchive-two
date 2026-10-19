package provider

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Capture is the set of files a source keeps. All means every part for that kind.
type Capture struct {
	All        bool `json:"all,omitempty"`
	HTML       bool `json:"html,omitempty"`
	Text       bool `json:"text,omitempty"`
	Screenshot bool `json:"screenshot,omitempty"`
	MHTML      bool `json:"mhtml,omitempty"`
	Metadata   bool `json:"metadata,omitempty"`
	Words      bool `json:"words,omitempty"`
	Audio      bool `json:"audio,omitempty"`
	Media      bool `json:"media,omitempty"`
	Comments   bool `json:"comments,omitempty"`
	Subtitles  bool `json:"subtitles,omitempty"`
	Video      bool `json:"video,omitempty"`
	// VideoQuality is low, mid, or high. Empty means high.
	VideoQuality string `json:"videoQuality,omitempty"`
	// AudioFormat is best, m4a, or opus. Empty means best.
	AudioFormat string `json:"audioFormat,omitempty"`
	// CommentLimit is "all" or a count. Empty means all.
	CommentLimit string `json:"commentLimit,omitempty"`
}

func (c Capture) mode() saveMode {
	return saveMode{
		HTML:       c.HTML,
		Text:       c.Text,
		Screenshot: c.Screenshot,
		MHTML:      c.MHTML,
		Metadata:   c.Metadata,
		Words:      c.Words,
		Media:      c.Media,
	}
}

// Resolve turns stored JSON into the parts that apply to kind. Empty JSON keeps everything.
func Resolve(kind, raw string) Capture {
	raw = strings.TrimSpace(raw)
	var stored Capture
	if raw == "" || json.Unmarshal([]byte(raw), &stored) != nil || stored.All {
		out := everything(kind)
		applyYouTubeOptions(kind, &out, stored)
		return out
	}
	out := onlyKind(kind, stored)
	applyYouTubeOptions(kind, &out, stored)
	return out
}

// EncodeSelection stores either every part or the listed part keys.
func EncodeSelection(kind string, all bool, selected []string) (string, error) {
	if !ValidKind(kind) {
		return "", errors.New(KindHint())
	}
	if all {
		return `{"all":true}`, nil
	}
	allowed := map[string]bool{}
	for _, key := range partKeys(kind) {
		allowed[key] = true
	}
	chosen := Capture{}
	for _, key := range selected {
		if allowed[key] {
			setPart(&chosen, key, true)
		}
	}
	if !anyPart(kind, chosen) {
		return "", errors.New("choose at least one part to capture")
	}
	raw, err := json.Marshal(chosen)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// NormalizeStored accepts empty, {"all":true}, or a partial selection and returns the stored JSON.
func NormalizeStored(kind, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	var stored Capture
	if raw != "" && raw != `{"all":true}` {
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			return "", errors.New("capture selection is not valid")
		}
	}
	quality, audio, comments, err := youtubeOptions(kind, stored)
	if err != nil {
		return "", err
	}
	if raw == "" || raw == `{"all":true}` || stored.All {
		return encodeParts(kind, true, nil, quality, audio, comments)
	}
	var keys []string
	for _, key := range partKeys(kind) {
		if partOn(stored, key) {
			keys = append(keys, key)
		}
	}
	return encodeParts(kind, false, keys, quality, audio, comments)
}

func encodeParts(kind string, all bool, selected []string, quality, audio, comments string) (string, error) {
	base, err := EncodeSelection(kind, all, selected)
	if err != nil {
		return "", err
	}
	if kind != KindYouTube {
		return base, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(base), &doc); err != nil {
		return "", err
	}
	if quality != "" && quality != "high" {
		doc["videoQuality"] = quality
	}
	if audio != "" && audio != "best" {
		doc["audioFormat"] = audio
	}
	if comments != "" && comments != "all" {
		doc["commentLimit"] = comments
	}
	if len(doc) == 1 {
		if on, ok := doc["all"].(bool); ok && on {
			return `{"all":true}`, nil
		}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func youtubeOptions(kind string, stored Capture) (quality, audio, comments string, err error) {
	if kind != KindYouTube {
		return "", "", "", nil
	}
	quality, err = canonicalQuality(stored.VideoQuality)
	if err != nil {
		return "", "", "", err
	}
	audio, err = canonicalAudio(stored.AudioFormat)
	if err != nil {
		return "", "", "", err
	}
	comments, err = canonicalComments(stored.CommentLimit)
	return quality, audio, comments, err
}

func canonicalQuality(raw string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "high":
		return "high", nil
	case "low", "mid":
		return strings.TrimSpace(strings.ToLower(raw)), nil
	default:
		return "", errors.New("choose a low, medium, or high video")
	}
}

func canonicalAudio(raw string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "best":
		return "best", nil
	case "m4a", "opus":
		return strings.TrimSpace(strings.ToLower(raw)), nil
	default:
		return "", errors.New("choose best, m4a, or opus audio")
	}
}

func canonicalComments(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "all" {
		return "all", nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100000 {
		return "", errors.New("choose all comments, or a number from 1 to 100000")
	}
	return strconv.Itoa(n), nil
}

func applyYouTubeOptions(kind string, out *Capture, stored Capture) {
	if out == nil || kind != KindYouTube {
		return
	}
	quality, _ := canonicalQuality(stored.VideoQuality)
	audio, _ := canonicalAudio(stored.AudioFormat)
	comments, _ := canonicalComments(stored.CommentLimit)
	if quality == "" {
		quality = "high"
	}
	if audio == "" {
		audio = "best"
	}
	if comments == "" {
		comments = "all"
	}
	out.VideoQuality = quality
	out.AudioFormat = audio
	out.CommentLimit = comments
}

func everything(kind string) Capture {
	out := Capture{All: true}
	for _, key := range partKeys(kind) {
		setPart(&out, key, true)
	}
	return out
}

func onlyKind(kind string, stored Capture) Capture {
	out := Capture{}
	for _, key := range partKeys(kind) {
		if partOn(stored, key) {
			setPart(&out, key, true)
		}
	}
	if !anyPart(kind, out) {
		return everything(kind)
	}
	return out
}

func partKeys(kind string) []string {
	switch kind {
	case KindWebpage, KindArticles:
		return []string{"html", "text", "screenshot", "mhtml", "metadata", "words"}
	case KindText:
		return []string{"text", "metadata"}
	case KindPodcast:
		return []string{"audio", "metadata"}
	case KindTrending:
		return []string{"text", "metadata", "words", "media"}
	case KindHN:
		return []string{"text", "comments", "screenshot", "metadata", "words"}
	case KindGoogle, KindDDG:
		return []string{"text", "screenshot", "metadata", "words"}
	case KindYouTube:
		return []string{"metadata", "subtitles", "video", "audio", "comments", "screenshot"}
	default:
		return nil
	}
}

func setPart(c *Capture, key string, on bool) {
	switch key {
	case "html":
		c.HTML = on
	case "text":
		c.Text = on
	case "screenshot":
		c.Screenshot = on
	case "mhtml":
		c.MHTML = on
	case "metadata":
		c.Metadata = on
	case "words":
		c.Words = on
	case "audio":
		c.Audio = on
	case "media":
		c.Media = on
	case "comments":
		c.Comments = on
	case "subtitles":
		c.Subtitles = on
	case "video":
		c.Video = on
	}
}

func partOn(c Capture, key string) bool {
	switch key {
	case "html":
		return c.HTML
	case "text":
		return c.Text
	case "screenshot":
		return c.Screenshot
	case "mhtml":
		return c.MHTML
	case "metadata":
		return c.Metadata
	case "words":
		return c.Words
	case "audio":
		return c.Audio
	case "media":
		return c.Media
	case "comments":
		return c.Comments
	case "subtitles":
		return c.Subtitles
	case "video":
		return c.Video
	default:
		return false
	}
}

func anyPart(kind string, c Capture) bool {
	for _, key := range partKeys(kind) {
		if partOn(c, key) {
			return true
		}
	}
	return false
}
