import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { getState, send } from "./api.js";
import { CapturePane } from "./CapturePane.jsx";
import { CellTip } from "./CellTip.jsx";
import { ChromeAlert } from "./ChromeAlert.jsx";
import { MenuBar, Sidebar, Toolbar } from "./chrome.jsx";
import { bindCommands } from "./commands.js";
import { buildMenu, ContextMenu, selectionLines, writeClipboard } from "./ContextMenu.jsx";
import { matchesQuery } from "./format.js";
import { captureRuns, itemText, libraryCounts, runVisible, scheduleRows, sourceVisible } from "./library.js";
import { useMessage } from "./Message.jsx";
import { readerCatalog, readerChoices, readerFiltered, readerView } from "./reader.js";
import { ReaderPane } from "./ReaderPane.jsx";
import { itemGridRows, itemsEmptyText, runGridRows, sourceGridRows } from "./rows.jsx";
import { createSelection, sameIds } from "./selection.js";
import { AddSheet } from "./AddSheet.jsx";
import { AboutSheet, ConfirmSheet, EditSheet, Settings } from "./Sheets.jsx";
import { SourcesPane } from "./SourcesPane.jsx";
import { Sprite } from "./sprite.jsx";
import { useCloud } from "./Words.jsx";
import { WordsPane } from "./WordsPane.jsx";

export function App() {
  const [library, setLibrary] = useState(null);
  const [view, setView] = useState("sources");
  const [filter, setFilter] = useState("all");
  const [query, setQuery] = useState("");
  const [scheduleIds, setScheduleIds] = useState([]);
  const [runIds, setRunIds] = useState([]);
  const [itemIds, setItemIds] = useState([]);
  const [readerIds, setReaderIds] = useState([]);
  const marks = useRef({
    sources: { anchor: null, lead: null },
    runs: { anchor: null, lead: null },
    items: { anchor: null, lead: null },
    reader: { anchor: null, lead: null },
  });
  const scope = useRef("sources");
  const orderRef = useRef({ sources: [], runs: [], items: [] });
  const picked = useRef({ sources: [], runs: [], items: [], reader: [] });
  const [banner, setBanner] = useState("");
  const [offline, setOffline] = useState(false);
  const [menu, setMenu] = useState("");
  const [context, setContext] = useState(null);
  const [sheet, setSheet] = useState("");
  const [confirm, setConfirm] = useState(null);
  const [editVersion, setEditVersion] = useState(0);
  const editRow = useRef(null);
  const [chromeOpen, setChromeOpen] = useState(false);
  const [chromeDetail, setChromeDetail] = useState("");
  const [focus, setFocus] = useState("");
  const [hideCommon, setHideCommon] = useState(() => {
    try { return localStorage.getItem("marchive-hide-common") !== "0"; } catch { return true; }
  });
  const [hideURL, setHideURL] = useState(() => {
    try { return localStorage.getItem("marchive-hide-url") !== "0"; } catch { return true; }
  });
  const [detailTab, setDetailTab] = useState("message");
  const [readerSource, setReaderSource] = useState("");
  const [readerKind, setReaderKind] = useState("");
  const [readerRead, setReaderRead] = useState("all");
  const [readerSort, setReaderSort] = useState("newest");
  const [readerGroup, setReaderGroup] = useState("none");
  const [readPending, setReadPending] = useState({});
  const holdUnread = useRef(new Set());
  const [compactCapture, setCompactCapture] = useState(() => window.matchMedia("(max-height: 1000px)").matches);
  const confirmAction = useRef(null);
  const readerList = useRef(null);

  const reload = useCallback(async () => {
    try {
      setLibrary(await getState());
      setOffline(false);
    } catch {
      setOffline(true);
    }
  }, []);

  useEffect(() => {
    const media = window.matchMedia("(max-height: 1000px)");
    const apply = () => setCompactCapture(media.matches);
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("error")) setBanner(params.get("error"));
    let stop = false;
    let timer = 0;
    async function tick() {
      if (!stop && !document.hidden) {
        try {
          setLibrary(await getState());
          setOffline(false);
        } catch {
          setOffline(true);
        }
      }
      if (!stop) timer = window.setTimeout(tick, document.hidden ? 15000 : 2000);
    }
    tick();
    function onShow() { if (!document.hidden) tick(); }
    document.addEventListener("visibilitychange", onShow);
    return () => {
      stop = true;
      window.clearTimeout(timer);
      document.removeEventListener("visibilitychange", onShow);
    };
  }, []);

  useEffect(() => {
    if (library && !library.chromePath && sessionStorage.getItem("marchive-chrome-later") !== "1") setChromeOpen(true);
  }, [library?.chromePath]);

  const schedules = useMemo(() => scheduleRows(library), [library]);
  const runs = useMemo(() => captureRuns(library), [library]);

  const selectedSchedules = scheduleIds.map((id) => schedules.find((row) => row.schedule.id === id)).filter(Boolean);
  const selectedRuns = runIds.map((id) => runs.find((run) => run.id === id)).filter(Boolean);
  const oneRun = selectedRuns.length === 1 ? selectedRuns[0] : null;
  const selectedItems = oneRun ? itemIds.map((id) => (oneRun.items || []).find((item) => item.id === id)).filter(Boolean) : [];
  const oneItem = selectedItems.length === 1 ? selectedItems[0] : null;
  const oneReader = readerIds.length === 1 ? readerIds[0] : null;
  const itemRevision = (oneRun?.items || []).map((item) => `${item.id}:${item.status}:${item.updatedAt || ""}`).join("|");
  const message = useMessage(oneItem?.id || null, itemRevision);
  const readerMessage = useMessage(oneReader, itemRevision);
  const captureHold = runIds.length > 1 ? "Select a single capture for more details." : "";
  const itemHold = runIds.length > 1 ? "Select a single capture for more details." : (itemIds.length > 1 ? "Select a single item for more details." : "");
  const readerHold = readerIds.length > 1 ? "Select a single item for more details." : "";

  useEffect(() => {
    setScheduleIds((ids) => {
      const next = ids.filter((id) => {
        const row = schedules.find((item) => item.schedule.id === id);
        if (!row) return false;
        if (view === "sources" && !sourceVisible(row, view, filter, query)) return false;
        return true;
      });
      if (sameIds(ids, next)) return ids;
      picked.current.sources = next;
      return next;
    });
  }, [schedules, view, filter, query]);

  useEffect(() => {
    setRunIds((ids) => {
      const next = ids.filter((id) => {
        const run = runs.find((item) => item.id === id);
        if (!run) return false;
        if (view === "captures" && !runVisible(run, view, filter, query)) return false;
        return true;
      });
      if (sameIds(ids, next)) return ids;
      picked.current.runs = next;
      return next;
    });
  }, [runs, view, filter, query]);

  useEffect(() => {
    if (runIds.length !== 1) {
      setItemIds((ids) => {
        if (!ids.length) return ids;
        picked.current.items = [];
        return [];
      });
      return;
    }
    const run = runs.find((item) => item.id === runIds[0]);
    setItemIds((ids) => {
      const next = ids.filter((id) => {
        const item = (run?.items || []).find((entry) => entry.id === id);
        if (!item) return false;
        if (view === "captures" && !matchesQuery(query, itemText(item))) return false;
        return true;
      });
      if (sameIds(ids, next)) return ids;
      picked.current.items = next;
      return next;
    });
  }, [runIds, runs, view, query]);

  const readerItems = useMemo(() => readerCatalog(runs), [runs]);
  const readerOpts = useMemo(() => readerChoices(readerItems), [readerItems]);
  const readerState = useMemo(
    () => readerView(readerItems, { query, source: readerSource, kind: readerKind, read: readerRead, sort: readerSort, group: readerGroup, pending: readPending }),
    [readerItems, query, readerSource, readerKind, readerRead, readerSort, readerGroup, readPending],
  );
  const shownReader = readerState.ordered;
  const filtersActive = readerFiltered({ source: readerSource, kind: readerKind, read: readerRead, query });

  useEffect(() => {
    setReadPending((prev) => {
      let changed = false;
      const next = { ...prev };
      readerItems.forEach((item) => {
        if (next[item.id] === undefined || next[item.id] !== item.read) return;
        delete next[item.id];
        changed = true;
      });
      return changed ? next : prev;
    });
  }, [readerItems]);

  useEffect(() => {
    const visible = new Set(shownReader.map((item) => item.id));
    setReaderIds((ids) => {
      const next = ids.filter((id) => visible.has(id));
      if (sameIds(ids, next)) return ids;
      picked.current.reader = next;
      return next;
    });
  }, [shownReader]);

  const counts = useMemo(() => libraryCounts(schedules, runs), [schedules, runs]);
  const wordQuery = (() => {
    const params = new URLSearchParams();
    if (!hideCommon) params.set("common", "1");
    if (!hideURL) params.set("url", "1");
    const text = params.toString();
    return text ? `?${text}` : "";
  })();
  const runSignature = oneRun ? `${oneRun.status}:${(oneRun.items || []).map((item) => item.status).join(",")}` : "";
  const wordsRevision = runs.map((run) => `${run.id}:${run.status}:${(run.items || []).length}`).join("|");
  const captureCloud = useCloud(oneRun && !captureHold ? `/api/runs/${oneRun.id}/words${wordQuery}` : "", !!oneRun && !captureHold, runSignature);
  const itemCloud = useCloud(oneItem && !itemHold ? `/api/items/${oneItem.id}/words${wordQuery}` : "", !!oneItem && !itemHold, itemRevision);
  const timeline = useCloud(`/api/words${wordQuery}`, view === "words", wordsRevision);

  function showView(nextView, nextFilter) {
    setMenu("");
    setView(nextView);
    setFilter(nextFilter ?? "");
  }

  function liveSchedules() {
    if (view !== "sources") return [];
    return (picked.current.sources || [])
      .map((id) => schedules.find((row) => row.schedule.id === id))
      .filter((row) => row && sourceVisible(row, view, filter, query));
  }

  function liveRuns() {
    return (picked.current.runs || []).map((id) => runs.find((run) => run.id === id)).filter(Boolean);
  }

  function liveItems() {
    const ids = view === "reader" ? picked.current.reader : picked.current.items;
    const found = [];
    (ids || []).forEach((id) => {
      for (const run of runs) {
        const item = (run.items || []).find((entry) => entry.id === id);
        if (item) {
          found.push(item);
          return;
        }
      }
    });
    return found;
  }

  const { act, openEdit, removeSelection, doAction, saveSchedule, removeSchedule } = bindCommands({
    view, library, reload, showView,
    setMenu, setContext, setSheet, setBanner, setChromeOpen, setEditVersion, setConfirm,
    editRow, confirmAction, liveSchedules, liveRuns, liveItems,
  });

  const markItems = useCallback((ids, read) => {
    const unique = [...new Set((ids || []).filter((id) => id))];
    if (!unique.length) return;
    unique.forEach((id) => {
      if (read) holdUnread.current.delete(id);
      else holdUnread.current.add(id);
    });
    setReadPending((prev) => {
      const next = { ...prev };
      unique.forEach((id) => { next[id] = read; });
      return next;
    });
    send("POST", "/api/items/read", { ids: unique, read }).then(() => reload()).catch(() => {
      setReadPending((prev) => {
        const next = { ...prev };
        unique.forEach((id) => { delete next[id]; });
        return next;
      });
    });
  }, [reload]);

  useEffect(() => {
    const current = readerIds.length === 1 ? readerIds[0] : null;
    holdUnread.current.forEach((id) => {
      if (id !== current) holdUnread.current.delete(id);
    });
  }, [readerIds]);

  useEffect(() => {
    if (view !== "reader" || context || readerIds.length !== 1) return;
    const id = readerIds[0];
    if (holdUnread.current.has(id)) return;
    const loaded = readerMessage.data && String(readerMessage.data.id) === String(id);
    if (!loaded) return;
    const item = readerItems.find((row) => row.id === id);
    const known = Object.prototype.hasOwnProperty.call(readPending, id) ? readPending[id] : item?.read;
    if (!item || known) return;
    markItems([id], true);
  }, [view, context, readerIds, readerMessage.data, readerItems, readPending, markItems]);

  const { applyIds, tableScope, take, move, selectAll, ensureRow } = createSelection({
    view, marks, scope, orderRef, picked, shownReader, readerItems,
    setScheduleIds, setRunIds, setItemIds, setReaderIds,
  });

  function openReader(id) {
    setReaderSource("");
    setReaderKind("");
    setReaderRead("all");
    setQuery("");
    showView("reader", "reader");
    marks.current.reader = { anchor: id, lead: id };
    scope.current = "reader";
    applyIds("reader", [id]);
  }

  function clearReaderFilters() {
    setReaderSource("");
    setReaderKind("");
    setReaderRead("all");
    setQuery("");
  }

  function isItemRead(item) {
    if (!item) return false;
    if (Object.prototype.hasOwnProperty.call(readPending, item.id)) return !!readPending[item.id];
    return !!item.read;
  }

  useEffect(() => {
    function shortcut(event, key) {
      return event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey && event.key.toLowerCase() === key;
    }
    function onKey(event) {
      const typing = event.target.closest("input, select, textarea");
      if (chromeOpen) {
        if (event.key === "Escape") setChromeOpen(false);
        event.preventDefault();
        return;
      }
      if (context) {
        if (event.key === "Escape") setContext(null);
        return;
      }
      if (event.key === "Escape") {
        if (event.target.id === "table-filter" && query) { setQuery(""); return; }
        setMenu("");
        setSheet("");
        return;
      }
      if (shortcut(event, "n")) { event.preventDefault(); doAction("add"); return; }
      if (shortcut(event, ",")) { event.preventDefault(); showView("settings", ""); return; }
      if (shortcut(event, "1")) { event.preventDefault(); showView("sources", "all"); return; }
      if (shortcut(event, "2")) { event.preventDefault(); showView("captures", "all"); return; }
      if (typing) return;
      if (event.metaKey && !event.altKey && !event.shiftKey && event.key.toLowerCase() === "c") {
        if (window.getSelection()?.toString()) return;
        const text = selectionLines(tableScope(), { liveSchedules, liveRuns, liveItems });
        if (text) {
          event.preventDefault();
          writeClipboard(text);
        }
        return;
      }
      if (event.metaKey && !event.altKey && !event.shiftKey && event.key.toLowerCase() === "a" && tableScope()) {
        event.preventDefault();
        selectAll();
        return;
      }
      if (shortcut(event, "r")) { event.preventDefault(); doAction("run"); return; }
      if (shortcut(event, "e")) { event.preventDefault(); doAction("edit"); return; }
      if (shortcut(event, "o")) { event.preventDefault(); doAction("files"); return; }
      if ((event.key === "ArrowDown" || event.key === "ArrowUp") && !event.metaKey && !event.altKey && !event.ctrlKey) {
        event.preventDefault();
        move(event.key === "ArrowDown" ? 1 : -1, event.shiftKey);
      }
      if (event.key === "Enter" && view === "sources") doAction("edit");
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  function toggleCommon(checked) {
    setHideCommon(checked);
    setFocus("");
    try { localStorage.setItem("marchive-hide-common", checked ? "1" : "0"); } catch { /* ignore */ }
  }

  function toggleURL(checked) {
    setHideURL(checked);
    setFocus("");
    try { localStorage.setItem("marchive-hide-url", checked ? "1" : "0"); } catch { /* ignore */ }
  }

  const openContext = useRef(() => {});
  openContext.current = (event) => {
    event.preventDefault();
    const items = buildMenu(event, {
      view,
      doAction,
      removeSelection,
      ensureRow,
      setScope: (kind) => { scope.current = kind; },
      selectAll,
      liveSchedules,
      liveRuns,
      liveItems,
      openReader,
      markRead: markItems,
      isRead: isItemRead,
      selectionText: (kind) => selectionLines(kind, { liveSchedules, liveRuns, liveItems }),
    });
    if (!items?.length) {
      setContext(null);
      return;
    }
    setMenu("");
    setContext({ x: event.clientX, y: event.clientY, items });
  };

  useEffect(() => {
    function onMenu(event) { openContext.current(event); }
    document.addEventListener("contextmenu", onMenu);
    return () => document.removeEventListener("contextmenu", onMenu);
  }, []);

  const editing = editVersion ? editRow.current : null;
  const inSources = view === "sources";
  const inCaptures = view === "captures";
  const pickedSchedules = inSources ? selectedSchedules.filter((row) => sourceVisible(row, view, filter, query)) : [];
  const cancelRuns = selectedRuns.filter((run) => run.status === "queued" || run.status === "running");
  const pickedFileItems = (view === "reader" ? readerIds : (inCaptures ? selectedItems.map((item) => item.id) : [])).map((id) => {
    for (const run of runs) {
      const item = (run.items || []).find((entry) => entry.id === id);
      if (item) return item;
    }
    return null;
  }).filter((item) => item?.downloadLocation);
  const editOn = pickedSchedules.length === 1;
  const sourcesOn = pickedSchedules.length > 0;
  const removeOn = (inSources && sourcesOn) || (inCaptures && (selectedRuns.length > 0 || selectedItems.length > 0)) || (view === "reader" && readerIds.length > 0);
  const activeRun = cancelRuns.length > 0;
  const filesOn = inSources ? pickedSchedules.some((row) => row.schedule.downloadLocation) : pickedFileItems.length > 0;
  const signing = !!(library?.signingIn || library?.browserBusy);
  const sourceRows = sourceGridRows(schedules, view, filter, query, scheduleIds, sourceVisible);
  const runRows = runGridRows(runs, view, filter, query, runIds, runVisible);
  const itemRows = itemGridRows(oneRun, query, itemIds);
  const visibleSources = sourceRows.filter((row) => row.visible);
  const visibleRuns = runRows.filter((row) => row.visible);
  const runnerOn = !!library?.runnerEnabled;

  let status = "Nothing selected";
  if (inSources && pickedSchedules.length === 1) status = `${pickedSchedules[0].name} selected`;
  else if (inSources && pickedSchedules.length > 1) status = `${pickedSchedules.length} sources selected`;
  else if (inCaptures && selectedItems.length > 1) status = `${selectedItems.length} items selected`;
  else if (inCaptures && oneItem) status = "Item selected";
  else if (inCaptures && selectedRuns.length > 1) status = `${selectedRuns.length} captures selected`;
  else if (inCaptures && oneRun) status = "Capture selected";
  else if (view === "reader" && readerIds.length > 1) status = `${readerIds.length} items selected`;
  else if (view === "reader" && oneReader) status = "Item selected";

  return (
    <>
      <Sprite />
      <MenuBar menu={menu} setMenu={setMenu} doAction={doAction} showView={showView} library={library} runnerOn={runnerOn} editOn={editOn} filesOn={filesOn} removeOn={removeOn} sourcesOn={sourcesOn} activeRun={activeRun} signing={signing} />
      <div className="app" onMouseDown={() => { if (menu) setMenu(""); }}>
        <Toolbar query={query} setQuery={setQuery} doAction={doAction} runnerOn={runnerOn} sourcesOn={sourcesOn} activeRun={activeRun} editOn={editOn} filesOn={filesOn} removeOn={removeOn} signing={signing} />
        <p className="banner" role="alert" hidden={!banner}>{banner}</p>
        <div className="workspace">
          <Sidebar view={view} filter={filter} counts={counts} showView={showView} />
          <main className="content">
            <SourcesPane view={view} query={query} sourceRows={sourceRows} visibleSources={visibleSources} orderRef={orderRef} take={take} marks={marks} scope={scope} picked={picked} setScheduleIds={setScheduleIds} openEdit={openEdit} />
            <CapturePane view={view} compact={compactCapture} query={query} visibleRuns={visibleRuns} runRows={runRows} itemRows={itemRows} itemsEmpty={itemsEmptyText(runIds, oneRun, itemRows)} orderRef={orderRef} take={take} marks={marks} scope={scope} picked={picked} setItemIds={setItemIds} act={act} detailTab={detailTab} setDetailTab={setDetailTab} oneItem={oneItem} message={message} itemHold={itemHold} captureCloud={captureCloud} oneRun={oneRun} captureHold={captureHold} itemCloud={itemCloud} hideCommon={hideCommon} hideURL={hideURL} toggleCommon={toggleCommon} toggleURL={toggleURL} />
            <ReaderPane
              view={view}
              groups={readerState.groups}
              shownReader={shownReader}
              readerIds={readerIds}
              query={query}
              readerList={readerList}
              take={take}
              oneReader={oneReader}
              readerMessage={readerMessage}
              readerHold={readerHold}
              sources={readerOpts.sources}
              kinds={readerOpts.kinds}
              source={readerSource}
              kind={readerKind}
              read={readerRead}
              sort={readerSort}
              group={readerGroup}
              filtered={filtersActive}
              onSource={setReaderSource}
              onKind={setReaderKind}
              onRead={setReaderRead}
              onSort={setReaderSort}
              onGroup={setReaderGroup}
              onClear={clearReaderFilters}
            />
            <WordsPane view={view} timeline={timeline} query={query} focus={focus} setFocus={setFocus} hideCommon={hideCommon} hideURL={hideURL} toggleCommon={toggleCommon} toggleURL={toggleURL} />
            <section className={view === "settings" ? "pane is-on" : "pane"}>
              {library ? <Settings library={library} onInstall={() => doAction("chrome-install")} onSave={(body) => act(() => send("POST", "/api/settings", body))} /> : null}
            </section>
          </main>
        </div>
        {offline ? (
          <div className="toast" role="alert">
            <p>Marchive isn't responding. This window will keep trying.</p>
            <button type="button" onClick={() => reload()}>Try again</button>
          </div>
        ) : null}
        <footer className="statusbar">
          <span>{chromeDetail || (library?.chromePath ? "Chrome ready" : library?.chromeError || "")}</span>
          <span>{status}</span>
          <span>{runnerOn ? "Runner on" : "Runner off"}</span>
        </footer>
        <div className="sheet-layer" hidden={!sheet} onMouseDown={(event) => event.stopPropagation()}>
          <AddSheet open={sheet === "add"} folder={library?.defaultFolder || ""} onClose={() => setSheet("")} onSubmit={(body) => act(() => send("POST", "/api/sources", body)).then(() => setSheet(""), () => {})} />
          <EditSheet open={sheet === "edit"} row={editing} onClose={() => setSheet("")} onSave={saveSchedule} onRemove={() => removeSchedule(editing?.id)} />
          <AboutSheet open={sheet === "about"} version={library?.version || ""} onClose={() => setSheet("")} />
          <ConfirmSheet
            open={sheet === "confirm"}
            text={confirm}
            onClose={() => { setSheet(""); confirmAction.current = null; }}
            onYes={(files) => {
              const action = confirmAction.current;
              setSheet("");
              confirmAction.current = null;
              if (action) action(!!files);
            }}
          />
        </div>
      </div>
      <ChromeAlert
        open={chromeOpen}
        onClose={() => {
          sessionStorage.setItem("marchive-chrome-later", "1");
          setChromeOpen(false);
        }}
        onStatus={setChromeDetail}
      />
      <CellTip />
      {context ? <ContextMenu menu={context} onClose={() => setContext(null)} /> : null}
    </>
  );
}
