const REVEAL = {
  sources: ".content > .pane.is-on .table-scroll",
  runs: ".runs-scroll",
  items: ".items-scroll",
  reader: ".reader-scroll",
};

export function sameIds(a, b) {
  return a.length === b.length && a.every((id, index) => id === b[index]);
}

export function rangeIds(ordered, from, to) {
  const start = ordered.indexOf(from);
  const end = ordered.indexOf(to);
  if (start < 0 || end < 0) return null;
  const lo = Math.min(start, end);
  const hi = Math.max(start, end);
  return ordered.slice(lo, hi + 1);
}

export function chooseRows(current, ordered, id, event, anchor) {
  const toggle = !!event?.metaKey;
  if (event?.shiftKey) {
    const range = rangeIds(ordered, anchor == null ? id : anchor, id);
    if (range) return toggle ? [...new Set([...current, ...range])] : range;
  }
  if (toggle) return current.includes(id) ? current.filter((item) => item !== id) : [...current, id];
  return [id];
}

export function reveal(kind, id) {
  const root = REVEAL[kind];
  if (!root) return;
  requestAnimationFrame(() => {
    const node = document.querySelector(`${root} [data-key="${CSS.escape(String(id))}"]`);
    node?.scrollIntoView({ block: "nearest" });
  });
}

export function createSelection({
  view, marks, scope, orderRef, picked, shownReader, readerItems,
  setScheduleIds, setRunIds, setItemIds, setReaderIds,
}) {
  function applyIds(kind, ids) {
    if (kind === "runs") {
      const current = picked.current.runs;
      const same = ids.length === 1 && current.length === 1 && ids[0] === current[0];
      picked.current.runs = ids;
      if (!same) {
        picked.current.items = [];
        setItemIds([]);
        marks.current.items = { anchor: null, lead: null };
      }
      setRunIds(ids);
      return;
    }
    picked.current[kind] = ids;
    if (kind === "sources") setScheduleIds(ids);
    else if (kind === "items") setItemIds(ids);
    else if (kind === "reader") {
      setReaderIds(ids);
      if (ids.length === 1) {
        const row = readerItems.find((item) => item.id === ids[0]);
        if (row) {
          marks.current.runs = { anchor: row.runId, lead: row.runId };
          marks.current.items = { anchor: ids[0], lead: ids[0] };
          picked.current.runs = [row.runId];
          picked.current.items = [ids[0]];
          setRunIds([row.runId]);
          setItemIds([ids[0]]);
        }
      }
    }
  }

  function tableScope() {
    if (view === "sources") return "sources";
    if (view === "reader") return "reader";
    if (view === "captures") return scope.current === "items" && (orderRef.current.items || []).length ? "items" : "runs";
    return "";
  }

  function orderedIds(kind) {
    if (kind === "reader") return shownReader.map((item) => item.id);
    return orderRef.current[kind] || [];
  }

  function take(kind, ordered, id, event) {
    const mark = marks.current[kind];
    const ids = chooseRows(picked.current[kind], ordered, id, event, mark.anchor);
    if (event?.shiftKey && mark.anchor != null && ordered.includes(mark.anchor)) mark.lead = id;
    else {
      mark.anchor = id;
      mark.lead = id;
    }
    scope.current = kind;
    applyIds(kind, ids);
    reveal(kind, id);
  }

  function move(delta, extend) {
    const kind = tableScope();
    const ordered = orderedIds(kind);
    if (!kind || !ordered.length) return;
    const mark = marks.current[kind];
    const current = picked.current[kind];
    let leadIndex = ordered.indexOf(mark.lead);
    if (leadIndex < 0 && current.length) leadIndex = ordered.indexOf(current[current.length - 1]);
    const nextIndex = leadIndex < 0 ? (delta < 0 ? ordered.length - 1 : 0) : Math.max(0, Math.min(ordered.length - 1, leadIndex + delta));
    const id = ordered[nextIndex];
    if (extend) {
      if (mark.anchor == null || !ordered.includes(mark.anchor)) mark.anchor = leadIndex >= 0 ? ordered[leadIndex] : id;
      mark.lead = id;
      applyIds(kind, rangeIds(ordered, mark.anchor, id));
    } else {
      mark.anchor = id;
      mark.lead = id;
      applyIds(kind, [id]);
    }
    scope.current = kind;
    reveal(kind, id);
  }

  function selectAll(kind) {
    const which = kind || tableScope();
    const ordered = orderedIds(which);
    if (!which || !ordered.length) return;
    marks.current[which] = { anchor: ordered[0], lead: ordered[ordered.length - 1] };
    scope.current = which;
    applyIds(which, ordered.slice());
    reveal(which, ordered[ordered.length - 1]);
  }

  function ensureRow(kind, id) {
    if (id == null) return;
    const current = picked.current[kind] || [];
    if (!current.includes(id)) {
      marks.current[kind] = { anchor: id, lead: id };
      applyIds(kind, [id]);
    }
    scope.current = kind;
  }

  return { applyIds, tableScope, orderedIds, take, move, selectAll, ensureRow };
}
