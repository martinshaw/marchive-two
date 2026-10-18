import { itemKindLabel, matchesQuery, stamp } from "./format.js";

export function readerCatalog(runs) {
  const items = [];
  (runs || []).forEach((run) => {
    (run.items || []).forEach((item) => {
      const at = Date.parse(item.createdAt || run.createdAt);
      items.push({
        id: item.id,
        runId: run.id,
        title: item.title || "Untitled",
        source: run.name || "Untitled",
        kind: item.kind || "",
        when: stamp(item.createdAt || run.createdAt),
        at: Number.isNaN(at) ? 0 : at,
        read: !!item.read,
      });
    });
  });
  return items;
}

export function readerChoices(items) {
  const sources = [];
  const kinds = [];
  const seenSource = new Set();
  const seenKind = new Set();
  items.forEach((item) => {
    if (item.source && !seenSource.has(item.source)) {
      seenSource.add(item.source);
      sources.push(item.source);
    }
    if (item.kind && !seenKind.has(item.kind)) {
      seenKind.add(item.kind);
      kinds.push(item.kind);
    }
  });
  sources.sort((a, b) => a.localeCompare(b));
  kinds.sort((a, b) => itemKindLabel(a).localeCompare(itemKindLabel(b)));
  return { sources, kinds };
}

export function readerView(items, opts) {
  const pending = opts.pending || {};
  const rows = items.map((item) => (Object.prototype.hasOwnProperty.call(pending, item.id) ? { ...item, read: !!pending[item.id] } : item));
  const filtered = rows.filter((item) => {
    if (opts.source && item.source !== opts.source) return false;
    if (opts.kind && item.kind !== opts.kind) return false;
    if (opts.read === "unread" && item.read) return false;
    if (opts.read === "read" && !item.read) return false;
    return matchesQuery(opts.query, `${item.title} ${item.source} ${itemKindLabel(item.kind)}`);
  });
  const sort = opts.sort || "newest";
  filtered.sort((a, b) => {
    if (sort === "oldest") return a.at - b.at || a.id - b.id;
    if (sort === "title") return a.title.localeCompare(b.title) || b.at - a.at || a.id - b.id;
    return b.at - a.at || b.id - a.id;
  });
  const groups = groupRows(filtered, opts.group, sort);
  return { groups, ordered: groups.flatMap((group) => group.items) };
}

function groupRows(rows, group, sort) {
  if (group !== "source") return [{ key: "all", label: "", items: rows }];
  const buckets = new Map();
  const seen = [];
  rows.forEach((item) => {
    const key = item.source || "Untitled";
    if (!buckets.has(key)) {
      buckets.set(key, []);
      seen.push(key);
    }
    buckets.get(key).push(item);
  });
  const keys = sort === "title" ? [...seen].sort((a, b) => a.localeCompare(b)) : seen;
  return keys.map((key) => ({ key, label: key, items: buckets.get(key) }));
}

export function readerFiltered(opts) {
  return !!(opts.source || opts.kind || (opts.read && opts.read !== "all") || (opts.query || "").trim());
}
