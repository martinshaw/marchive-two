//go:build !darwin

package browser

import (
	"context"
	"os/exec"

	"github.com/chromedp/chromedp"
)

func (c *Client) allocator(ctx context.Context, bin string, headed bool) (context.Context, context.CancelFunc, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(bin),
		chromedp.UserDataDir(c.profile),
		chromedp.Flag("mute-audio", true),
		chromedp.WindowSize(1280, 1800),
	)
	if headed {
		opts = append(opts, chromedp.Flag("headless", false))
	} else {
		opts = append(opts, chromedp.Flag("headless", "new"))
	}
	// Keep extensions installed in the Marchive profile, such as an ad blocker.
	opts = append(opts, chromedp.Flag("disable-extensions", false))
	ctx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	return ctx, cancel, nil
}

func releaseProfile(string) {}

func startVisibleChrome(ctx context.Context, bin, profile, page string) *exec.Cmd {
	return exec.CommandContext(ctx, bin,
		"--user-data-dir="+profile,
		"--profile-directory=Default",
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		page,
	)
}
