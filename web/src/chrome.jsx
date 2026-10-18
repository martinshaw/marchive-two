import { Icon } from "./sprite.jsx";

export const SIDES = [
  { label: "Library" },
  { view: "sources", filter: "all", icon: "#i-tray", tone: "tone-library", text: "All Sources", count: "sources" },
  { view: "captures", filter: "all", icon: "#i-dot", tone: "tone-capture", text: "All Captures", count: "captures" },
  { label: "Sources" },
  { view: "sources", filter: "webpage", icon: "#i-globe", tone: "tone-web", text: "Webpages", count: "webpage" },
  { view: "sources", filter: "article_index", icon: "#i-news", tone: "tone-news", text: "Articles", count: "article_index" },
  { view: "sources", filter: "podcast", icon: "#i-podcast", tone: "tone-podcast", text: "Podcasts", count: "podcast" },
  { view: "sources", filter: "text_extract", icon: "#i-text", tone: "tone-text", text: "Text", count: "text_extract" },
  { view: "sources", filter: "x_trending", icon: "#i-trend", tone: "tone-x", text: "X trending", count: "x_trending" },
  { view: "sources", filter: "hn_entry", icon: "#i-hn", tone: "tone-hn", text: "Hacker News", count: "hn_entry" },
  { view: "sources", filter: "google_search", icon: "#i-google", tone: "tone-google", text: "Google", count: "google_search" },
  { view: "sources", filter: "ddg_search", icon: "#i-ddg", tone: "tone-ddg", text: "DuckDuckGo", count: "ddg_search" },
  { label: "Captures" },
  { view: "captures", filter: "active", icon: "#i-play", tone: "tone-active", text: "In Progress", count: "active" },
  { view: "captures", filter: "failed", icon: "#i-x", tone: "tone-failed", text: "Failed", count: "failed" },
  { view: "captures", filter: "succeeded", icon: "#i-dot", tone: "tone-ok", text: "Succeeded", count: "succeeded" },
  { label: "Analysis" },
  { view: "words", filter: "words", icon: "#i-cloud", tone: "tone-words", text: "Words" },
  { label: "Apps" },
  { view: "reader", filter: "reader", icon: "#i-mail", tone: "tone-mail", text: "Reader" },
  { label: "Marchive" },
  { view: "settings", filter: "", icon: "#i-gear", tone: "tone-gear", text: "Settings" },
];

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
  return (
    <aside className="sidebar">
      {SIDES.map((item) => item.label ? <p className="side-label" key={item.label}>{item.label}</p> : (
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
    </aside>
  );
}
