package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/martinshaw/marchive-two/internal/version"
)

const maxMediaBytes int64 = 1 << 30

const downloadIdle = 45 * time.Second

// mediaClient is shared so feed and episode downloads reuse connections.
var mediaClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
			return errors.New("redirect left http")
		}
		return nil
	},
}

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
	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, rawURL, nil)
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
	body := &activityReader{r: resp.Body}
	stop := watchIdle(dlCtx, cancel, body, downloadIdle)
	buf := make([]byte, 256<<10)
	n, copyErr := io.CopyBuffer(f, io.LimitReader(body, maxMediaBytes+1), buf)
	stop()
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		if ctx.Err() == nil && dlCtx.Err() != nil {
			return errors.New("download stalled")
		}
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

// activityReader records the last time a read returned bytes.
type activityReader struct {
	r    io.Reader
	mu   sync.Mutex
	last time.Time
}

func (a *activityReader) Read(p []byte) (int, error) {
	n, err := a.r.Read(p)
	if n > 0 {
		a.mu.Lock()
		a.last = time.Now()
		a.mu.Unlock()
	}
	return n, err
}

func (a *activityReader) since() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last.IsZero() {
		return 0
	}
	return time.Since(a.last)
}

func watchIdle(ctx context.Context, cancel context.CancelFunc, body *activityReader, idle time.Duration) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		started := time.Now()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				quiet := body.since()
				if quiet == 0 {
					quiet = time.Since(started)
				}
				if quiet > idle {
					cancel()
					return
				}
			}
		}
	}()
	return func() { close(done) }
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
