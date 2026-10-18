package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

func ContentHash(text string) string {
	normalized := strings.Join(strings.Fields(text), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func ShortHash(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])[:8]
}

// SafeFileName keeps a readable title and a short identity suffix.
func SafeFileName(title, identity string) string {
	var b strings.Builder
	lastDash := false
	count := 0
	for _, r := range title {
		if count >= 80 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			count++
			continue
		}
		if !lastDash && count > 0 {
			b.WriteByte('-')
			lastDash = true
			count++
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "item"
	}
	return name + "-" + ShortHash(identity)
}

// SourceDirName is an id-prefixed slug of a source name, such as "4-bbc-news".
func SourceDirName(id int64, name string) string {
	slug := fileSlug(name)
	if slug == "" {
		slug = "source"
	}
	return strconv.FormatInt(id, 10) + "-" + slug
}

func fileSlug(name string) string {
	var b strings.Builder
	lastDash := false
	count := 0
	for _, r := range strings.ToLower(name) {
		if count >= 60 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			count++
			continue
		}
		if !lastDash && count > 0 {
			b.WriteByte('-')
			lastDash = true
			count++
		}
	}
	return strings.Trim(b.String(), "-")
}

// NormalizeURL strips a fragment and trailing slash so the same address matches.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errEmptyURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errBadScheme
	}
	if u.Host == "" {
		return "", errEmptyURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Path != "/" {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	u.RawPath = ""
	return u.String(), nil
}

func hostKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

var (
	errEmptyURL  = errorString("enter a URL")
	errBadScheme = errorString("only http and https URLs are supported")
)

type errorString string

func (e errorString) Error() string { return string(e) }
