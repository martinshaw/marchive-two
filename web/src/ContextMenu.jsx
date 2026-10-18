import { useEffect, useLayoutEffect, useRef, useState } from "react";

export async function writeClipboard(text) {
  const value = text == null ? "" : String(text);
  if (!value) return;
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    const area = document.createElement("textarea");
    area.value = value;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.left = "-1000px";
    document.body.appendChild(area);
    area.select();
    document.execCommand("copy");
    area.remove();
  }
}

export async function readClipboard() {
  try { return await navigator.clipboard.readText(); } catch { return null; }
}

export function editableField(node) {
  const el = node?.closest?.("input, textarea");
  if (!el || el.disabled || el.readOnly) return null;
  const type = (el.getAttribute("type") || "text").toLowerCase();
  if (["button", "checkbox", "color", "file", "radio", "range", "reset", "submit"].includes(type)) return null;
  return el;
}

export function fieldRange(el) {
  const start = el.selectionStart;
  const end = el.selectionEnd;
  if (start == null || end == null) return { start: 0, end: 0, text: "", whole: true };
  return { start, end, text: el.value.slice(start, end), whole: false };
}

export function replaceField(el, start, end, insert) {
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  const next = el.value.slice(0, start) + insert + el.value.slice(end);
  if (setter) setter.call(el, next);
  else el.value = next;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  const caret = start + insert.length;
  if (el.selectionStart != null) el.setSelectionRange(caret, caret);
  el.focus();
}

function lines(values) {
  return values.map((value) => String(value ?? "").trim()).filter(Boolean).join("\n");
}

function rowKey(node) {
  const raw = node?.dataset?.key;
  if (raw == null || raw === "") return null;
  const id = Number(raw);
  return Number.isNaN(id) ? raw : id;
}

export function menuList(parts) {
  const items = [];
  parts.flat().forEach((entry) => {
    if (!entry) return;
    if (entry.sep) {
      if (items.length && !items[items.length - 1].sep) items.push({ sep: true });
      return;
    }
    items.push(entry);
  });
  while (items[0]?.sep) items.shift();
  while (items.at(-1)?.sep) items.pop();
  return items;
}

function editGroup({ copy, paste, selectAll, cut }) {
  return [
    cut ? { label: "Cut", key: "⌘X", disabled: !!cut.disabled, action: cut.action } : null,
    { label: "Copy", key: "⌘C", disabled: !copy, action: copy || (() => {}) },
    { label: "Paste", key: "⌘V", disabled: !paste, action: paste || (() => {}) },
    { sep: true },
    { label: "Select All", key: "⌘A", disabled: !selectAll, action: selectAll || (() => {}) },
  ];
}

function linkItems(href) {
  if (!href) return [];
  return [
    { label: "Open Link", action: () => window.open(href, "_blank", "noopener") },
    { label: "Copy Link", action: () => writeClipboard(href) },
    { sep: true },
  ];
}

export function buildMenu(event, api) {
  const target = event.target;
  if (target.closest?.(".context-menu")) return null;
  const field = editableField(target);
  if (field) {
    const range = fieldRange(field);
    const selected = !range.whole && range.text.length > 0;
    return menuList([
      editGroup({
        cut: {
          disabled: !selected,
          action: () => {
            writeClipboard(range.text);
            replaceField(field, range.start, range.end, "");
          },
        },
        copy: selected ? () => writeClipboard(range.text) : null,
        paste: async () => {
          const text = await readClipboard();
          if (text == null) return;
          if (range.whole) replaceField(field, 0, field.value.length, text);
          else replaceField(field, range.start, range.end, text);
        },
        selectAll: field.value ? () => { field.focus(); field.select(); } : null,
      }),
    ]);
  }

  const href = target.closest?.("a[href]")?.href || "";
  const word = target.closest?.(".cloud-word, .words-legend button");
  if (word) {
    const label = word.textContent.trim();
    return menuList([
      { label: "Copy Word", action: () => writeClipboard(label) },
      { sep: true },
      editGroup({ copy: () => writeClipboard(label), paste: null, selectAll: null }),
    ]);
  }

  const sourceRow = target.closest?.(".pane.is-on tbody tr[data-key]");
  const runRow = target.closest?.(".runs-scroll tbody tr[data-key]");
  const itemRow = target.closest?.(".items-scroll tbody tr[data-key]");
  const readerRow = target.closest?.(".reader-row");
  if (runRow) {
    api.ensureRow("runs", rowKey(runRow));
    const selected = api.liveRuns();
    const note = lines(selected.map((run) => run.error || run.note));
    const names = lines(selected.map((run) => run.name));
    return menuList([
      linkItems(href),
      { label: "Cancel Capture", disabled: !selected.some((run) => run.status === "queued" || run.status === "running"), action: () => api.doAction("cancel") },
      { sep: true },
      { label: "Copy Source", disabled: !names, action: () => writeClipboard(names) },
      { label: "Copy Note", disabled: !note, action: () => writeClipboard(note) },
      { sep: true },
      editGroup({
        copy: names ? () => writeClipboard(api.selectionText("runs")) : null,
        paste: null,
        selectAll: () => api.selectAll("runs"),
      }),
      { sep: true },
      { label: "Remove…", disabled: !selected.length, action: () => api.removeSelection("runs") },
    ]);
  }
  if (itemRow) {
    api.ensureRow("items", rowKey(itemRow));
    return itemMenu(api, href, api.liveItems());
  }
  if (readerRow) {
    api.ensureRow("reader", rowKey(readerRow));
    return itemMenu(api, href, api.liveItems());
  }
  if (sourceRow && api.view === "sources") {
    api.ensureRow("sources", rowKey(sourceRow));
    const selected = api.liveSchedules();
    const urls = lines(selected.map((row) => row.source.url));
    const names = lines(selected.map((row) => row.name));
    return menuList([
      linkItems(href),
      { label: "Run Now", key: "⌃R", disabled: !selected.length, action: () => api.doAction("run") },
      { label: "Edit Schedule…", key: "⌃E", disabled: selected.length !== 1, action: () => api.doAction("edit") },
      { label: "Show in Finder", key: "⌃O", disabled: !selected.some((row) => row.schedule.downloadLocation), action: () => api.doAction("files") },
      { sep: true },
      { label: "Copy URL", disabled: !urls, action: () => writeClipboard(urls) },
      { label: "Copy Name", disabled: !names, action: () => writeClipboard(names) },
      { sep: true },
      editGroup({
        copy: names ? () => writeClipboard(api.selectionText("sources")) : null,
        paste: null,
        selectAll: () => api.selectAll("sources"),
      }),
      { sep: true },
      { label: "Remove…", disabled: !selected.length, action: () => api.doAction("remove") },
    ]);
  }

  const article = target.closest?.("article.message");
  if (article) {
    const title = article.querySelector(".message-title")?.innerText?.trim() || "";
    const address = article.querySelector(".message-meta a")?.href || "";
    const body = () => {
      const sel = window.getSelection();
      if (sel && !sel.isCollapsed && article.contains(sel.anchorNode)) return sel.toString();
      return article.querySelector(".message-body")?.innerText?.trim() || title;
    };
    return menuList([
      linkItems(href || address),
      { label: "Copy Title", disabled: !title || title === "Item Details" || title === "Reader", action: () => writeClipboard(title) },
      { label: "Copy Address", disabled: !address, action: () => writeClipboard(address) },
      { sep: true },
      editGroup({
        copy: body() ? () => writeClipboard(body()) : null,
        paste: null,
        selectAll: article.querySelector(".message-body") ? () => selectContents(article.querySelector(".message-body")) : null,
      }),
    ]);
  }

  if (target.closest?.(".words-chart")) {
    const words = [...document.querySelectorAll(".words-legend button")].map((button) => button.textContent.trim()).filter(Boolean);
    const text = lines(words);
    return menuList([
      editGroup({ copy: text ? () => writeClipboard(text) : null, paste: null, selectAll: null }),
    ]);
  }

  const kind = listKind(target, api.view);
  if (kind) api.setScope(kind);
  return menuList([
    api.view === "sources" && target.closest?.(".pane.is-on") ? { label: "New Source…", key: "⌃N", action: () => api.doAction("add") } : null,
    { sep: true },
    editGroup({
      copy: null,
      paste: null,
      selectAll: kind ? () => api.selectAll(kind) : null,
    }),
  ]);
}

function itemMenu(api, href, selected) {
  const titles = lines(selected.map((item) => item.title || "Untitled"));
  const urls = lines(selected.map((item) => item.url));
  const errors = lines(selected.map((item) => item.error));
  const one = selected.length === 1 ? selected[0] : null;
  return menuList([
    linkItems(href),
    api.view === "captures" ? { label: "Open in Reader", disabled: !one, action: () => api.openReader(one.id) } : null,
    api.view === "reader" ? { label: "Mark as Read", disabled: !selected.some((item) => !api.isRead?.(item)), action: () => api.markRead(selected.map((item) => item.id), true) } : null,
    api.view === "reader" ? { label: "Mark as Unread", disabled: !selected.some((item) => api.isRead?.(item)), action: () => api.markRead(selected.map((item) => item.id), false) } : null,
    { label: "Show in Finder", key: "⌃O", disabled: !selected.some((item) => item.downloadLocation), action: () => api.doAction("files") },
    { label: "Open Address", disabled: !urls, action: () => urls.split("\n").forEach((url) => window.open(url, "_blank", "noopener")) },
    { sep: true },
    { label: "Copy Title", disabled: !titles, action: () => writeClipboard(titles) },
    { label: "Copy Address", disabled: !urls, action: () => writeClipboard(urls) },
    { label: "Copy Error", disabled: !errors, action: () => writeClipboard(errors) },
    { sep: true },
    editGroup({
      copy: titles ? () => writeClipboard(api.selectionText(api.view === "reader" ? "reader" : "items")) : null,
      paste: null,
      selectAll: () => api.selectAll(api.view === "reader" ? "reader" : "items"),
    }),
    { sep: true },
    { label: "Remove…", disabled: !selected.length, action: () => api.removeSelection(api.view === "reader" ? "reader" : "items") },
  ]);
}

function listKind(target, view) {
  if (target.closest?.(".items-scroll")) return "items";
  if (target.closest?.(".runs-scroll")) return "runs";
  if (target.closest?.(".reader-list")) return "reader";
  if (view === "sources" && target.closest?.(".pane.is-on")) return "sources";
  if (view === "captures" && target.closest?.(".captures-pane")) return "runs";
  return "";
}

function selectContents(node) {
  const range = document.createRange();
  range.selectNodeContents(node);
  const selection = window.getSelection();
  selection.removeAllRanges();
  selection.addRange(range);
}

export function ContextMenu({ menu, onClose }) {
  const ref = useRef(null);
  const [hi, setHi] = useState(-1);

  useEffect(() => {
    setHi(menu.items.findIndex((item) => !item.sep && !item.disabled));
  }, [menu]);

  useLayoutEffect(() => {
    const node = ref.current;
    if (!node) return;
    const box = node.getBoundingClientRect();
    let x = menu.x;
    let y = menu.y;
    if (x + box.width > window.innerWidth - 6) x = Math.max(6, window.innerWidth - box.width - 6);
    if (y + box.height > window.innerHeight - 6) y = Math.max(6, menu.y - box.height);
    if (y + box.height > window.innerHeight - 6) y = Math.max(6, window.innerHeight - box.height - 6);
    node.style.left = `${x}px`;
    node.style.top = `${y}px`;
  }, [menu]);

  useEffect(() => {
    function outside(event) {
      if (event.button === 2) return;
      if (ref.current?.contains(event.target)) return;
      onClose();
    }
    function onKey(event) {
      const items = menu.items;
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onClose();
        return;
      }
      if (event.key !== "ArrowDown" && event.key !== "ArrowUp" && event.key !== "Enter") return;
      event.preventDefault();
      event.stopPropagation();
      if (event.key === "Enter") {
        const item = items[hi];
        if (item && !item.sep && !item.disabled) {
          onClose();
          item.action?.();
        }
        return;
      }
      const delta = event.key === "ArrowDown" ? 1 : -1;
      let next = hi;
      for (let step = 0; step < items.length; step += 1) {
        next = (next + delta + items.length) % items.length;
        if (!items[next].sep && !items[next].disabled) {
          setHi(next);
          return;
        }
      }
    }
    document.addEventListener("mousedown", outside, true);
    document.addEventListener("keydown", onKey, true);
    document.addEventListener("scroll", onClose, true);
    window.addEventListener("blur", onClose);
    return () => {
      document.removeEventListener("mousedown", outside, true);
      document.removeEventListener("keydown", onKey, true);
      document.removeEventListener("scroll", onClose, true);
      window.removeEventListener("blur", onClose);
    };
  }, [menu, hi, onClose]);

  return (
    <div ref={ref} className="context-menu" role="menu" style={{ left: menu.x, top: menu.y }}>
      {menu.items.map((item, index) => (item.sep ? <div className="menu-sep" key={`sep-${index}`} /> : (
        <button
          key={`${item.label}-${index}`}
          type="button"
          role="menuitem"
          disabled={item.disabled}
          className={index === hi ? "is-hi" : undefined}
          onMouseEnter={() => { if (!item.disabled) setHi(index); }}
          onClick={() => {
            if (item.disabled) return;
            onClose();
            item.action?.();
          }}
        >
          <span>{item.label}</span>
          {item.key ? <span className="key">{item.key}</span> : <span />}
        </button>
      )))}
    </div>
  );
}

export function selectionLines(kind, api) {
  if (kind === "sources") {
    return lines(api.liveSchedules().map((row) => [row.name, row.source.url, row.schedule.downloadLocation].filter(Boolean).join("\t")));
  }
  if (kind === "runs") {
    return lines(api.liveRuns().map((run) => [run.status, run.name, run.error || run.note || ""].filter(Boolean).join("\t")));
  }
  if (kind === "items" || kind === "reader") {
    return lines(api.liveItems().map((item) => [item.title || "Untitled", item.url || "", item.error || ""].filter(Boolean).join("\t")));
  }
  return "";
}
