package browser

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// revealGotoLinks asks Google where each /goto result points and stores that
// address on the anchor. The token in the href is not the page address.
func revealGotoLinks(ctx context.Context) error {
	var hrefs []string
	if err := chromedp.Evaluate(collectGotoJS, &hrefs).Do(ctx); err != nil {
		return err
	}
	if len(hrefs) == 0 {
		return nil
	}
	var pageURL string
	if err := chromedp.Location(&pageURL).Do(ctx); err != nil {
		return err
	}
	cookies, err := network.GetCookies().Do(ctx)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	seen := map[string]bool{}
	var unique []string
	for _, href := range hrefs {
		href = strings.TrimSpace(href)
		if href == "" || seen[href] {
			continue
		}
		seen[href] = true
		unique = append(unique, href)
		if len(unique) >= 40 {
			break
		}
	}
	found := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	for _, href := range unique {
		wg.Add(1)
		go func(href string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			final := followGoto(ctx, client, pageURL, href, cookies)
			if final == "" {
				return
			}
			mu.Lock()
			found[href] = final
			mu.Unlock()
		}(href)
	}
	wg.Wait()
	if len(found) == 0 {
		return nil
	}
	raw, err := json.Marshal(found)
	if err != nil {
		return err
	}
	script := `(function (map) {
	  var links = document.querySelectorAll('a[href*="/goto?url="]');
	  for (var i = 0; i < links.length; i++) {
	    var href = links[i].getAttribute("href");
	    if (map[href]) links[i].setAttribute("data-marchive-url", map[href]);
	  }
	})(` + string(raw) + `)`
	return chromedp.Evaluate(script, nil).Do(ctx)
}

const collectGotoJS = `(function () {
  var links = document.querySelectorAll('a[href*="/goto?url="]');
  var hrefs = [];
  for (var i = 0; i < links.length; i++) {
    var href = links[i].getAttribute("href");
    if (href) hrefs.push(href);
  }
  return hrefs;
})()`

func followGoto(ctx context.Context, client *http.Client, pageURL, href string, cookies []*network.Cookie) string {
	abs := href
	if base, err := url.Parse(pageURL); err == nil {
		if ref, err := url.Parse(href); err == nil {
			abs = base.ResolveReference(ref).String()
		}
	}
	if !strings.Contains(abs, "/goto") {
		return ""
	}
	reqCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, abs, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if pageURL != "" {
		req.Header.Set("Referer", pageURL)
	}
	if header := googleCookieHeader(cookies); header != "" {
		req.Header.Set("Cookie", header)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if loc := redirectLocation(abs, resp); loc != "" {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return loc
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return metaRefresh(abs, string(body))
}

func googleCookieHeader(cookies []*network.Cookie) string {
	var b strings.Builder
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
		if !strings.Contains(domain, "google.") {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(cookie.Name)
		b.WriteByte('=')
		b.WriteString(cookie.Value)
	}
	return b.String()
}

func redirectLocation(base string, resp *http.Response) string {
	if resp == nil || resp.StatusCode < 300 || resp.StatusCode > 399 {
		return ""
	}
	loc := strings.TrimSpace(resp.Header.Get("Location"))
	if loc == "" {
		return ""
	}
	return externalLocation(base, loc)
}

func metaRefresh(base, body string) string {
	lower := strings.ToLower(body)
	i := strings.Index(lower, "http-equiv=\"refresh\"")
	if i < 0 {
		i = strings.Index(lower, "http-equiv='refresh'")
	}
	if i < 0 {
		return ""
	}
	snippet := body[i:]
	if len(snippet) > 400 {
		snippet = snippet[:400]
	}
	j := strings.Index(strings.ToLower(snippet), "url=")
	if j < 0 {
		return ""
	}
	rest := strings.TrimSpace(snippet[j+4:])
	rest = strings.Trim(rest, `"' >`)
	if cut := strings.IndexAny(rest, `"'<>`); cut >= 0 {
		rest = rest[:cut]
	}
	return externalLocation(base, strings.TrimSpace(rest))
}

func externalLocation(base, loc string) string {
	ref, err := url.Parse(loc)
	if err != nil {
		return ""
	}
	if root, err := url.Parse(base); err == nil {
		ref = root.ResolveReference(ref)
	}
	if ref.Scheme != "http" && ref.Scheme != "https" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(ref.Hostname()), "www.")
	if host == "" || host == "google.com" || strings.HasPrefix(host, "google.") || strings.HasSuffix(host, ".google.com") {
		if target := ref.Query().Get("q"); strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return target
		}
		if target := ref.Query().Get("url"); strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return target
		}
		return ""
	}
	return ref.String()
}
