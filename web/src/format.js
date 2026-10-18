const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

export function stamp(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");
  return `${date.getDate()} ${MONTHS[date.getMonth()]} ${date.getFullYear()} ${hours}:${minutes}`;
}

export function unix(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return String(Math.floor(date.getTime() / 1000));
}

export function shortWhen(iso) {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString(undefined, { day: "numeric", month: "short" });
}

export function everyParts(seconds) {
  if (seconds === 0) return { count: 1, unit: "once" };
  if (seconds % 3600 === 0 && seconds >= 3600) return { count: seconds / 3600, unit: "hours" };
  if (seconds % 60 === 0 && seconds >= 60) return { count: seconds / 60, unit: "minutes" };
  if (!seconds) return { count: 1, unit: "seconds" };
  return { count: seconds, unit: "seconds" };
}

export function everyLabel(seconds) {
  const { count, unit } = everyParts(seconds);
  if (unit === "once") return "Once";
  const name = count === 1 && unit.length > 1 ? unit.slice(0, -1) : unit;
  return `${count} ${name}`;
}

export function intervalSeconds(count, unit) {
  if (unit === "once") return 0;
  const n = Number(count);
  const mult = unit === "hours" ? 3600 : unit === "minutes" ? 60 : 1;
  return n * mult;
}

export function providerLabel(kind) {
  if (kind === "webpage") return "Webpage";
  if (kind === "article_index") return "Article index";
  if (kind === "podcast") return "Podcast";
  if (kind === "text_extract") return "Text extract";
  if (kind === "x_trending") return "X trending";
  if (kind === "hn_entry") return "Hacker News entry";
  if (kind === "google_search") return "Google search";
  if (kind === "ddg_search") return "DuckDuckGo";
  return kind || "";
}

export function itemKindLabel(kind) {
  if (kind === "snapshot") return "Webpage";
  if (kind === "article") return "Article";
  if (kind === "episode") return "Episode";
  if (kind === "text") return "Text";
  if (kind === "tweet") return "Tweet";
  if (kind === "entry") return "Hacker News";
  if (kind === "result") return "Result";
  return kind || "";
}

export function fileLabel(file) {
  const names = {
    "text.txt": "Text",
    "article.html": "Article",
    "page.html": "Page",
    "screenshot.jpg": "Screenshot",
    "metadata.json": "Details",
    "words.json": "Word index",
    "comments.txt": "Comments",
    "site.txt": "Linked site",
    "snapshot.mhtml": "Archive",
  };
  if (names[file.name]) return names[file.name];
  if (file.kind === "audio") return "Audio";
  if (file.kind === "video") return "Video";
  if (file.kind === "image") return "Image";
  return file.name;
}

export function itemMeter(status) {
  if (status === "queued") return { status, percent: 0, busy: false };
  if (status === "running") return { status, percent: 0, busy: true };
  return { status, percent: 100, busy: false };
}

export function runMeter(status, items) {
  if (status === "queued") return { status, percent: 0, busy: false };
  if (status === "running") {
    const list = items || [];
    const done = list.filter((item) => item.status !== "queued" && item.status !== "running").length;
    if (!list.length || done === 0) return { status, percent: 0, busy: true };
    return { status, percent: Math.floor((done * 100) / list.length), busy: false };
  }
  return { status, percent: 100, busy: false };
}

const CAPTURE_PARTS = [
  { key: "html", kinds: ["webpage"], label: "Page HTML" },
  { key: "html", kinds: ["article_index"], label: "Article HTML" },
  { key: "text", kinds: ["webpage", "article_index", "text_extract"], label: "Readable text" },
  { key: "text", kinds: ["x_trending"], label: "Tweet text" },
  { key: "text", kinds: ["hn_entry"], label: "Story text" },
  { key: "text", kinds: ["google_search", "ddg_search"], label: "Result text" },
  { key: "comments", kinds: ["hn_entry"], label: "Comments" },
  { key: "screenshot", kinds: ["webpage", "article_index"], label: "Screenshot" },
  { key: "screenshot", kinds: ["hn_entry"], label: "Linked site screenshot" },
  { key: "mhtml", kinds: ["webpage", "article_index"], label: "Page archive" },
  { key: "audio", kinds: ["podcast"], label: "Episode audio" },
  { key: "media", kinds: ["x_trending"], label: "Images, audio, and video" },
  { key: "words", kinds: ["webpage", "article_index"], label: "Word index" },
  { key: "words", kinds: ["x_trending"], label: "Word list" },
  { key: "words", kinds: ["hn_entry"], label: "Word index" },
  { key: "words", kinds: ["google_search", "ddg_search"], label: "Word index" },
  { key: "metadata", kinds: ["webpage", "article_index", "text_extract"], label: "Metadata" },
  { key: "metadata", kinds: ["x_trending"], label: "Author, hashtags, and address" },
  { key: "metadata", kinds: ["hn_entry"], label: "Title, author, score, and address" },
  { key: "metadata", kinds: ["google_search", "ddg_search"], label: "Title, snippet, and address" },
  { key: "metadata", kinds: ["podcast"], label: "Episode details" },
];

export function captureParts(kind) {
  return CAPTURE_PARTS.filter((part) => part.kinds.includes(kind));
}

export function parseCapture(raw) {
  if (!raw) return { all: true };
  try {
    const stored = JSON.parse(raw);
    if (!stored || stored.all) return { all: true };
    return stored;
  } catch {
    return { all: true };
  }
}

export function encodeCapture(all, keys) {
  if (all) return '{"all":true}';
  const chosen = {};
  keys.forEach((key) => { chosen[key] = true; });
  return JSON.stringify(chosen);
}

export function matchesQuery(query, text) {
  const q = (query || "").trim().toLowerCase();
  if (!q) return true;
  return text.toLowerCase().includes(q);
}
