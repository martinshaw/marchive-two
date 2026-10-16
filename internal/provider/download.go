package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/version"
)

const maxMediaBytes int64 = 1 << 30

type statusError struct {
	code int
	url  string
}

func (e statusError) Error() string {
	return fmt.Sprintf("HTTP %d for %s", e.code, e.url)
}

func (e statusError) StatusCode() int { return e.code }

func getBytes(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Marchive/"+version.Version)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, statusError{code: resp.StatusCode, url: rawURL}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response is larger than %d bytes", limit)
	}
	return body, nil
}

func downloadFile(ctx context.Context, client *http.Client, rawURL, dest string) error {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Marchive/"+version.Version)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError{code: resp.StatusCode, url: rawURL}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxMediaBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if n > maxMediaBytes {
		os.Remove(tmp)
		return fmt.Errorf("file is larger than %d bytes", maxMediaBytes)
	}
	return os.Rename(tmp, dest)
}

func extensionFrom(rawURL, contentType string) string {
	u, err := parseURL(rawURL)
	if err == nil {
		ext := strings.ToLower(path.Ext(u.Path))
		if len(ext) > 1 && len(ext) <= 5 {
			return ext
		}
	}
	switch {
	case strings.Contains(contentType, "mpeg"), strings.Contains(contentType, "mp3"):
		return ".mp3"
	case strings.Contains(contentType, "mp4"):
		return ".mp4"
	case strings.Contains(contentType, "ogg"):
		return ".ogg"
	default:
		return ".bin"
	}
}
