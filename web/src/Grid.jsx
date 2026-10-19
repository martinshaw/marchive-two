import { useEffect, useLayoutEffect, useRef, useState } from "react";

function safeDragName(name) {
  const clean = String(name || "Archive").replace(/[\\/:*?"<>|]/g, " ").replace(/\s+/g, " ").trim();
  return (clean || "Archive").slice(0, 80);
}

function fileURL(path) {
  const slash = path.replaceAll("\\", "/");
  const abs = slash.startsWith("/") ? slash : `/${slash}`;
  return `file://${abs.split("/").map((part, index) => (index === 0 ? "" : encodeURIComponent(part))).join("/")}`;
}

function startFinderDrag(event, row) {
  const path = row.dragPath;
  if (!path) return;
  const platform = navigator.platform || "";
  const ext = platform.startsWith("Win") || path.includes("\\") ? ".url" : (platform.startsWith("Linux") ? ".desktop" : ".inetloc");
  const file = `${safeDragName(row.dragName)}${ext}`;
  const endpoint = new URL("/api/finder", window.location.origin);
  endpoint.searchParams.set("path", path);
  event.dataTransfer.effectAllowed = "copy";
  event.dataTransfer.setData("text/plain", path);
  event.dataTransfer.setData("text/uri-list", fileURL(path));
  event.dataTransfer.setData("DownloadURL", `application/octet-stream:${file}:${endpoint}`);
  const ghost = document.createElement("div");
  ghost.className = "drag-proxy";
  ghost.textContent = row.dragName || file;
  document.body.appendChild(ghost);
  event.dataTransfer.setDragImage(ghost, 12, 12);
  event.currentTarget.addEventListener("dragend", () => ghost.remove(), { once: true });
}

function sortValue(sort, text) {
  const raw = String(sort != null && sort !== "" ? sort : text || "").trim();
  if (!raw) return { empty: true, num: null, text: "" };
  if (/^-?\d+(\.\d+)?$/.test(raw)) return { empty: false, num: Number(raw), text: raw };
  return { empty: false, num: null, text: raw.toLowerCase() };
}

function compareRows(a, b, spec) {
  const av = sortValue(a.cells[spec.index]?.sort, a.cells[spec.index]?.sortText);
  const bv = sortValue(b.cells[spec.index]?.sort, b.cells[spec.index]?.sortText);
  if (av.empty !== bv.empty) return av.empty ? 1 : -1;
  let cmp = 0;
  if (av.num != null && bv.num != null) cmp = av.num - bv.num;
  else cmp = av.text.localeCompare(bv.text, undefined, { numeric: true, sensitivity: "base" });
  if (!cmp) cmp = String(a.key).localeCompare(String(b.key), undefined, { numeric: true });
  return cmp * spec.dir;
}

export function Grid({ columns, rows, visible, empty, onActivate, onOrder }) {
  const tableRef = useRef(null);
  const [widths, setWidths] = useState(null);
  const [sort, setSort] = useState(null);
  const drag = useRef(null);

  const ordered = sort ? [...rows].sort((a, b) => compareRows(a, b, sort)) : rows;
  const orderedKeys = ordered.filter((row) => row.visible).map((row) => row.key);

  useEffect(() => {
    onOrder?.(orderedKeys);
  });

  useLayoutEffect(() => {
    if (!visible || widths) return;
    const table = tableRef.current;
    const parent = table?.closest(".table-scroll");
    if (!table || !parent || parent.clientWidth < 80) return;
    const headers = table.querySelectorAll("thead th");
    if (!headers.length) return;
    const next = [];
    let sum = 0;
    let grow = headers.length - 1;
    headers.forEach((th, index) => {
      const width = Math.max(56, Math.round(th.getBoundingClientRect().width));
      next.push(width);
      sum += width;
      if (th.classList.contains("col-name")) grow = index;
    });
    const extra = parent.clientWidth - sum;
    if (extra > 1) next[grow] += extra;
    setWidths(next);
  }, [visible, widths, columns.length, rows.length]);

  useEffect(() => {
    function move(event) {
      const state = drag.current;
      if (!state) return;
      const dx = event.clientX - state.startX;
      if (Math.abs(dx) > 2) state.moved = true;
      const next = state.start.slice();
      let width = Math.max(56, state.start[state.index] + dx);
      const delta = width - state.start[state.index];
      next[state.index] = width;
      if (state.index + 1 < next.length) {
        const neighbor = state.start[state.index + 1] - delta;
        if (neighbor < 56) {
          next[state.index] = state.start[state.index] + (state.start[state.index + 1] - 56);
          next[state.index + 1] = 56;
        } else next[state.index + 1] = neighbor;
      }
      setWidths(next.map((n) => Math.max(56, Math.round(n))));
    }
    function up() {
      if (drag.current?.moved) drag.current.suppress = true;
      drag.current = drag.current ? { ...drag.current, active: false } : null;
      document.body.classList.remove("is-resizing");
    }
    document.addEventListener("pointermove", move);
    document.addEventListener("pointerup", up);
    return () => {
      document.removeEventListener("pointermove", move);
      document.removeEventListener("pointerup", up);
    };
  }, []);

  function toggleSort(index) {
    setSort((spec) => {
      if (spec && spec.index === index) return { index, dir: -spec.dir };
      return { index, dir: 1 };
    });
  }

  function startResize(event, index) {
    event.preventDefault();
    const headers = tableRef.current.querySelectorAll("thead th");
    const start = Array.from(headers).map((cell) => cell.getBoundingClientRect().width);
    drag.current = { index, startX: event.clientX, start, moved: false, active: true, suppress: false };
    document.body.classList.add("is-resizing");
  }

  const sum = widths ? widths.reduce((total, width) => total + width, 0) : 0;

  return (
    <>
      <table className="grid" ref={tableRef} aria-multiselectable="true" style={widths ? { width: `${sum}px` } : undefined}>
        {widths && (
          <colgroup>
            {widths.map((width, index) => <col key={columns[index].label} style={{ width: `${width}px` }} />)}
          </colgroup>
        )}
        <thead>
          <tr>
            {columns.map((column, index) => {
              const on = sort && sort.index === index;
              return (
                <th
                  key={column.label}
                  className={column.className || ""}
                  aria-sort={on ? (sort.dir < 0 ? "descending" : "ascending") : "none"}
                >
                  <button
                    type="button"
                    className="col-sort"
                    onClick={(event) => {
                      if (drag.current?.suppress) {
                        drag.current.suppress = false;
                        event.preventDefault();
                        return;
                      }
                      toggleSort(index);
                    }}
                  >
                    {column.label}
                    <span className="col-mark" aria-hidden="true" />
                  </button>
                  <span
                    className="col-resize"
                    role="separator"
                    aria-orientation="vertical"
                    aria-label={`Resize ${column.label}`}
                    onPointerDown={(event) => startResize(event, index)}
                  />
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {ordered.map((row) => (
            <tr
              key={row.key}
              data-key={row.key}
              hidden={!row.visible}
              aria-selected={row.selected ? "true" : "false"}
              className={row.selected ? "is-selected" : undefined}
              onMouseDown={(event) => {
                if (event.shiftKey || event.metaKey) event.preventDefault();
              }}
              onClick={(event) => {
                if (!row.visible) return;
                if (event.ctrlKey && !event.metaKey) return;
                if (event.shiftKey || event.metaKey) event.preventDefault();
                onActivate?.(row, false, event, orderedKeys);
              }}
              onDoubleClick={() => { if (row.visible) onActivate?.(row, true, null, orderedKeys); }}
              draggable={row.dragPath ? "true" : undefined}
              onDragStart={(event) => startFinderDrag(event, row)}
            >
              {row.cells.map((cell, index) => (
                <td key={columns[index].label} className={cell.className || undefined}>{cell.node}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      <p className="empty" hidden={!empty}>{empty}</p>
    </>
  );
}
