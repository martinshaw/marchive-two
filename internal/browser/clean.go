package browser

// blockedURLPatterns stops common ad and tracker hosts from loading.
// Patterns use Chrome's Network.setBlockedURLs wildcards.
var blockedURLPatterns = []string{
	"*://*.doubleclick.net/*",
	"*://*.googlesyndication.com/*",
	"*://*.google-analytics.com/*",
	"*://*.googletagmanager.com/*",
	"*://*.googleadservices.com/*",
	"*://*.adservice.google.com/*",
	"*://*.facebook.net/*",
	"*://*.scorecardresearch.com/*",
	"*://*.amazon-adsystem.com/*",
	"*://*.adnxs.com/*",
	"*://*.outbrain.com/*",
	"*://*.taboola.com/*",
	"*://*.moatads.com/*",
	"*://*.quantserve.com/*",
}

// dismissCookieJS clicks an accept button inside a cookie or consent dialog.
const dismissCookieJS = `(function () {
  function clickMatch(buttons, pattern) {
    for (var i = 0; i < buttons.length; i++) {
      var text = (buttons[i].innerText || buttons[i].getAttribute("aria-label") || "").replace(/\s+/g, " ").trim();
      if (!text || text.length > 40) continue;
      if (pattern.test(text)) {
        buttons[i].click();
        return true;
      }
    }
    return false;
  }
  var root = document.querySelector('[id*="cookie" i], [class*="cookie" i], [id*="consent" i], [class*="consent" i], #onetrust-banner-sdk');
  if (root && clickMatch(root.querySelectorAll('button, [role="button"]'), /^(accept|agree|allow all|accept all|i agree|got it|ok|consent)$/i)) return;
  clickMatch(document.querySelectorAll('button, [role="button"]'), /^(accept all|i agree|agree|allow all)$/i);
})()`

const scrollJS = `(function () {
  var height = Math.max(document.body ? document.body.scrollHeight : 0, document.documentElement ? document.documentElement.scrollHeight : 0);
  window.scrollTo(0, height);
  return height;
})()`
