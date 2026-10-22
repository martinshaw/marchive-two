import { send } from "./api.js";
import { everyParts, intervalSeconds } from "./format.js";

export function bindCommands({
  view, library, reload, showView,
    setMenu, setContext, setSheet, setBanner, setChromeOpen, setChromeHold, setYtDlpOpen, setEditVersion, setConfirm,
  editRow, confirmAction,
  liveSchedules, liveRuns, liveItems,
}) {
  async function act(fn) {
    try {
      await fn();
      setBanner("");
      await reload();
    } catch (err) {
      setBanner(err.message || "Could not save that change.");
      throw err;
    }
  }

  function openEdit(row) {
    const parts = everyParts(row.schedule.intervalSeconds);
    editRow.current = {
      id: row.schedule.id,
      sourceId: row.source.id,
      title: `${row.name} · ${row.source.url}`,
      every: String(parts.count),
      unit: parts.unit,
      maxNew: String(row.schedule.maxNew),
      folder: row.schedule.downloadLocation,
      enabled: row.schedule.enabled,
      kind: row.kind,
      selector: row.source.linkSelector || "",
      capture: row.source.capture || '{"all":true}',
    };
    setEditVersion((n) => n + 1);
    setSheet("edit");
    setMenu("");
  }

  function ask(text, action) {
    confirmAction.current = action;
    setConfirm(text);
    setSheet("confirm");
    setMenu("");
  }

  function removeSelection(kind) {
    const which = kind || (view === "sources" ? "sources" : view === "reader" ? "reader" : (liveItems().length ? "items" : "runs"));
    if (which === "sources") {
      const rows = liveSchedules();
      if (!rows.length) return;
      const ids = [...new Set(rows.map((row) => row.source.id))];
      const text = ids.length === 1 ? "Remove this source?" : `Remove ${ids.length} sources?`;
      ask(text, (files) => act(async () => {
        for (const id of ids) await send("DELETE", `/api/sources/${id}${files ? "?files=1" : ""}`);
      }));
      return;
    }
    if (which === "runs") {
      const rows = liveRuns();
      if (!rows.length) return;
      const text = rows.length === 1 ? "Remove this capture?" : `Remove ${rows.length} captures?`;
      ask(text, (files) => act(async () => {
        for (const run of rows) await send("DELETE", `/api/runs/${run.id}${files ? "?files=1" : ""}`);
      }));
      return;
    }
    const rows = liveItems();
    if (!rows.length) return;
    const text = rows.length === 1 ? "Remove this item?" : `Remove ${rows.length} items?`;
    ask(text, (files) => act(async () => {
      for (const item of rows) await send("DELETE", `/api/items/${item.id}${files ? "?files=1" : ""}`);
    }));
  }

  function doAction(action) {
    setMenu("");
    setContext(null);
    if (action === "add") { setSheet("add"); return; }
    if (action === "about") { setSheet("about"); return; }
    if (action === "chrome-install") {
      sessionStorage.removeItem("marchive-chrome-later");
      setChromeOpen(true);
      return;
    }
    if (action === "ytdlp") {
      setYtDlpOpen(true);
      return;
    }
    if (action === "settings") { showView("settings", ""); return; }
    if (action === "reset") { setSheet("reset"); return; }
    if (action === "runner") {
      act(() => send("POST", "/api/runner", { enabled: !library?.runnerEnabled }));
      return;
    }
    if (action === "signin" || action === "chrome") {
      if (!library?.chromePath) {
        sessionStorage.removeItem("marchive-chrome-later");
        setChromeOpen(true);
        return;
      }
      if (library?.browserBusy || library?.signingIn) {
        setChromeHold(true);
        return;
      }
      const path = action === "chrome" ? "/api/browser/open" : "/api/browser/signin";
      act(() => send("POST", path, {}));
      return;
    }
    if (action === "run" && view === "sources") {
      const ids = liveSchedules().map((row) => row.schedule.id);
      if (!ids.length) return;
      act(async () => {
        for (const id of ids) await send("POST", `/api/schedules/${id}/run`, {});
      });
      return;
    }
    if (action === "cancel") {
      const ids = liveRuns().filter((run) => run.status === "queued" || run.status === "running").map((run) => run.id);
      if (!ids.length) return;
      act(async () => {
        for (const id of ids) await send("POST", `/api/runs/${id}/cancel`, {});
      });
      return;
    }
    if (action === "files") {
      const paths = view === "sources"
        ? liveSchedules().map((row) => row.schedule.downloadLocation)
        : liveItems().map((item) => item.downloadLocation);
      const unique = [...new Set(paths.filter(Boolean))];
      if (unique.length) act(async () => {
        for (const path of unique) await send("POST", "/api/open", { path });
      });
      return;
    }
    if (action === "remove") {
      removeSelection("");
      return;
    }
    if (action === "edit") {
      const rows = liveSchedules();
      if (rows.length === 1) openEdit(rows[0]);
    }
  }

  function saveSchedule(form) {
    setSheet("");
    const body = {
      intervalSeconds: intervalSeconds(form.every, form.unit),
      downloadLocation: form.folder,
      maxNew: Number(form.maxNew),
      enabled: form.enabled,
      capture: form.capture,
    };
    if (form.kind === "article_index") body.linkSelector = form.selector;
    act(() => send("PATCH", `/api/schedules/${form.id}`, body));
  }

  function removeSchedule(id) {
    ask("Remove this schedule?", (files) => act(() => send("DELETE", `/api/schedules/${id}${files ? "?files=1" : ""}`)));
  }

  return { act, openEdit, ask, removeSelection, doAction, saveSchedule, removeSchedule };
}
