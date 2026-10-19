import { everyLabel, itemMeter, matchesQuery, providerLabel, runMeter, stamp, unix } from "./format.js";
import { itemText } from "./library.js";

export function Pill({ meter }) {
  return (
    <span className={`pill ${meter.status}${meter.busy ? " is-busy" : ""}`} style={{ "--progress": `${meter.percent}%` }} role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={meter.busy ? undefined : meter.percent} aria-label={meter.status}>
      <span className="pill-fill" />
      <span className="pill-label">{meter.status}</span>
    </span>
  );
}

export function sourceGridRows(schedules, view, filter, query, scheduleIds, sourceVisible) {
  return schedules.map((row) => {
    const visible = sourceVisible(row, view, filter, query);
    return {
      key: row.schedule.id,
      visible,
      selected: visible && scheduleIds.includes(row.schedule.id),
      row,
      dragPath: row.schedule.downloadLocation || "",
      dragName: row.name,
      cells: [
        { className: "name", sortText: row.name, node: <><span className="primary">{row.name}</span>{row.source.linkSelector ? <span className="sub">{row.source.linkSelector}</span> : null}</> },
        { sortText: providerLabel(row.kind), node: providerLabel(row.kind) },
        { className: "clip", sortText: row.source.url, node: <a href={row.source.url} draggable={false}>{row.source.url}</a> },
        { sort: String(row.schedule.intervalSeconds), sortText: everyLabel(row.schedule.intervalSeconds), node: everyLabel(row.schedule.intervalSeconds) },
        { sort: unix(row.schedule.nextRunAt), sortText: stamp(row.schedule.nextRunAt), node: stamp(row.schedule.nextRunAt) },
        { className: "num", sort: String(row.schedule.maxNew), sortText: String(row.schedule.maxNew), node: row.schedule.maxNew },
        { className: "clip path", sortText: row.schedule.downloadLocation, node: row.schedule.downloadLocation },
        { sortText: row.schedule.enabled ? "On" : "Off", node: row.schedule.enabled ? "On" : "Off" },
      ],
    };
  });
}

export function runGridRows(runs, view, filter, query, runIds, runVisible, folders) {
  return runs.map((run) => {
    const visible = runVisible(run, view, filter, query);
    return {
      key: run.id,
      visible,
      selected: visible && runIds.includes(run.id),
      run,
      dragPath: folders?.get(run.scheduleId) || "",
      dragName: run.name,
      cells: [
        { sortText: run.status, node: <Pill meter={runMeter(run.status, run.items)} /> },
        { className: "name", sortText: run.name, node: <span className="primary">{run.name}</span> },
        { sortText: providerLabel(run.provider), node: providerLabel(run.provider) },
        { sort: unix(run.createdAt), sortText: stamp(run.createdAt), node: stamp(run.createdAt) },
        { className: "clip", sortText: run.note, node: run.error ? <span className="fault">{run.error}</span> : run.note },
        { className: "num", sort: String((run.items || []).length), sortText: String((run.items || []).length), node: (run.items || []).length },
      ],
    };
  });
}

export function itemGridRows(oneRun, query, itemIds) {
  return (oneRun?.items || []).map((item) => {
    const title = item.title || "Untitled";
    const text = itemText(item);
    const visible = matchesQuery(query, text);
    return {
      key: item.id,
      visible,
      selected: visible && itemIds.includes(item.id),
      item,
      dragPath: item.downloadLocation || "",
      dragName: title,
      cells: [
        { sortText: item.status, node: <Pill meter={itemMeter(item.status)} /> },
        { className: "name", sortText: title, node: <span className="primary">{title}</span> },
        { sortText: item.kind, node: item.kind },
        { className: "clip", sortText: item.url, node: item.url ? <a href={item.url} draggable={false}>{item.url}</a> : null },
        { className: "clip", sortText: item.error, node: item.error ? <span className="fault">{item.error}</span> : null },
      ],
    };
  });
}

export function itemsEmptyText(runIds, oneRun, itemRows) {
  const visibleItems = itemRows.filter((row) => row.visible);
  if (runIds.length > 1) return "Select a single capture for more details.";
  if (!oneRun) return "Select a capture to see its items.";
  if (!(oneRun.items || []).length) return "This capture has no items.";
  if (!visibleItems.length) return "No items match this filter.";
  return "";
}
