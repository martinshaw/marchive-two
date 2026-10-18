import { everyLabel, matchesQuery, providerLabel, stamp } from "./format.js";

export function sourceVisible(row, view, filter, query) {
  if (view !== "sources") return false;
  if (filter !== "all" && row.kind !== filter) return false;
  return matchesQuery(query, row.text);
}

export function runVisible(run, view, filter, query) {
  if (view !== "captures") return false;
  if (filter && filter !== "all") {
    if (filter === "active") {
      if (run.status !== "queued" && run.status !== "running") return false;
    } else if (run.status !== filter) return false;
  }
  return matchesQuery(query, run.text);
}

export function itemText(item) {
  const title = item.title || "Untitled";
  return [item.status, title, item.kind, item.url, item.error].filter(Boolean).join(" ");
}

export function scheduleRows(library) {
  const rows = [];
  (library?.sources || []).forEach((source) => {
    (source.schedules || []).forEach((schedule) => {
      const name = source.name || "Untitled";
      const text = [name, source.linkSelector, providerLabel(source.provider), source.url, everyLabel(schedule.intervalSeconds), stamp(schedule.nextRunAt), schedule.maxNew, schedule.downloadLocation, schedule.enabled ? "On" : "Off"].filter(Boolean).join(" ");
      rows.push({ source, schedule, kind: source.provider, name, text });
    });
  });
  return rows;
}

export function captureRuns(library) {
  return (library?.runs || []).map((run) => {
    const name = run.sourceName || `Source ${run.sourceId}`;
    const note = run.error || run.note || "";
    const text = [run.status, name, providerLabel(run.provider), stamp(run.createdAt), note, (run.items || []).length].join(" ");
    return { ...run, name, note, text };
  });
}

export function libraryCounts(schedules, runs) {
  const next = { sources: 0, captures: 0, webpage: 0, article_index: 0, podcast: 0, text_extract: 0, x_trending: 0, hn_entry: 0, google_search: 0, ddg_search: 0, active: 0, failed: 0, succeeded: 0 };
  schedules.forEach((row) => {
    next.sources += 1;
    if (next[row.kind] !== undefined) next[row.kind] += 1;
  });
  runs.forEach((run) => {
    next.captures += 1;
    if (run.status === "queued" || run.status === "running") next.active += 1;
    else if (next[run.status] !== undefined) next[run.status] += 1;
  });
  return next;
}
