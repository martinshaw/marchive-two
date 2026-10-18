import { Fragment, useRef, useState } from "react";

function storedSizes(key, fallback) {
  try {
    const raw = JSON.parse(localStorage.getItem(key) || "");
    if (Array.isArray(raw) && raw.length === fallback.length && raw.every((size) => Number.isFinite(size) && size > 0)) return raw;
  } catch { /* Keep the built-in sizes. */ }
  return fallback;
}

function clampPair(sizes, index, nextA, span, min) {
  const total = sizes[index] + sizes[index + 1];
  const minPct = Math.min(total / 2, (min / Math.max(span, 1)) * 100);
  let a = nextA;
  let b = total - a;
  if (a < minPct) { a = minPct; b = total - a; }
  if (b < minPct) { b = minPct; a = total - b; }
  if (a < minPct || b < minPct) return sizes;
  const next = sizes.slice();
  next[index] = a;
  next[index + 1] = b;
  return next;
}

export function Split({ storageKey, direction = "column", sizes: initial, min = 88, children }) {
  const panes = Array.isArray(children) ? children.filter(Boolean) : [children];
  const [sizes, setSizes] = useState(() => storedSizes(storageKey, initial));
  const [active, setActive] = useState(-1);
  const box = useRef(null);
  const sizesRef = useRef(sizes);
  const drag = useRef(null);
  sizesRef.current = sizes.length === panes.length ? sizes : initial;
  const live = sizes.length === panes.length ? sizes : initial;

  function remember(next) {
    sizesRef.current = next;
    setSizes(next);
    try { localStorage.setItem(storageKey, JSON.stringify(next)); } catch { /* The layout still updates. */ }
  }

  function span() {
    const rect = box.current?.getBoundingClientRect();
    return direction === "column" ? rect?.height || 1 : rect?.width || 1;
  }

  function move(event) {
    const current = drag.current;
    if (!current || event.pointerId !== current.pointer) return;
    const pos = direction === "column" ? event.clientY : event.clientX;
    const delta = ((pos - current.start) / span()) * 100;
    const next = clampPair(current.sizes, current.index, current.sizes[current.index] + delta, span(), min);
    sizesRef.current = next;
    setSizes(next);
  }

  function end(event) {
    const current = drag.current;
    if (!current || event.pointerId !== current.pointer) return;
    drag.current = null;
    setActive(-1);
    document.body.classList.remove("is-row-resize", "is-col-resize");
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", end);
    window.removeEventListener("pointercancel", end);
    remember(sizesRef.current);
  }

  function start(index, event) {
    if (event.button !== 0) return;
    event.preventDefault();
    drag.current = {
      index,
      pointer: event.pointerId,
      start: direction === "column" ? event.clientY : event.clientX,
      sizes: sizesRef.current.slice(),
    };
    setActive(index);
    document.body.classList.add(direction === "column" ? "is-row-resize" : "is-col-resize");
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", end);
    window.addEventListener("pointercancel", end);
    try { event.currentTarget.setPointerCapture(event.pointerId); } catch { /* The window listeners still follow the pointer. */ }
  }

  function nudge(index, amount) {
    remember(clampPair(sizesRef.current, index, sizesRef.current[index] + amount, span(), min));
  }

  return (
    <div ref={box} className={direction === "column" ? "split is-column" : "split is-row"}>
      {panes.map((pane, index) => (
        <Fragment key={index}>
          {index > 0 ? (
            <div
              className={active === index - 1 ? "split-gutter is-on" : "split-gutter"}
              role="separator"
              tabIndex={0}
              aria-orientation={direction === "column" ? "horizontal" : "vertical"}
              aria-label="Resize"
              onPointerDown={(event) => start(index - 1, event)}
              onDoubleClick={() => remember(initial)}
              onKeyDown={(event) => {
                const step = event.shiftKey ? 8 : 3;
                if (direction === "column" && event.key === "ArrowDown") { event.preventDefault(); nudge(index - 1, step); }
                if (direction === "column" && event.key === "ArrowUp") { event.preventDefault(); nudge(index - 1, -step); }
                if (direction === "row" && event.key === "ArrowRight") { event.preventDefault(); nudge(index - 1, step); }
                if (direction === "row" && event.key === "ArrowLeft") { event.preventDefault(); nudge(index - 1, -step); }
              }}
            />
          ) : null}
          <div className="split-pane" style={{ flex: `${live[index]} 1 0px` }}>{pane}</div>
        </Fragment>
      ))}
    </div>
  );
}
