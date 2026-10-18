const TIME_TAIL = /[·•]\s*(?:\d+\s*[smhdw]|[A-Za-z]{3,9}\s+\d{1,2})\s*$/;
const TOKEN = /(https?:\/\/[^\s<]+)|(@[A-Za-z0-9_]{1,15})|(#[\p{L}\p{N}_]+)/gu;

export function handleFromURL(raw) {
  try {
    const url = new URL(raw);
    const host = url.hostname.replace(/^www\./, "");
    if (!["x.com", "twitter.com", "mobile.x.com", "mobile.twitter.com"].includes(host)) return "";
    const [user, kind] = url.pathname.split("/").filter(Boolean);
    if (kind === "status" && /^[A-Za-z0-9_]{1,15}$/.test(user || "")) return `@${user}`;
  } catch {
    return "";
  }
  return "";
}

export function tweetIdentity({ title = "", text = "", url = "", authors = [], handle = "" } = {}) {
  let raw = String(authors?.[0] || "").trim();
  const fromURL = handleFromURL(url);
  if (!raw) {
    const head = String(title).split(": ")[0].trim();
    if (fromURL && head.toLowerCase().includes(fromURL.toLowerCase())) raw = head;
    else raw = authorFromTitle(title, text);
  }
  raw = raw.replace(TIME_TAIL, "").trim();
  let at = String(handle || "").trim();
  if (at && !at.startsWith("@")) at = `@${at}`;
  if (!at) at = fromURL;
  let name = raw;
  if (at) {
    const idx = name.toLowerCase().lastIndexOf(at.toLowerCase());
    if (idx > 0) name = name.slice(0, idx).trim();
    else if (idx === 0) name = "";
  }
  if (!name) name = at.replace(/^@/, "") || "Post";
  return { name, handle: at };
}

function authorFromTitle(title, text) {
  const idx = String(title).indexOf(": ");
  if (idx <= 0) return "";
  const name = title.slice(0, idx).trim();
  let rest = title.slice(idx + 2).trim().replace(/(?:…|\.\.\.)$/, "").trim();
  rest = rest.replace(/\s+/g, " ");
  const body = String(text || "").replace(/\s+/g, " ").trim();
  if (rest && body.startsWith(rest)) return name;
  return "";
}

export function tweetParts(text) {
  const source = String(text || "");
  const parts = [];
  let last = 0;
  for (const match of source.matchAll(TOKEN)) {
    const index = match.index ?? 0;
    if (index > last) parts.push({ type: "text", value: source.slice(last, index) });
    let value = match[0];
    const type = match[1] ? "link" : match[2] ? "mention" : "tag";
    let extra = "";
    if (type === "link") {
      const clean = value.replace(/[),.;!?]+$/u, "");
      extra = value.slice(clean.length);
      value = clean;
    }
    if (value) parts.push({ type, value });
    if (extra) parts.push({ type: "text", value: extra });
    last = index + match[0].length;
  }
  if (last < source.length) parts.push({ type: "text", value: source.slice(last) });
  return parts;
}

export function tweetMedia(files) {
  return (files || []).filter((file) => /^(image|video|audio)-\d+\./i.test(file.name));
}

export function tweetStamp(iso) {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const time = date.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  const day = date.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  return `${time} · ${day}`;
}

const AVATAR_COLORS = ["#1d9bf0", "#7856ff", "#f91880", "#00ba7c", "#ff7a00"];

export function tweetAvatar(seed) {
  let n = 0;
  for (const ch of String(seed || "x")) n = (n + ch.charCodeAt(0)) % AVATAR_COLORS.length;
  const source = String(seed || "X").replace(/^@/, "");
  const letter = [...source].find((ch) => /\p{L}|\p{N}/u.test(ch)) || "X";
  return { color: AVATAR_COLORS[n], letter: letter.toUpperCase() };
}
