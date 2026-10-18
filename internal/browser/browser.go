// Package browser drives the Chrome already installed on this Mac.
package browser

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Options selects which artifacts a fetch keeps.
type Options struct {
	Screenshot bool
	MHTML      bool
	Scroll     bool
	// Headed opens a visible window. X answers HTTP 403 to the headless window.
	Headed bool
}

// Document is the rendered page Chrome returned.
type Document struct {
	URL        string
	HTML       string
	Screenshot []byte
	MHTML      string
}

// Fetcher loads a page. Tests can substitute their own implementation.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string, opt Options) (*Document, error)
}

// Client launches Chrome with a persistent profile, one capture at a time.
type Client struct {
	profile   string
	install   *Installer
	mu        sync.Mutex
	busy      bool
	signingIn bool
}

func New(profileDir string) *Client {
	return &Client{profile: profileDir, install: NewInstaller()}
}

// Status reports whether a capture or a sign-in window is using Chrome.
func (c *Client) Status() (busy, signingIn bool) {
	if c == nil {
		return false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busy, c.signingIn
}

// BeginSignIn opens a visible Chrome window on Marchive's profile and returns
// without waiting. The window stays until the user quits Chrome.
func (c *Client) BeginSignIn() error {
	c.mu.Lock()
	if c.busy || c.signingIn {
		c.mu.Unlock()
		return errors.New("Chrome is already in use")
	}
	bin, err := Find()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	if err := os.MkdirAll(c.profile, 0o755); err != nil {
		c.mu.Unlock()
		return err
	}
	if err := prepareProfile(c.profile); err != nil {
		c.mu.Unlock()
		return err
	}
	c.signingIn = true
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			c.signingIn = false
			c.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		cmd := startVisibleChrome(ctx, bin, c.profile)
		_ = cmd.Run()
	}()
	return nil
}

// Fetch renders rawURL in headless Chrome.
func (c *Client) Fetch(ctx context.Context, rawURL string, opt Options) (*Document, error) {
	if c == nil {
		return nil, errors.New("chrome is not configured")
	}
	c.mu.Lock()
	if c.signingIn || c.busy {
		c.mu.Unlock()
		return nil, errors.New("Chrome is already in use")
	}
	bin, err := Find()
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if err := os.MkdirAll(c.profile, 0o755); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.busy = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.busy = false
		c.mu.Unlock()
	}()

	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	allocCtx, allocCancel, err := c.allocator(fetchCtx, bin, opt.Headed)
	if err != nil {
		return nil, err
	}
	defer allocCancel()
	taskCtx, taskCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(string, ...any) {}))
	defer taskCancel()

	var html, finalURL string
	load := &pageLoad{}
	actions := []chromedp.Action{
		network.Enable(),
		network.SetBlockedURLs(blockedURLPatterns),
		chromedp.ActionFunc(func(ctx context.Context) error {
			load.listen(ctx)
			return nil
		}),
		chromedp.Navigate(rawURL),
		chromedp.Evaluate(dismissCookieJS, nil),
		chromedp.ActionFunc(func(ctx context.Context) error { return load.wait(ctx) }),
	}
	if opt.Scroll {
		actions = append(actions,
			chromedp.Evaluate(scrollJS, nil),
			chromedp.ActionFunc(func(ctx context.Context) error { return load.wait(ctx) }),
			chromedp.Evaluate(scrollJS, nil),
			chromedp.ActionFunc(func(ctx context.Context) error { return load.wait(ctx) }),
			chromedp.Evaluate(`window.scrollTo(0, 0)`, nil),
		)
	}
	actions = append(actions,
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	)
	doc := &Document{}
	if opt.Screenshot {
		actions = append(actions, chromedp.FullScreenshot(&doc.Screenshot, 80))
	}
	if err := chromedp.Run(taskCtx, actions...); err != nil {
		return nil, err
	}
	doc.URL = finalURL
	doc.HTML = html
	if opt.MHTML {
		if err := chromedp.Run(taskCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			archived, err := freezePage(ctx, finalURL)
			if err != nil || !strings.Contains(archived, "multipart/related") {
				data, snapErr := page.CaptureSnapshot().WithFormat(page.CaptureSnapshotFormatMhtml).Do(ctx)
				if snapErr != nil {
					if err != nil {
						return err
					}
					return snapErr
				}
				archived = data
			}
			doc.MHTML = archived
			return nil
		})); err != nil {
			return nil, err
		}
	}
	if doc.HTML == "" {
		return nil, errors.New("chrome returned an empty page")
	}
	return doc, nil
}

// Find returns the Chrome or Chromium executable.
func Find() (string, error) {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		filepath.Join(home, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("Google Chrome is not installed. Webpage and article captures need Chrome. Podcast captures do not.")
}
