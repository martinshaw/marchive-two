import { useCallback, useEffect, useRef, useState } from "react";
import { send } from "./api.js";
import { intervalSeconds } from "./format.js";
import { CaptureFields } from "./Sheets.jsx";
import { Icon } from "./sprite.jsx";
import { YtDlpStep } from "./YtDlpAlert.jsx";

const X_PAGES = [
  { id: "trending", label: "Global Trending", url: "https://x.com/explore/tabs/trending" },
  { id: "explore", label: "Explore", url: "https://x.com/explore" },
  { id: "news", label: "News", url: "https://x.com/explore/tabs/news" },
  { id: "sports", label: "Sports", url: "https://x.com/explore/tabs/sports" },
  { id: "entertainment", label: "Entertainment", url: "https://x.com/explore/tabs/entertainment" },
  { id: "us", label: "United States", url: "https://x.com/explore/tabs/trending" },
  { id: "uk", label: "United Kingdom", url: "https://x.com/explore/tabs/trending" },
  { id: "ca", label: "Canada", url: "https://x.com/explore/tabs/trending" },
  { id: "au", label: "Australia", url: "https://x.com/explore/tabs/trending" },
  { id: "ie", label: "Ireland", url: "https://x.com/explore/tabs/trending" },
  { id: "in", label: "India", url: "https://x.com/explore/tabs/trending" },
  { id: "de", label: "Germany", url: "https://x.com/explore/tabs/trending" },
  { id: "fr", label: "France", url: "https://x.com/explore/tabs/trending" },
  { id: "jp", label: "Japan", url: "https://x.com/explore/tabs/trending" },
  { id: "br", label: "Brazil", url: "https://x.com/explore/tabs/trending" },
  { id: "custom", label: "Custom address", url: "" },
];

const HN_PAGES = [
  { id: "newest", label: "Newest", url: "https://news.ycombinator.com/newest" },
  { id: "front", label: "Front page", url: "https://news.ycombinator.com/news" },
  { id: "best", label: "Best", url: "https://news.ycombinator.com/best" },
  { id: "ask", label: "Ask", url: "https://news.ycombinator.com/ask" },
  { id: "show", label: "Show", url: "https://news.ycombinator.com/show" },
  { id: "jobs", label: "Jobs", url: "https://news.ycombinator.com/jobs" },
  { id: "custom", label: "Custom address", url: "" },
];

function searchAddress(provider, query) {
  const value = query.trim();
  if (!value) return "";
  if (/^https?:\/\//i.test(value)) return value;
  const q = encodeURIComponent(value);
  if (provider === "google_search") return `https://www.google.com/search?q=${q}&hl=en`;
  if (provider === "ddg_search") return `https://duckduckgo.com/?q=${q}`;
  return "";
}

function isSearchKind(provider) {
  return provider === "google_search" || provider === "ddg_search";
}

function pagesFor(provider) {
  if (provider === "x_trending") return X_PAGES;
  if (provider === "hn_entry") return HN_PAGES;
  return null;
}

function pageById(pages, id) {
  return (pages || []).find((page) => page.id === id) || (pages || [])[0] || { id: "", label: "", url: "" };
}

function pageForURL(pages, url) {
  const found = (pages || []).find((page) => page.id !== "custom" && page.url === url.trim());
  return found ? found.id : "custom";
}

function isPresetURL(url) {
  const value = url.trim();
  if (!value) return false;
  return X_PAGES.some((page) => page.url === value) || HN_PAGES.some((page) => page.url === value);
}

const KINDS = [
  { id: "webpage", title: "Webpage", icon: "#i-globe", tone: "tone-web", text: "A page Chrome can open, saved as HTML, text, and a screenshot." },
  { id: "article_index", title: "Articles", icon: "#i-news", tone: "tone-news", text: "A page of links. Each new article is saved on its own." },
  { id: "podcast", title: "Podcast", icon: "#i-podcast", tone: "tone-podcast", text: "An RSS or Atom feed. New episodes are saved with their audio." },
  { id: "text_extract", title: "Text", icon: "#i-text", tone: "tone-text", text: "The readable text and details from a page, without a full snapshot." },
  { id: "x_trending", title: "X / Twitter Trending", icon: "#i-trend", tone: "tone-x", text: "Trending posts on X. Each new post is saved with its text, author, hashtags, words, and any images, audio, or video. Sign in with Chrome first." },
  { id: "hn_entry", title: "Hacker News entry", icon: "#i-hn", tone: "tone-hn", text: "New stories from Hacker News. Each one keeps its comments, details, word index, and a screenshot when the story links to a site." },
  { id: "google_search", title: "Google search", icon: "#i-google", tone: "tone-google", text: "Organic results from a Google search. Each new result keeps its title, address, snippet, details, word index, and screenshots of the results page and the linked page." },
  { id: "ddg_search", title: "DuckDuckGo", icon: "#i-ddg", tone: "tone-ddg", text: "Organic results from a DuckDuckGo search, saved the same way as Google results." },
  { id: "youtube_video", title: "YouTube video", icon: "#i-youtube", tone: "tone-youtube", text: "One YouTube video. Saves the description, subtitles, a video file, an audio file, and comments. Chrome takes the watch-page picture and signed-in cookies." },
];

function blank(folder) {
  return { name: "", url: "", search: "", page: "trending", provider: "", selector: "", every: "1", unit: "hours", maxNew: "20", folder: folder || "", capture: '{"all":true}', enabled: true };
}

function checkFailure(err) {
  const message = err && err.message ? err.message : "";
  if (message === "Failed to fetch" || message === "Load failed" || message === "NetworkError when attempting to fetch resource.") {
    return "Marchive didn't answer. Reload this window and try again.";
  }
  return message || "This address cannot be captured.";
}

export function AddSheet({ open, folder, ytDlpReady, onYtDlpInstalled, onClose, onSubmit }) {
  const [form, setForm] = useState(blank(folder));
  const [step, setStep] = useState("kind");
  const [query, setQuery] = useState("");
  const [toolReady, setToolReady] = useState(!!ytDlpReady);
  const [check, setCheck] = useState({ state: "idle", url: "", provider: "", message: "" });
  const flush = useRef(false);
  const [tick, setTick] = useState(0);
  const markTool = useCallback(() => {
    setToolReady(true);
    if (onYtDlpInstalled) onYtDlpInstalled();
  }, [onYtDlpInstalled]);
  useEffect(() => {
    if (!open) return;
    setForm(blank(folder));
    setStep("kind");
    setToolReady(!!ytDlpReady);
    setQuery("");
    setCheck({ state: "idle", url: "", provider: "", message: "" });
  }, [open, folder]);
  useEffect(() => {
    if (open && ytDlpReady) setToolReady(true);
  }, [open, ytDlpReady]);

  const url = form.url.trim();
  const checkedKey = useRef("");
  const picks = pagesFor(form.provider);
  const searching = isSearchKind(form.provider);
  const pageLabel = picks && form.page !== "custom" ? pageById(picks, form.page).label : "";
  useEffect(() => {
    if (!open || step !== "details") return undefined;
    if (!url || !form.provider) {
      checkedKey.current = "";
      setCheck({ state: "idle", url: "", provider: form.provider, message: "" });
      return undefined;
    }
    const key = `${form.provider}\n${url}\n${pageLabel}`;
    const immediate = flush.current;
    flush.current = false;
    if (!immediate && checkedKey.current === key) return undefined;
    let cancelled = false;
    setCheck({ state: "checking", url, provider: form.provider, message: "Checking this address…" });
    const timer = window.setTimeout(() => {
      send("POST", "/api/sources/check", { url, provider: form.provider }).then((data) => {
        if (cancelled) return;
        checkedKey.current = key;
        const title = (data.title || "").trim();
        setCheck({
          state: "ok",
          url,
          provider: form.provider,
          message: pageLabel ? `Looks right · ${pageLabel}` : title ? `Looks right · ${title}` : "This address looks right.",
        });
      }, (err) => {
        if (cancelled) return;
        checkedKey.current = "";
        setCheck({ state: "bad", url, provider: form.provider, message: checkFailure(err) });
      });
    }, immediate ? 0 : 400);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [open, step, url, form.provider, pageLabel, tick]);

  if (!open) return null;
  function set(key, value) { setForm((current) => ({ ...current, [key]: value })); }
  function setSearch(value) {
    setForm((current) => {
      const trimmed = value.trim();
      const next = { ...current, search: value, url: searchAddress(current.provider, trimmed) };
      if (!current.name.trim() || current.name.trim() === current.search.trim()) {
        next.name = /^https?:\/\//i.test(trimmed) ? current.name : trimmed;
      }
      return next;
    });
  }
  function chooseKind(id) {
    setForm((current) => {
      const next = { ...current, provider: id, capture: '{"all":true}' };
      const pages = pagesFor(id);
      const previous = pagesFor(current.provider);
      const preset = isPresetURL(current.url);
      if (pages) {
        if (!current.url.trim() || preset) {
          const page = pages[0];
          next.url = page.url;
          next.page = page.id;
          const previousLabel = previous ? pageById(previous, current.page).label : "";
          if (!current.name.trim() || current.name.trim() === previousLabel) next.name = page.label;
        } else {
          next.page = pageForURL(pages, current.url);
        }
      } else if (preset) {
        next.url = "";
      }
      if (isSearchKind(id)) {
        const typed = current.search.trim();
        if (typed && !/^https?:\/\//i.test(typed)) next.url = searchAddress(id, typed);
      }
      return next;
    });
  }
  function choosePage(id) {
    setForm((current) => {
      const pages = pagesFor(current.provider) || [];
      const page = pageById(pages, id);
      const previous = pageById(pages, current.page);
      const next = { ...current, page: page.id };
      if (page.id === "custom") return next;
      next.url = page.url;
      if (!current.name.trim() || current.name.trim() === previous.label) next.name = page.label;
      return next;
    });
  }
  const needle = query.trim().toLowerCase();
  const kinds = KINDS.filter((kind) => !needle || `${kind.title} ${kind.text}`.toLowerCase().includes(needle));
  const chosen = KINDS.find((kind) => kind.id === form.provider);
  const seconds = intervalSeconds(form.every, form.unit);
  const once = form.unit === "once";
  const everyOk = once || (Number.isFinite(seconds) && seconds >= 30);
  const addressOk = check.state === "ok" && check.url === url && check.provider === form.provider;
  const youtube = form.provider === "youtube_video";
  const needsTool = youtube && !toolReady;
  const canAdd = addressOk && everyOk && form.folder.trim() && Number(form.maxNew) >= 0 && !needsTool;

  return (
    <form
      className="sheet wizard"
      onSubmit={(event) => {
        event.preventDefault();
        if (step === "kind") {
          if (!form.provider) return;
          setStep(needsTool ? "tool" : "details");
          return;
        }
        if (step === "tool") {
          if (!needsTool) setStep("details");
          return;
        }
        if (!canAdd) return;
        onSubmit({
          name: form.name,
          url: form.url,
          provider: form.provider,
          linkSelector: form.provider === "article_index" ? form.selector : "",
          capture: form.capture,
          intervalSeconds: seconds,
          downloadLocation: form.folder,
          maxNew: Number(form.maxNew),
          enabled: form.enabled,
        });
      }}
    >
      <header className="sheet-head">
        <h2>New Source</h2>
        <div className="wizard-steps">
          <button type="button" className={step === "kind" ? "is-on" : ""} onClick={() => setStep("kind")}>1 Kind</button>
          {needsTool || step === "tool" ? (
            <button type="button" className={step === "tool" ? "is-on" : ""} disabled={!form.provider} onClick={() => form.provider && setStep("tool")}>2 yt-dlp</button>
          ) : null}
          <button type="button" className={step === "details" ? "is-on" : ""} disabled={!form.provider || needsTool} onClick={() => form.provider && !needsTool && setStep("details")}>{needsTool || step === "tool" ? "3 Details" : "2 Details"}</button>
        </div>
      </header>
      {step === "tool" ? (
        <YtDlpStep active={open && step === "tool"} onReady={markTool} />
      ) : step === "kind" ? (
        <div className="sheet-body kind-step">
          <label>Search
            <input value={query} placeholder="Webpage, podcast, articles…" autoFocus autoComplete="off" spellCheck="false" onChange={(event) => setQuery(event.target.value)} />
          </label>
          <div className="kind-pick">
            {kinds.length ? (
              <div className="kind-list" role="listbox" aria-label="Source kind">
                {kinds.map((kind) => (
                  <button
                    type="button"
                    role="option"
                    aria-selected={form.provider === kind.id}
                    key={kind.id}
                    className={form.provider === kind.id ? "kind-row is-on" : "kind-row"}
                    onClick={() => chooseKind(kind.id)}
                    onDoubleClick={() => { chooseKind(kind.id); setStep(2); }}
                  >
                    <Icon href={kind.icon} className={`kind-icon ${kind.tone}`} />
                    <span className="kind-title">{kind.title}</span>
                  </button>
                ))}
              </div>
            ) : <p className="kind-empty">No kinds match.</p>}
            <aside className="kind-detail" aria-live="polite">
              {chosen ? (
                <>
                  <Icon href={chosen.icon} className={`kind-detail-icon ${chosen.tone}`} />
                  <h3>{chosen.title}</h3>
                  <p>{chosen.text}</p>
                </>
              ) : (
                <p className="kind-detail-empty">Choose a kind. What it saves is explained here, and the list can grow in this column.</p>
              )}
            </aside>
          </div>
        </div>
      ) : (
        <div className="sheet-body">
          <div className="detail-grid">
          <div className="detail-main">
          {searching ? (
            <label>Search
              <input
                required
                placeholder="climate news, or a search address"
                value={form.search}
                aria-invalid={check.state === "bad"}
                onChange={(event) => setSearch(event.target.value)}
                onBlur={() => { flush.current = true; setTick((n) => n + 1); }}
              />
              {check.message ? <span className={check.state === "bad" ? "hint is-bad" : "hint"}>{check.message}</span> : <span className="hint">Each check saves new results. A result keeps its title, address, snippet, details, and word index, plus a screenshot of the results page and of the linked page.</span>}
            </label>
          ) : null}
          {picks ? (
            <label>Page
              <select aria-label="Page" value={form.page} onChange={(event) => choosePage(event.target.value)}>
                {picks.map((page) => <option key={page.id} value={page.id}>{page.label}</option>)}
              </select>
              <span className="hint">{form.provider === "hn_entry"
                ? "Each check saves new stories from this list. A story with a link also gets a screenshot of that site. Ask and Show posts keep their text and comments."
                : "News, Sports, and Entertainment list topics. Marchive opens a few of those pages and saves the posts. A country opens the trending page and uses the place saved in the Marchive Chrome profile."}</span>
              {form.page !== "custom" && check.message ? <span className={check.state === "bad" ? "hint is-bad" : "hint"}>{check.message}</span> : null}
            </label>
          ) : null}
          {searching || (picks && form.page !== "custom") ? null : (
          <label>URL
            <input
              required
              placeholder={form.provider === "youtube_video" ? "https://www.youtube.com/watch?v=" : "https://example.com"}
              value={form.url}
              aria-invalid={check.state === "bad"}
              onChange={(event) => set("url", event.target.value)}
              onBlur={() => { flush.current = true; setTick((n) => n + 1); }}
            />
            {check.message ? <span className={check.state === "bad" ? "hint is-bad" : "hint"}>{check.message}</span> : <span className="hint">{form.provider === "youtube_video" ? "A watch, shorts, or youtu.be address. yt-dlp saves the files. Chrome saves the watch page and any cookies from the Marchive profile." : "Checked against the kind you chose."}</span>}
          </label>
          )}
          <label hidden={form.provider !== "article_index"}>Article link selector
            <input value={form.selector} placeholder="main a" onChange={(event) => set("selector", event.target.value)} />
            <span className="hint">CSS selector for links on an index page. Leave empty to guess.</span>
          </label>
          <label>Name <input value={form.name} placeholder="Optional" onChange={(event) => set("name", event.target.value)} /></label>
          <div className="detail-pair">
            <label>Check every
              <span className={once ? "every-field is-once" : "every-field"}>
                <input type="number" min="1" required={!once} disabled={once} aria-label="How many" value={form.every} onChange={(event) => set("every", event.target.value)} />
                <select aria-label="Unit" value={form.unit} onChange={(event) => set("unit", event.target.value)}>
                  <option value="minutes">minutes</option>
                  <option value="hours">hours</option>
                  <option value="seconds">seconds</option>
                  <option value="once">Once</option>
                </select>
              </span>
              <span className={everyOk ? "hint" : "hint is-bad"}>{once ? "Marchive checks once, then turns the schedule off." : everyOk ? "How often Marchive looks for something new." : "At least 30 seconds."}</span>
            </label>
            <label>New items each check
              <input type="number" min="0" max="1000" required value={form.maxNew} onChange={(event) => set("maxNew", event.target.value)} />
              <span className="hint">How many new pages or episodes to save each time. 0 saves every new one.</span>
            </label>
          </div>
          <label>Archive folder <input required value={form.folder} placeholder="'Marchive' in home directory" onChange={(event) => set("folder", event.target.value)} /></label>
          <label className="check"><input type="checkbox" checked={form.enabled} onChange={(event) => set("enabled", event.target.checked)} /> Schedule enabled</label>
          </div>
          <div className="detail-side">
            <CaptureFields kind={form.provider} capture={form.capture} legend="Capture items" onChange={(capture) => set("capture", capture)} />
          </div>
          </div>
        </div>
      )}
      <footer className="sheet-foot">
        {step !== "kind" ? <button type="button" onClick={() => setStep(step === "details" && youtube && !ytDlpReady ? "tool" : "kind")}>Back</button> : null}
        <span className="sheet-spacer" />
        <button type="button" onClick={onClose}>Cancel</button>
        {step === "details"
          ? <button type="submit" className="push" disabled={!canAdd}>Add</button>
          : <button type="submit" className="push" disabled={step === "kind" ? !form.provider : needsTool}>Continue</button>}
      </footer>
    </form>
  );
}
