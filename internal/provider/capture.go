package provider

import (
	"encoding/json"
	"errors"
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
}

func (c Capture) mode() saveMode {
	return saveMode{
		HTML:       c.HTML,
		Text:       c.Text,
		Screenshot: c.Screenshot,
		MHTML:      c.MHTML,
		Metadata:   c.Metadata,
		Words:      c.Words,
	}
}

// Resolve turns stored JSON into the parts that apply to kind. Empty JSON keeps everything.
func Resolve(kind, raw string) Capture {
	raw = strings.TrimSpace(raw)
	var stored Capture
	if raw == "" || json.Unmarshal([]byte(raw), &stored) != nil || stored.All {
		return everything(kind)
	}
	return onlyKind(kind, stored)
}

// EncodeSelection stores either every part or the listed part keys.
func EncodeSelection(kind string, all bool, selected []string) (string, error) {
	if !ValidKind(kind) {
		return "", errors.New("choose a webpage, article index, podcast, or text extract")
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
	if raw == "" || raw == `{"all":true}` {
		return EncodeSelection(kind, true, nil)
	}
	var stored Capture
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return "", errors.New("capture selection is not valid")
	}
	if stored.All {
		return `{"all":true}`, nil
	}
	var keys []string
	for _, key := range partKeys(kind) {
		if partOn(stored, key) {
			keys = append(keys, key)
		}
	}
	return EncodeSelection(kind, false, keys)
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
