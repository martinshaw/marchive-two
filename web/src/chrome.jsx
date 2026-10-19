import { useEffect, useRef, useState } from "react";
import { Icon } from "./sprite.jsx";

const SIDE_KEY = "marchive-sidebar";
const SIDE_WIDTH_KEY = "marchive-sidebar-width";
const SIDE_DEFAULT = 220;
const SIDE_MIN = 168;
const SIDE_MAX = 420;

const SECTIONS = [
  {
    id: "library",
    label: "Library",
    items: [
      { view: "sources", filter: "all", icon: "#i-tray", tone: "tone-library", text: "All Sources", count: "sources" },
      { view: "captures", filter: "all", icon: "#i-dot", tone: "tone-capture", text: "All Captures", count: "captures" },
    ],
  },
  {
    id: "sources",
    label: "Sources",
    items: [
      { view: "sources", filter: "webpage", icon: "#i-globe", tone: "tone-web", text: "Webpages", count: "webpage" },
      { view: "sources", filter: "article_index", icon: "#i-news", tone: "tone-news", text: "Articles", count: "article_index" },
      { view: "sources", filter: "podcast", icon: "#i-podcast", tone: "tone-podcast", text: "Podcasts", count: "podcast" },
      { view: "sources", filter: "text_extract", icon: "#i-text", tone: "tone-text", text: "Text", count: "text_extract" },
      { view: "sources", filter: "x_trending", icon: "#i-trend", tone: "tone-x", text: "X trending", count: "x_trending" },
      { view: "sources", filter: "hn_entry", icon: "#i-hn", tone: "tone-hn", text: "Hacker News", count: "hn_entry" },
      { view: "sources", filter: "google_search", icon: "#i-google", tone: "tone-google", text: "Google", count: "google_search" },
      { view: "sources", filter: "ddg_search", icon: "#i-ddg", tone: "tone-ddg", text: "DuckDuckGo", count: "ddg_search" },
      { view: "sources", filter: "youtube_video", icon: "#i-youtube", tone: "tone-youtube", text: "YouTube", count: "youtube_video" },
    ],
  },
  {
    id: "captures",
    label: "Captures",
    items: [
      { view: "captures", filter: "active", icon: "#i-play", tone: "tone-active", text: "In Progress", count: "active" },
      { view: "captures", filter: "failed", icon: "#i-x", tone: "tone-failed", text: "Failed", count: "failed" },
      { view: "captures", filter: "succeeded", icon: "#i-dot", tone: "tone-ok", text: "Succeeded", count: "succeeded" },
    ],
  },
  {
    id: "analysis",
    label: "Analysis",
    items: [
      { view: "words", filter: "words", icon: "#i-cloud", tone: "tone-words", text: "Words" },
    ],
  },
  {
    id: "apps",
    label: "Apps",
    items: [
      { view: "reader", filter: "reader", icon: "#i-mail", tone: "tone-mail", text: "Reader" },
    ],
  },
  {
    id: "marchive",
    label: "Marchive",
    items: [
      { view: "settings", filter: "", icon: "#i-gear", tone: "tone-gear", text: "Settings" },
    ],
  },
];

function storedSidebar() {
  const known = SECTIONS.map((section) => section.id);
  const fallback = { order: known, collapsed: {} };
  try {
    const raw = JSON.parse(localStorage.getItem(SIDE_KEY) || "");
    const order = Array.isArray(raw.order) ? raw.order.filter((id) => known.includes(id)) : [];
    for (const id of known) if (!order.includes(id)) order.push(id);
    const collapsed = {};
    for (const id of known) collapsed[id] = !!raw.collapsed?.[id];
    return { order, collapsed };
  } catch {
    return fallback;
  }
}

function storedSideWidth() {
  try {
    const width = Number(localStorage.getItem(SIDE_WIDTH_KEY));
    if (Number.isFinite(width)) return clampSide(width, SIDE_MAX);
  } catch { /* Keep the built-in width. */ }
  return SIDE_DEFAULT;
}

function clampSide(value, max) {
  return Math.round(Math.min(Math.max(value, SIDE_MIN), Math.max(SIDE_MIN, max)));
}

function pack(order, dragId, dropIndex) {
  const rest = order.filter((id) => id !== dragId);
  return [...rest.slice(0, dropIndex), dragId, ...rest.slice(dropIndex)];
}

export function Menu({ name, titleClass = "menu-title", open, onOpen, onHover, children }) {
  return (
    <div className={open ? "menu is-open" : "menu"} onMouseDown={(event) => event.stopPropagation()}>
      <button type="button" className={titleClass} aria-expanded={open} aria-haspopup="true" onMouseOver={onHover} onClick={onOpen}>{name}</button>
      <div className="menu-pop" role="menu" hidden={!open}>{children}</div>
    </div>
  );
}

function Tool({ icon, label, disabled, danger, onClick }) {
  return (
    <button type="button" className={danger ? "tool danger-tool" : "tool"} disabled={disabled} onMouseDown={(event) => event.stopPropagation()} onClick={onClick}>
      <Icon href={icon} />
      <span>{label}</span>
    </button>
  );
}

export function MenuBar({ menu, setMenu, doAction, showView, library, runnerOn, editOn, filesOn, removeOn, sourcesOn, activeRun, signing }) {
  return (
    <header className="menubar">
      <nav className="menus" aria-label="Application">
        <Menu name="Marchive" titleClass="menu-title app-title" open={menu === "app"} onOpen={() => setMenu(menu === "app" ? "" : "app")} onHover={() => menu && setMenu("app")}>
          <button type="button" role="menuitem" onClick={() => doAction("about")}>About Marchive</button>
          {!library?.chromePath ? <button type="button" role="menuitem" onClick={() => doAction("chrome-install")}>Install Google Chrome…</button> : null}
          <button type="button" role="menuitem" onClick={() => doAction("ytdlp")}>{library?.ytDlpPath ? "Update yt-dlp…" : "Install yt-dlp…"}</button>
          <div className="menu-sep" />
          <button type="button" role="menuitem" onClick={() => doAction("settings")}>Settings…<span className="key">⌃,</span></button>
        </Menu>
        <Menu name="File" open={menu === "file"} onOpen={() => setMenu(menu === "file" ? "" : "file")} onHover={() => menu && setMenu("file")}>
          <button type="button" role="menuitem" onClick={() => doAction("add")}>New Source…<span className="key">⌃N</span></button>
          <button type="button" role="menuitem" disabled={!editOn} onClick={() => doAction("edit")}>Edit Schedule…<span className="key">⌃E</span></button>
          <div className="menu-sep" />
          <button type="button" role="menuitem" disabled={!filesOn} onClick={() => doAction("files")}>Show in Finder<span className="key">⌃O</span></button>
          <button type="button" role="menuitem" disabled={!removeOn} onClick={() => doAction("remove")}>Remove…</button>
        </Menu>
        <Menu name="Capture" open={menu === "capture"} onOpen={() => setMenu(menu === "capture" ? "" : "capture")} onHover={() => menu && setMenu("capture")}>
          <button type="button" role="menuitem" disabled={!sourcesOn} onClick={() => doAction("run")}>Run Now<span className="key">⌃R</span></button>
          <button type="button" role="menuitem" onClick={() => doAction("runner")}>{runnerOn ? "Stop Runner" : "Start Runner"}</button>
          <button type="button" role="menuitem" disabled={!activeRun} onClick={() => doAction("cancel")}>Cancel Capture</button>
          <div className="menu-sep" />
          <button type="button" role="menuitem" disabled={signing} onClick={() => doAction("signin")}>Sign In with Chrome</button>
        </Menu>
        <Menu name="View" open={menu === "view"} onOpen={() => setMenu(menu === "view" ? "" : "view")} onHover={() => menu && setMenu("view")}>
          <button type="button" role="menuitem" onClick={() => showView("sources", "all")}>All Sources<span className="key">⌃1</span></button>
          <button type="button" role="menuitem" onClick={() => showView("captures", "all")}>All Captures<span className="key">⌃2</span></button>
          <button type="button" role="menuitem" onClick={() => showView("words", "words")}>Words</button>
          <button type="button" role="menuitem" onClick={() => showView("reader", "reader")}>Reader</button>
          <div className="menu-sep" />
          <button type="button" role="menuitem" onClick={() => showView("sources", "webpage")}>Webpages</button>
          <button type="button" role="menuitem" onClick={() => showView("sources", "article_index")}>Articles</button>
          <button type="button" role="menuitem" onClick={() => showView("sources", "podcast")}>Podcasts</button>
          <button type="button" role="menuitem" onClick={() => showView("sources", "text_extract")}>Text</button>
          <button type="button" role="menuitem" onClick={() => showView("sources", "x_trending")}>X trending</button>
          <button type="button" role="menuitem" onClick={() => showView("sources", "hn_entry")}>Hacker News</button>
          <button type="button" role="menuitem" onClick={() => showView("sources", "google_search")}>Google</button>
          <button type="button" role="menuitem" onClick={() => showView("sources", "ddg_search")}>DuckDuckGo</button>
          <button type="button" role="menuitem" onClick={() => showView("sources", "youtube_video")}>YouTube</button>
        </Menu>
      </nav>
      <p className="menu-extra">{runnerOn ? "Runner on" : "Runner off"}</p>
    </header>
  );
}

export function Toolbar({ query, setQuery, doAction, runnerOn, sourcesOn, activeRun, editOn, filesOn, removeOn, signing }) {
  return (
    <div className="toolbar">
      <div className="tools" role="toolbar" aria-label="Actions">
        <button type="button" className={runnerOn ? "tool is-on" : "tool"} id="tool-runner" onMouseDown={(event) => event.stopPropagation()} onClick={() => doAction("runner")}>
          <Icon href={runnerOn ? "#i-stop" : "#i-play"} />
          <span>{runnerOn ? "Stop" : "Start"}</span>
        </button>
        <span className="tool-sep" />
        <Tool icon="#i-plus" label="Add" onClick={() => doAction("add")} />
        <Tool icon="#i-play" label="Run" disabled={!sourcesOn} onClick={() => doAction("run")} />
        <Tool icon="#i-x" label="Cancel" disabled={!activeRun} onClick={() => doAction("cancel")} />
        <span className="tool-sep" />
        <Tool icon="#i-pencil" label="Edit" disabled={!editOn} onClick={() => doAction("edit")} />
        <Tool icon="#i-folder" label="Files" disabled={!filesOn} onClick={() => doAction("files")} />
        <Tool icon="#i-trash" label="Remove" danger disabled={!removeOn} onClick={() => doAction("remove")} />
        <span className="tool-sep" />
        <Tool icon="#i-person" label="Sign In" disabled={signing} onClick={() => doAction("signin")} />
      </div>
      <label className="filter-field">
        <input id="table-filter" type="search" placeholder="Filter" autoComplete="off" spellCheck="false" aria-label="Filter" value={query} onChange={(event) => setQuery(event.target.value)} />
      </label>
    </div>
  );
}

export function Sidebar({ view, filter, counts, showView }) {
  const stored = useRef(null);
  if (!stored.current) stored.current = storedSidebar();
  const [order, setOrder] = useState(stored.current.order);
  const [collapsed, setCollapsed] = useState(stored.current.collapsed);
  const [drag, setDrag] = useState(null);
  const asideRef = useRef(null);
  const wrapRef = useRef(null);
  const [sideWidth, setSideWidth] = useState(storedSideWidth);
  const preferredRef = useRef(sideWidth);
  const [splitOn, setSplitOn] = useState(false);
  const sessionRef = useRef(null);
  const skipClick = useRef(false);
  const settleTimer = useRef(0);
  const byId = Object.fromEntries(SECTIONS.map((section) => [section.id, section]));

  useEffect(() => {
    try { localStorage.setItem(SIDE_KEY, JSON.stringify({ order, collapsed })); } catch { /* The sidebar still updates. */ }
  }, [order, collapsed]);

  useEffect(() => () => {
    window.clearTimeout(settleTimer.current);
    document.body.classList.remove("is-side-drag", "is-col-resize");
    sessionRef.current?.cleanup?.();
  }, []);

  function sideLimit() {
    const parent = wrapRef.current?.parentElement;
    if (!parent) return SIDE_MAX;
    return Math.min(SIDE_MAX, parent.clientWidth - 280);
  }

  useEffect(() => {
    function fit() {
      setSideWidth(clampSide(preferredRef.current, sideLimit()));
    }
    fit();
    window.addEventListener("resize", fit);
    return () => window.removeEventListener("resize", fit);
  }, []);

  function rememberWidth(next) {
    const width = clampSide(next, SIDE_MAX);
    preferredRef.current = width;
    const shown = clampSide(width, sideLimit());
    setSideWidth(shown);
    try { localStorage.setItem(SIDE_WIDTH_KEY, String(width)); } catch { /* The sidebar still resizes. */ }
    return shown;
  }

  function onSplitPointerDown(event) {
    if (event.button !== 0 || sessionRef.current) return;
    event.preventDefault();
    event.stopPropagation();
    const pointer = event.pointerId;
    const startX = event.clientX;
    const startW = wrapRef.current.getBoundingClientRect().width;
    let delta = 0;
    setSplitOn(true);
    document.body.classList.add("is-col-resize");
    try { event.currentTarget.setPointerCapture(pointer); } catch { /* Window listeners still follow the pointer. */ }
    function move(e) {
      if (e.pointerId !== pointer) return;
      delta = e.clientX - startX;
      setSideWidth(clampSide(startW + delta, sideLimit()));
    }
    function end(e) {
      if (e.pointerId !== pointer) return;
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", end);
      window.removeEventListener("pointercancel", end);
      document.body.classList.remove("is-col-resize");
      setSplitOn(false);
      if (delta !== 0) rememberWidth(startW + delta);
    }
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", end);
    window.addEventListener("pointercancel", end);
  }

  function onSplitKey(event) {
    const step = event.shiftKey ? 24 : 8;
    if (event.key === "ArrowRight") { event.preventDefault(); rememberWidth(sideWidth + step); }
    if (event.key === "ArrowLeft") { event.preventDefault(); rememberWidth(sideWidth - step); }
  }

  function onHeaderClick(id) {
    if (skipClick.current) {
      skipClick.current = false;
      return;
    }
    setCollapsed((prev) => ({ ...prev, [id]: !prev[id] }));
  }

  function onHeaderPointerDown(event, id) {
    if (event.button !== 0 || sessionRef.current) return;
    const aside = asideRef.current;
    if (!aside) return;
    const sideRect = aside.getBoundingClientRect();
    const base = {};
    const heights = {};
    for (const el of aside.querySelectorAll(".side-group")) {
      const rect = el.getBoundingClientRect();
      base[el.dataset.id] = rect.top - sideRect.top + aside.scrollTop;
      heights[el.dataset.id] = rect.height;
    }
    const contentY = (clientY) => {
      const rect = aside.getBoundingClientRect();
      return clientY - rect.top + aside.scrollTop;
    };
    const session = {
      id,
      order,
      base,
      heights,
      origin: base[order[0]],
      grab: contentY(event.clientY) - base[id],
      startX: event.clientX,
      startY: event.clientY,
      lastY: event.clientY,
      moved: false,
      dropIndex: order.indexOf(id),
    };
    sessionRef.current = session;

    function topsFor(dropIndex) {
      const tops = {};
      let y = session.origin;
      for (const key of pack(session.order, session.id, dropIndex)) {
        tops[key] = y;
        y += session.heights[key];
      }
      return tops;
    }

    function dropIndexFor(clientY) {
      const center = contentY(clientY) - session.grab + session.heights[session.id] / 2;
      let index = 0;
      for (const key of session.order) {
        if (key === session.id) continue;
        if (center > session.base[key] + session.heights[key] / 2) index += 1;
      }
      return index;
    }

    function paint(clientY, settling) {
      const dropIndex = settling ? session.dropIndex : dropIndexFor(clientY);
      session.dropIndex = dropIndex;
      const tops = topsFor(dropIndex);
      const translates = {};
      const rect = aside.getBoundingClientRect();
      const min = aside.scrollTop;
      const max = aside.scrollTop + rect.height - Math.min(session.heights[session.id], 36);
      for (const key of session.order) {
        if (key === session.id && !settling) {
          const desired = Math.max(min, Math.min(contentY(clientY) - session.grab, max));
          translates[key] = desired - session.base[key];
        } else {
          translates[key] = tops[key] - session.base[key];
        }
      }
      session.liveTranslates = translates;
      session.slotTranslates = Object.fromEntries(session.order.map((key) => [key, tops[key] - session.base[key]]));
      setDrag({ id: session.id, translates, settling });
    }

    function move(e) {
      const current = sessionRef.current;
      if (!current || current.settling) return;
      current.lastY = e.clientY;
      if (!current.moved && Math.hypot(e.clientX - current.startX, e.clientY - current.startY) < 5) return;
      if (!current.moved) {
        current.moved = true;
        document.body.classList.add("is-side-drag");
      }
      const rect = aside.getBoundingClientRect();
      if (e.clientY < rect.top + 28) aside.scrollTop -= 12;
      else if (e.clientY > rect.bottom - 28) aside.scrollTop += 12;
      paint(e.clientY, false);
    }

    function end(e) {
      session.cleanup();
      document.body.classList.remove("is-side-drag");
      const current = sessionRef.current;
      if (!current || !current.moved) {
        sessionRef.current = null;
        return;
      }
      skipClick.current = true;
      if (e.type === "pointercancel") {
        sessionRef.current = null;
        setDrag(null);
        return;
      }
      const next = pack(current.order, current.id, current.dropIndex);
      const same = next.every((key, index) => key === current.order[index]);
      current.settling = true;
      setDrag({ id: current.id, translates: current.liveTranslates, settling: true });
      requestAnimationFrame(() => {
        setDrag({ id: current.id, translates: current.slotTranslates, settling: true });
      });
      settleTimer.current = window.setTimeout(() => {
        if (!same) setOrder(next);
        setDrag(null);
        sessionRef.current = null;
      }, 200);
    }

    session.cleanup = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", end);
      window.removeEventListener("pointercancel", end);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", end);
    window.addEventListener("pointercancel", end);
  }

  return (
    <div className="sidebar-wrap" ref={wrapRef} style={{ width: sideWidth }}>
    <aside className="sidebar" ref={asideRef}>
      {order.map((id) => {
        const section = byId[id];
        if (!section) return null;
        const closed = !!collapsed[id];
        const lifted = drag?.id === id;
        const style = drag ? {
          transform: `translateY(${drag.translates[id] || 0}px)`,
          transition: lifted && !drag.settling ? "none" : undefined,
          zIndex: lifted ? 4 : undefined,
        } : undefined;
        return (
          <div key={id} className={closed ? "side-group is-collapsed" : "side-group"} data-id={id} style={style} role="group" aria-label={section.label}>
            <div className={lifted ? "side-card is-lifted" : "side-card"}>
              <button
                type="button"
                className="side-label"
                aria-expanded={!closed}
                onMouseDown={(event) => event.stopPropagation()}
                onPointerDown={(event) => onHeaderPointerDown(event, id)}
                onClick={() => onHeaderClick(id)}
              >
                <span>{section.label}</span>
                <svg className="side-chevron" viewBox="0 0 12 12" aria-hidden="true">
                  <path d="M2.2 4.4 6 8.1 9.8 4.4" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
              </button>
              <div className={closed ? "side-items is-closed" : "side-items"}>
                <div className="side-items-inner" inert={closed || undefined}>
                  {section.items.map((item) => (
                    <button
                      type="button"
                      key={`${item.view}-${item.filter}`}
                      className={view === item.view && filter === item.filter ? "side-item is-selected" : "side-item"}
                      onMouseDown={(event) => event.stopPropagation()}
                      onClick={() => showView(item.view, item.filter)}
                    >
                      <Icon href={item.icon} className={`side-icon ${item.tone}`} />
                      <span>{item.text}</span>
                      {item.count ? <span className="count">{counts[item.count] || ""}</span> : null}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          </div>
        );
      })}
    </aside>
    <div
      className={splitOn ? "side-split is-on" : "side-split"}
      role="separator"
      tabIndex={0}
      aria-orientation="vertical"
      aria-label="Resize sidebar"
      aria-valuemin={SIDE_MIN}
      aria-valuemax={SIDE_MAX}
      aria-valuenow={sideWidth}
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={onSplitPointerDown}
      onDoubleClick={() => rememberWidth(SIDE_DEFAULT)}
      onKeyDown={onSplitKey}
    />
    </div>
  );
}
