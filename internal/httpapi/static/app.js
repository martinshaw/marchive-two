(function () {
  var state = { view: "sources", filter: "all", query: "", schedule: null, run: null, item: null };
  var columnWidths = {};
  var columnSort = {};
  var suppressClick = false;
  var confirmAction = null;
  var titles = {
    all: ["Sources", "Library"],
    webpage: ["Webpages", "Sources"],
    article_index: ["Articles", "Sources"],
    podcast: ["Podcasts", "Sources"],
    text_extract: ["Text", "Sources"],
    captures: ["Captures", "Library"],
    active: ["In Progress", "Captures"],
    failed: ["Failed", "Captures"],
    succeeded: ["Succeeded", "Captures"],
    settings: ["Settings", "Marchive"]
  };

  function $(id) { return document.getElementById(id); }

  function pageShortcut(event, key) {
    return event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey && event.key.toLowerCase() === key;
  }

  function closeMenus() {
    document.querySelectorAll(".menu.is-open").forEach(function (menu) {
      menu.classList.remove("is-open");
      menu.querySelector(".menu-title").setAttribute("aria-expanded", "false");
      menu.querySelector(".menu-pop").hidden = true;
    });
  }

  function openMenu(menu) {
    closeMenus();
    menu.classList.add("is-open");
    menu.querySelector(".menu-title").setAttribute("aria-expanded", "true");
    menu.querySelector(".menu-pop").hidden = false;
  }

  function openSheet(id) {
    closeMenus();
    $("sheet-layer").hidden = false;
    document.querySelectorAll(".sheet").forEach(function (sheet) { sheet.hidden = true; });
    $(id).hidden = false;
    var field = $(id).querySelector("input, select, button");
    if (field) field.focus();
  }

  function closeSheet() {
    $("sheet-layer").hidden = true;
    document.querySelectorAll(".sheet").forEach(function (sheet) { sheet.hidden = true; });
    confirmAction = null;
  }

  function selectedSource() {
    return document.querySelector("#sources-table tbody tr.is-selected");
  }

  function selectedRun() {
    return document.querySelector("#runs-table tbody tr.is-selected");
  }

  function selectedItem() {
    return document.querySelector("#items-table tbody tr.is-selected");
  }

  function setView(view, filter) {
    state.view = view;
    state.filter = filter || "";
    document.querySelectorAll(".pane").forEach(function (pane) {
      pane.classList.toggle("is-on", pane.dataset.pane === view);
    });
    document.querySelectorAll(".side-item").forEach(function (item) {
      var on = item.dataset.view === view && (item.dataset.filter || "") === (filter || "");
      item.classList.toggle("is-selected", on);
    });
    var key = view === "settings" ? "settings" : (view === "captures" && filter === "all" ? "captures" : (filter || "all"));
    var title = titles[key] || titles.all;
    $("view-title").textContent = title[0];
    $("view-sub").textContent = title[1];
    applyFilters();
    syncToolbar();
    enhanceGrids();
  }

  function visibleRows(table) {
    return Array.prototype.filter.call(table.querySelectorAll("tbody tr"), function (row) {
      return !row.hidden;
    });
  }

  function selectRow(row) {
    if (!row) return;
    var table = row.closest("table");
    table.querySelectorAll("tbody tr.is-selected").forEach(function (other) {
      other.classList.remove("is-selected");
    });
    row.classList.add("is-selected");
    if (table.id === "sources-table") state.schedule = row.dataset.schedule;
    if (table.id === "runs-table") {
      var sameRun = state.run === row.dataset.run;
      state.run = row.dataset.run;
      if (!sameRun) state.item = null;
      showItems(row.dataset.run);
      if (state.item) {
        var itemRow = document.querySelector('#items-table tbody tr[data-item="' + state.item + '"]');
        if (itemRow && !itemRow.hidden) itemRow.classList.add("is-selected");
        else state.item = null;
      }
    }
    if (table.id === "items-table") state.item = row.dataset.item;
    syncToolbar();
  }

  function showItems(runID) {
    var any = false;
    var shown = 0;
    document.querySelectorAll("#items-table tbody tr").forEach(function (row) {
      var mine = row.dataset.run === runID;
      var on = mine && matchesQuery(row);
      row.hidden = !on;
      if (!on) row.classList.remove("is-selected");
      if (mine) any = true;
      if (on) shown++;
    });
    var empty = $("items-empty");
    if (empty) {
      empty.hidden = shown !== 0;
      if (!any) empty.textContent = "This capture has no items.";
      else if (!shown) empty.textContent = "No items match this filter.";
      else empty.textContent = "";
    }
  }

  function matchesQuery(row) {
    var q = (state.query || "").trim().toLowerCase();
    if (!q) return true;
    return rowText(row).toLowerCase().indexOf(q) !== -1;
  }

  function rowText(row) {
    var parts = [];
    Array.prototype.forEach.call(row.children, function (cell) {
      var label = cellLabel(cell);
      if (label) parts.push(label);
    });
    return parts.join(" ");
  }

  function cellLabel(cell) {
    var parts = [];
    Array.prototype.forEach.call(cell.childNodes, function (node) {
      if (node.nodeType === 1 && node.classList.contains("forms")) return;
      var text = (node.textContent || "").replace(/\s+/g, " ").trim();
      if (text) parts.push(text);
    });
    return parts.join("\n");
  }

  function rowMatches(row) {
    if (state.view === "sources") {
      return state.filter === "all" || row.dataset.kind === state.filter;
    }
    if (state.view === "captures") {
      if (!state.filter || state.filter === "all") return true;
      if (state.filter === "active") return row.dataset.status === "queued" || row.dataset.status === "running";
      return row.dataset.status === state.filter;
    }
    return true;
  }

  function applyFilters() {
    var sources = $("sources-table");
    if (sources) {
      var shown = 0;
      sources.querySelectorAll("tbody tr").forEach(function (row) {
        var on = state.view === "sources" && rowMatches(row) && matchesQuery(row);
        row.hidden = !on;
        if (!on) row.classList.remove("is-selected");
        if (on) shown++;
      });
      var keptSource = state.schedule ? sources.querySelector('tbody tr[data-schedule="' + state.schedule + '"]') : null;
      if (!keptSource) state.schedule = null;
      else if (!keptSource.hidden) keptSource.classList.add("is-selected");
      var sourcesEmpty = $("sources-empty");
      if (sourcesEmpty) {
        sourcesEmpty.hidden = shown !== 0;
        sourcesEmpty.textContent = (state.query || "").trim() ? "No sources match this filter." : "No sources in this section.";
      }
    }
    var runs = $("runs-table");
    if (runs) {
      var runShown = 0;
      var keep = null;
      runs.querySelectorAll("tbody tr").forEach(function (row) {
        var on = state.view === "captures" && rowMatches(row) && matchesQuery(row);
        row.hidden = !on;
        if (on) {
          runShown++;
          if (row.dataset.run === state.run) keep = row;
        }
      });
      var runsEmpty = $("runs-empty");
      if (runsEmpty) {
        runsEmpty.hidden = runShown !== 0;
        runsEmpty.textContent = (state.query || "").trim() ? "No captures match this filter." : "No captures in this section.";
      }
      if (state.view === "captures") {
        if (keep) selectRow(keep);
        else {
          state.run = null;
          state.item = null;
          document.querySelectorAll("#items-table tbody tr").forEach(function (row) { row.hidden = true; });
          var itemsEmpty = $("items-empty");
          if (itemsEmpty) {
            itemsEmpty.hidden = false;
            itemsEmpty.textContent = "Select a capture to see its items.";
          }
        }
      }
    }
    recount();
  }

  function recount() {
    var counts = { sources: 0, captures: 0, webpage: 0, article_index: 0, podcast: 0, text_extract: 0, active: 0, failed: 0, succeeded: 0 };
    document.querySelectorAll("#sources-table tbody tr").forEach(function (row) {
      counts.sources++;
      if (counts[row.dataset.kind] !== undefined) counts[row.dataset.kind]++;
    });
    document.querySelectorAll("#runs-table tbody tr").forEach(function (row) {
      counts.captures++;
      if (row.dataset.status === "queued" || row.dataset.status === "running") counts.active++;
      else if (counts[row.dataset.status] !== undefined) counts[row.dataset.status]++;
    });
    document.querySelectorAll("[data-count]").forEach(function (node) {
      var n = counts[node.dataset.count] || 0;
      node.textContent = n ? String(n) : "";
    });
  }

  function syncRunner(on, signing, busy) {
    var button = $("tool-runner");
    var label = $("tool-runner-label");
    var menu = document.querySelector('[data-action="runner"]');
    if (button && label) {
      button.value = on ? "false" : "true";
      label.textContent = on ? "Stop" : "Start";
      var play = button.querySelector(".icon-start");
      var stop = button.querySelector(".icon-stop");
      if (play) play.hidden = on;
      if (stop) stop.hidden = !on;
    }
    if (menu) menu.textContent = on ? "Stop Runner" : "Start Runner";
    var text = on ? "Runner on" : "Runner off";
    if ($("menu-runner")) $("menu-runner").textContent = text;
    if ($("status-runner")) $("status-runner").textContent = text;
    var signin = $("tool-signin");
    if (signin) signin.disabled = signing || busy;
    var signinMenu = document.querySelector('[data-action="signin"]');
    if (signinMenu) signinMenu.disabled = signing || busy;
  }

  function syncToolbar() {
    var source = selectedSource();
    var run = selectedRun();
    var item = selectedItem();
    var inSources = state.view === "sources";
    var inCaptures = state.view === "captures";
    var active = run && (run.dataset.status === "queued" || run.dataset.status === "running");
    setEnabled("run", inSources && !!source);
    setEnabled("edit", inSources && !!source);
    setEnabled("remove", (inSources && !!source) || false);
    setEnabled("cancel", inCaptures && !!active);
    var files = false;
    if (inSources && source) files = true;
    if (inCaptures && item && item.querySelector("form[id^='item-open-']")) files = true;
    setEnabled("files", files);
    var status = $("status-selection");
    if (!status) return;
    if (inSources && source) status.textContent = (source.dataset.name || "Untitled") + " selected";
    else if (inCaptures && item) status.textContent = "Item selected";
    else if (inCaptures && run) status.textContent = "Capture selected";
    else status.textContent = "Nothing selected";
  }

  function setEnabled(action, on) {
    document.querySelectorAll('[data-action="' + action + '"]').forEach(function (node) {
      if (node.classList.contains("tool") || node.getAttribute("role") === "menuitem") node.disabled = !on;
    });
  }

  function submitForm(id) {
    var form = $(id);
    if (form) postForm(form);
  }

  function showBanner(message) {
    var banner = $("banner");
    if (!banner) return;
    banner.hidden = !message;
    banner.textContent = message || "";
  }

  var refreshGen = 0;

  function snapshotScroll() {
    var saved = [];
    document.querySelectorAll(".table-scroll, .sidebar").forEach(function (el) {
      saved.push({ el: el, top: el.scrollTop, left: el.scrollLeft });
    });
    return saved;
  }

  function restoreScroll(saved) {
    saved.forEach(function (item) {
      if (!item.el.isConnected) return;
      item.el.scrollTop = item.top;
      item.el.scrollLeft = item.left;
    });
  }

  function replaceRows(body, html) {
    if (!body || body.innerHTML === html) return false;
    body.innerHTML = html;
    return true;
  }

  function applyLibrary(html) {
    var doc = new DOMParser().parseFromString(html, "text/html");
    var scrolls = snapshotScroll();
    var changed = false;
    var sourceRows = doc.querySelector("#library-sources tbody");
    var sourceBody = document.querySelector("#sources-table tbody");
    if (sourceRows && replaceRows(sourceBody, sourceRows.innerHTML)) changed = true;
    ["runs-table", "items-table"].forEach(function (id) {
      var next = doc.querySelector("#" + id + " tbody");
      var current = document.querySelector("#" + id + " tbody");
      if (next && replaceRows(current, next.innerHTML)) changed = true;
    });
    var nextPulse = doc.querySelector("#pulse");
    var pulse = $("pulse");
    if (nextPulse && pulse) {
      pulse.dataset.runner = nextPulse.dataset.runner || "";
      pulse.dataset.signing = nextPulse.dataset.signing || "";
      pulse.dataset.busy = nextPulse.dataset.busy || "";
    }
    if (changed) {
      enhanceGrids();
      applyFilters();
    }
    restoreScroll(scrolls);
    readPulse();
    syncToolbar();
  }

  async function refreshLibrary() {
    var gen = ++refreshGen;
    try {
      var response = await fetch("/fragment/library", { cache: "no-store" });
      if (!response.ok || gen !== refreshGen) return;
      var html = await response.text();
      if (gen !== refreshGen) return;
      applyLibrary(html);
    } catch (err) {
      /* Keep the tables that are already on screen. */
    }
  }

  async function postForm(form, submitter) {
    var action = form.action;
    if (submitter && submitter.getAttribute("formaction")) action = submitter.formAction;
    var body = submitter ? new FormData(form, submitter) : new FormData(form);
    var response;
    try {
      response = await fetch(action, {
        method: "POST",
        body: body,
        headers: { "Accept": "application/json" }
      });
    } catch (err) {
      showBanner("Could not save that change.");
      return;
    }
    var data = {};
    var type = response.headers.get("Content-Type") || "";
    if (type.indexOf("json") !== -1) {
      try { data = await response.json(); } catch (err) { data = {}; }
    }
    if (!response.ok) {
      showBanner(data.error || "Could not save that change.");
      return;
    }
    showBanner("");
    if (form.id === "sheet-add") {
      form.reset();
      syncSelector();
      closeSheet();
    } else if (form.id === "sheet-edit") {
      closeSheet();
    }
    await refreshLibrary();
  }

  function askRemove() {
    var source = selectedSource();
    if (!source) return;
    $("confirm-text").textContent = "Remove this source and its schedules? Files stay on disk.";
    confirmAction = function () {
      submitForm("del-source-" + source.dataset.source + "-" + source.dataset.schedule);
    };
    openSheet("sheet-confirm");
  }

  function doAction(action) {
    closeMenus();
    if (action === "add") { openSheet("sheet-add"); return; }
    if (action === "chrome-install") { showChromeAlert(); return; }
    if (action === "settings") { setView("settings", ""); return; }
    if (action === "runner") { $("runner-form").requestSubmit($("tool-runner")); return; }
    if (action === "signin") {
      if (document.body.dataset.chromeMissing === "1") {
        showChromeAlert();
        return;
      }
      $("signin-form").requestSubmit();
      return;
    }
    if (action === "run") {
      var source = selectedSource();
      if (source) submitForm("run-" + source.dataset.schedule);
      return;
    }
    if (action === "files") {
      var item = selectedItem();
      if (state.view === "captures" && item) {
        submitForm("item-open-" + item.dataset.item);
        return;
      }
      var selected = selectedSource();
      if (selected) submitForm("open-" + selected.dataset.schedule);
      return;
    }
    if (action === "remove") { askRemove(); return; }
    if (action === "cancel") {
      var run = selectedRun();
      if (run) submitForm("cancel-" + run.dataset.run);
      return;
    }
    if (action === "edit") {
      var row = selectedSource();
      if (!row) return;
      var form = $("sheet-edit");
      form.action = "/schedules/" + row.dataset.schedule;
      $("edit-remove-schedule").formAction = "/schedules/" + row.dataset.schedule + "/delete";
      $("edit-source").textContent = (row.dataset.name || "Untitled") + " · " + row.dataset.url;
      $("edit-every").value = row.dataset.everyCount;
      $("edit-unit").value = row.dataset.everyUnit;
      $("edit-max").value = row.dataset.maxNew;
      $("edit-folder").value = row.dataset.folder;
      $("edit-enabled").checked = row.dataset.enabled === "true";
      var kindField = $("edit-kind");
      if (kindField) kindField.value = row.dataset.kind || "webpage";
      applyCapture(form, row.dataset.kind || "webpage", row.dataset.capture || "");
      openSheet("sheet-edit");
    }
  }

  function moveSelection(delta) {
    var table = state.view === "sources" ? $("sources-table") : $("runs-table");
    if (!table || state.view === "settings") return;
    var rows = visibleRows(table);
    if (!rows.length) return;
    var current = table.querySelector("tbody tr.is-selected");
    var index = rows.indexOf(current);
    var next = rows[Math.max(0, Math.min(rows.length - 1, index + delta))];
    if (index === -1) next = delta < 0 ? rows[rows.length - 1] : rows[0];
    selectRow(next);
    next.scrollIntoView({ block: "nearest" });
  }

  function syncPortMode() {
    var mode = $("port-mode");
    var row = $("port-number-row");
    var input = $("listen-port");
    if (!mode || !input) return;
    var fixed = mode.value === "fixed";
    if (row) row.hidden = !fixed;
    input.disabled = !fixed;
    input.required = fixed;
  }

  function syncSelector() {
    var provider = $("provider");
    var selector = $("selector-field");
    if (!provider || !selector) return;
    selector.hidden = provider.value !== "article_index";
    applyCapture($("sheet-add"), provider.value, "");
  }

  function applyCapture(form, kind, raw) {
    if (!form) return;
    var stored = { all: true };
    if (raw) {
      try { stored = JSON.parse(raw); } catch (err) { stored = { all: true }; }
    }
    if (raw === "") stored = { all: true };
    form.querySelectorAll("[data-part]").forEach(function (label) {
      var kinds = (label.dataset.kinds || "").split(" ");
      var shown = kinds.indexOf(kind) !== -1;
      label.hidden = !shown;
      var input = label.querySelector("input");
      if (!input) return;
      input.disabled = !shown;
      if (shown) input.checked = !!(stored.all || stored[input.value]);
    });
    var all = form.querySelector("[data-capture-all]");
    if (!all) return;
    var visible = Array.prototype.filter.call(form.querySelectorAll("[data-part]"), function (label) { return !label.hidden; });
    var every = visible.length > 0 && visible.every(function (label) { return label.querySelector("input").checked; });
    all.checked = !!(stored.all || every);
    visible.forEach(function (label) {
      var input = label.querySelector("input");
      input.disabled = all.checked;
      if (all.checked) input.checked = true;
    });
  }

  function bindCapture(form) {
    if (!form) return;
    var all = form.querySelector("[data-capture-all]");
    if (all) {
      all.addEventListener("change", function () {
        form.querySelectorAll("[data-part]").forEach(function (label) {
          if (label.hidden) return;
          var input = label.querySelector("input");
          input.disabled = all.checked;
          if (all.checked) input.checked = true;
        });
      });
    }
    form.querySelectorAll("[data-part] input").forEach(function (input) {
      input.addEventListener("change", function () {
        var visible = Array.prototype.filter.call(form.querySelectorAll("[data-part]"), function (label) { return !label.hidden; });
        var every = visible.every(function (label) { return label.querySelector("input").checked; });
        if (all) {
          all.checked = every;
          visible.forEach(function (label) { label.querySelector("input").disabled = every; });
        }
      });
    });
  }

  function readPulse() {
    var pulse = $("pulse");
    if (!pulse) return;
    syncRunner(pulse.dataset.runner === "on", pulse.dataset.signing === "1", pulse.dataset.busy === "1");
  }

  function sortValue(cell) {
    if (!cell) return { empty: true, num: null, text: "" };
    var raw = cell.dataset.sort != null ? cell.dataset.sort : cellLabel(cell).replace(/\n/g, " ");
    raw = String(raw).trim();
    if (!raw) return { empty: true, num: null, text: "" };
    if (/^-?\d+(\.\d+)?$/.test(raw)) return { empty: false, num: Number(raw), text: raw };
    return { empty: false, num: null, text: raw.toLowerCase() };
  }

  function applySort(table) {
    var spec = columnSort[table.id];
    if (!spec) return;
    var body = table.querySelector("tbody");
    if (!body) return;
    var rows = Array.prototype.slice.call(body.querySelectorAll("tr"));
    rows.sort(function (a, b) {
      var av = sortValue(a.children[spec.index]);
      var bv = sortValue(b.children[spec.index]);
      if (av.empty !== bv.empty) return av.empty ? 1 : -1;
      var cmp = 0;
      if (av.num != null && bv.num != null) cmp = av.num - bv.num;
      else cmp = av.text.localeCompare(bv.text, undefined, { numeric: true, sensitivity: "base" });
      if (!cmp) {
        cmp = (a.dataset.schedule || a.dataset.run || a.dataset.item || "").localeCompare(b.dataset.schedule || b.dataset.run || b.dataset.item || "", undefined, { numeric: true });
      }
      return cmp * spec.dir;
    });
    rows.forEach(function (row) { body.appendChild(row); });
  }

  function paintSort(table) {
    var spec = columnSort[table.id];
    table.querySelectorAll("thead th").forEach(function (th, index) {
      var on = !!(spec && spec.index === index);
      th.classList.toggle("is-sorted", on);
      th.setAttribute("aria-sort", on ? (spec.dir < 0 ? "descending" : "ascending") : "none");
    });
  }

  function toggleSort(table, index) {
    var spec = columnSort[table.id];
    if (spec && spec.index === index) spec.dir = -spec.dir;
    else columnSort[table.id] = { index: index, dir: 1 };
    applySort(table);
    paintSort(table);
  }

  function dressHeader(table) {
    table.querySelectorAll("thead th").forEach(function (th) {
      if (th.querySelector(".col-sort")) return;
      var label = th.textContent.trim();
      th.textContent = "";
      var button = document.createElement("button");
      button.type = "button";
      button.className = "col-sort";
      button.textContent = label;
      var mark = document.createElement("span");
      mark.className = "col-mark";
      mark.setAttribute("aria-hidden", "true");
      button.appendChild(mark);
      var handle = document.createElement("span");
      handle.className = "col-resize";
      handle.setAttribute("role", "separator");
      handle.setAttribute("aria-orientation", "vertical");
      handle.setAttribute("aria-label", "Resize " + label);
      th.appendChild(button);
      th.appendChild(handle);
      th.setAttribute("aria-sort", "none");
    });
  }

  function ensureCols(table, count) {
    var group = table.querySelector("colgroup");
    if (!group) {
      group = document.createElement("colgroup");
      table.insertBefore(group, table.firstChild);
    }
    while (group.children.length < count) group.appendChild(document.createElement("col"));
    while (group.children.length > count) group.removeChild(group.lastChild);
    return group;
  }

  function writeWidths(table, list) {
    columnWidths[table.id] = list.map(function (n) { return Math.max(56, Math.round(n)); });
    var group = ensureCols(table, list.length);
    var sum = 0;
    columnWidths[table.id].forEach(function (width, index) {
      group.children[index].style.width = width + "px";
      sum += width;
    });
    table.style.width = sum + "px";
  }

  function measureColumns(table) {
    if (columnWidths[table.id]) return columnWidths[table.id];
    var parent = table.closest(".table-scroll");
    if (!parent || parent.clientWidth < 80) return null;
    var headers = table.querySelectorAll("thead th");
    if (!headers.length) return null;
    var next = [];
    var sum = 0;
    Array.prototype.forEach.call(headers, function (th) {
      var width = Math.max(56, Math.round(th.getBoundingClientRect().width));
      next.push(width);
      sum += width;
    });
    var extra = parent.clientWidth - sum;
    if (extra > 1) {
      var grow = next.length - 1;
      Array.prototype.forEach.call(headers, function (th, index) {
        if (th.classList.contains("col-name")) grow = index;
      });
      next[grow] += extra;
    }
    return next;
  }

  function enhanceGrids() {
    document.querySelectorAll("table.grid").forEach(function (table) {
      dressHeader(table);
      var widths = measureColumns(table);
      if (widths) writeWidths(table, widths);
      applySort(table);
      paintSort(table);
    });
  }

  function overflows(el) {
    return el.scrollWidth > el.clientWidth + 1 || el.scrollHeight > el.clientHeight + 1;
  }

  function cellOverflows(cell) {
    if (overflows(cell)) return true;
    var nodes = cell.querySelectorAll(".primary, .sub, a, .fault, .pill");
    for (var i = 0; i < nodes.length; i++) {
      if (overflows(nodes[i])) return true;
    }
    return false;
  }

  var tipTimer = 0;
  function hideTip() {
    window.clearTimeout(tipTimer);
    tipTimer = 0;
    var tip = $("cell-tip");
    if (tip) tip.hidden = true;
  }

  function showTip(cell, x, y) {
    var tip = $("cell-tip");
    var text = cellLabel(cell);
    if (!tip || !text || !cellOverflows(cell)) {
      hideTip();
      return;
    }
    tip.textContent = text;
    tip.hidden = false;
    var left = x + 12;
    var top = y + 16;
    tip.style.left = "0px";
    tip.style.top = "0px";
    var box = tip.getBoundingClientRect();
    if (left + box.width > window.innerWidth - 8) left = window.innerWidth - box.width - 8;
    if (top + box.height > window.innerHeight - 8) top = y - box.height - 10;
    tip.style.left = Math.max(8, left) + "px";
    tip.style.top = Math.max(8, top) + "px";
  }

  document.addEventListener("click", function (event) {
    if (suppressClick) {
      suppressClick = false;
      return;
    }
    var sortButton = event.target.closest(".col-sort");
    if (sortButton) {
      event.preventDefault();
      var header = sortButton.closest("th");
      var headers = sortButton.closest("tr").children;
      toggleSort(sortButton.closest("table"), Array.prototype.indexOf.call(headers, header));
      return;
    }
    if (event.target.closest("#chrome-alert") && !event.target.closest("[data-action='chrome-install']")) return;
    if (event.target.closest("[data-action='chrome-install']")) {
      event.preventDefault();
      closeMenus();
      showChromeAlert();
      return;
    }
    var menuTitle = event.target.closest(".menu-title");
    if (menuTitle) {
      var menu = menuTitle.closest(".menu");
      if (menu.classList.contains("is-open")) closeMenus();
      else openMenu(menu);
      return;
    }
    var side = event.target.closest(".side-item");
    if (side) {
      setView(side.dataset.view, side.dataset.filter || "");
      return;
    }
    var menuItem = event.target.closest(".menu-pop [data-action], .menu-pop [data-view]");
    if (menuItem) {
      if (menuItem.dataset.view) setView(menuItem.dataset.view, menuItem.dataset.filter || "all");
      else doAction(menuItem.dataset.action);
      return;
    }
    var tool = event.target.closest(".tool[data-action]");
    if (tool && !tool.disabled) {
      event.preventDefault();
      doAction(tool.dataset.action);
      return;
    }
    var row = event.target.closest("#sources-table tbody tr, #runs-table tbody tr, #items-table tbody tr");
    if (row && !row.hidden) {
      selectRow(row);
      return;
    }
    if (!event.target.closest(".menu")) closeMenus();
  });

  document.addEventListener("dblclick", function (event) {
    var source = event.target.closest("#sources-table tbody tr");
    if (source && !source.hidden) { selectRow(source); doAction("edit"); return; }
    var item = event.target.closest("#items-table tbody tr");
    if (item && !item.hidden && item.querySelector("form")) {
      selectRow(item);
      doAction("files");
    }
  });

  document.addEventListener("mouseover", function (event) {
    var title = event.target.closest(".menu-title");
    if (!title) return;
    if (document.querySelector(".menu.is-open")) openMenu(title.closest(".menu"));
  });

  document.addEventListener("keydown", function (event) {
    var typing = event.target.closest("input, select, textarea");
    if (chromeOpen()) {
      if (event.key === "Escape" && chromePhase !== "downloading" && chromePhase !== "installing") dismissChrome();
      event.preventDefault();
      return;
    }
    if (event.key === "Escape") {
      var filterBox = $("table-filter");
      if (filterBox && event.target === filterBox && filterBox.value) {
        filterBox.value = "";
        state.query = "";
        applyFilters();
        return;
      }
      closeMenus();
      closeSheet();
      return;
    }
    if (pageShortcut(event, "n")) { event.preventDefault(); doAction("add"); return; }
    if (pageShortcut(event, ",")) { event.preventDefault(); setView("settings", ""); return; }
    if (pageShortcut(event, "1")) { event.preventDefault(); setView("sources", "all"); return; }
    if (pageShortcut(event, "2")) { event.preventDefault(); setView("captures", "all"); return; }
    if (typing) return;
    if (pageShortcut(event, "r")) { event.preventDefault(); doAction("run"); return; }
    if (pageShortcut(event, "e")) { event.preventDefault(); doAction("edit"); return; }
    if (pageShortcut(event, "o")) { event.preventDefault(); doAction("files"); return; }
    if (event.key === "ArrowDown") { event.preventDefault(); moveSelection(1); }
    if (event.key === "ArrowUp") { event.preventDefault(); moveSelection(-1); }
    if (event.key === "Enter" && state.view === "sources") doAction("edit");
  });

  document.querySelectorAll("[data-sheet-close]").forEach(function (button) {
    button.addEventListener("click", closeSheet);
  });
  $("confirm-yes").addEventListener("click", function () {
    var action = confirmAction;
    closeSheet();
    if (action) action();
  });
  $("edit-remove-schedule").addEventListener("click", function (event) {
    event.preventDefault();
    var source = selectedSource();
    if (!source) return;
    $("confirm-text").textContent = "Remove this schedule? Files stay on disk.";
    confirmAction = function () { submitForm("del-schedule-" + source.dataset.schedule); };
    openSheet("sheet-confirm");
  });
  var provider = $("provider");
  if (provider) provider.addEventListener("change", syncSelector);
  var portMode = $("port-mode");
  if (portMode) portMode.addEventListener("change", syncPortMode);
  syncSelector();
  syncPortMode();
  bindCapture($("sheet-add"));
  bindCapture($("sheet-edit"));

  var filterBox = $("table-filter");
  if (filterBox) {
    filterBox.addEventListener("input", function () {
      state.query = filterBox.value;
      applyFilters();
    });
  }

  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (!form || form.tagName !== "FORM") return;
    if ((form.getAttribute("method") || "").toLowerCase() !== "post") return;
    event.preventDefault();
    postForm(form, event.submitter);
  });

  document.addEventListener("pointerdown", function (event) {
    var handle = event.target.closest(".col-resize");
    if (!handle) return;
    event.preventDefault();
    var th = handle.closest("th");
    var table = th.closest("table");
    var headers = Array.prototype.slice.call(table.querySelectorAll("thead th"));
    var index = headers.indexOf(th);
    var startX = event.clientX;
    var start = headers.map(function (cell) { return cell.getBoundingClientRect().width; });
    var moved = false;
    function move(ev) {
      var dx = ev.clientX - startX;
      if (Math.abs(dx) > 2) moved = true;
      var next = start.slice();
      var width = Math.max(56, start[index] + dx);
      var delta = width - start[index];
      next[index] = width;
      if (index + 1 < next.length) {
        var neighbor = start[index + 1] - delta;
        if (neighbor < 56) {
          next[index] = start[index] + (start[index + 1] - 56);
          next[index + 1] = 56;
        } else next[index + 1] = neighbor;
      }
      writeWidths(table, next);
    }
    function up() {
      if (moved) suppressClick = true;
      document.body.classList.remove("is-resizing");
      document.removeEventListener("pointermove", move);
      document.removeEventListener("pointerup", up);
    }
    document.body.classList.add("is-resizing");
    document.addEventListener("pointermove", move);
    document.addEventListener("pointerup", up);
  });

  document.addEventListener("mouseover", function (event) {
    var cell = event.target.closest("table.grid tbody td");
    window.clearTimeout(tipTimer);
    if (!cell) {
      hideTip();
      return;
    }
    var x = event.clientX;
    var y = event.clientY;
    tipTimer = window.setTimeout(function () { showTip(cell, x, y); }, 350);
  });
  document.addEventListener("mouseout", function (event) {
    if (event.target.closest && event.target.closest("table.grid tbody td")) hideTip();
  });
  document.addEventListener("scroll", hideTip, true);

  function scheduleRefresh() {
    window.setTimeout(function () {
      if (!document.hidden) refreshLibrary();
      scheduleRefresh();
    }, document.hidden ? 15000 : 2000);
  }
  document.addEventListener("visibilitychange", function () {
    if (!document.hidden) refreshLibrary();
  });
  scheduleRefresh();

  setView("sources", "all");
  readPulse();
  recount();
  syncToolbar();
  if (document.body.dataset.chromeMissing === "1" && sessionStorage.getItem("marchive-chrome-later") !== "1") {
    showChromeAlert();
  }
})();

var chromeTimer = 0;
var chromePhase = "idle";
var chromeGen = 0;

function chromeOpen() {
  var layer = document.getElementById("chrome-alert");
  return layer && !layer.hidden;
}

function showChromeAlert() {
  sessionStorage.removeItem("marchive-chrome-later");
  var layer = document.getElementById("chrome-alert");
  if (!layer) return;
  layer.hidden = false;
  pollChrome();
}

function dismissChrome() {
  if (chromePhase === "downloading" || chromePhase === "installing") return;
  sessionStorage.setItem("marchive-chrome-later", "1");
  var layer = document.getElementById("chrome-alert");
  if (layer) layer.hidden = true;
  if (chromeTimer) {
    window.clearTimeout(chromeTimer);
    chromeTimer = 0;
  }
}

function mb(n) {
  if (!n || n < 0) return "0 MB";
  return (n / (1024 * 1024)).toFixed(1) + " MB";
}

function renderChrome(data) {
  chromePhase = data.phase || "idle";
  var title = document.getElementById("chrome-alert-title");
  var text = document.getElementById("chrome-alert-text");
  var progress = document.getElementById("chrome-progress");
  var bar = document.getElementById("chrome-progress-bar");
  var label = document.getElementById("chrome-progress-label");
  var later = document.getElementById("chrome-later");
  var install = document.getElementById("chrome-install");
  var cancel = document.getElementById("chrome-cancel");
  var ok = document.getElementById("chrome-ok");
  var working = chromePhase === "downloading" || chromePhase === "installing";
  if (title) {
    title.textContent = chromePhase === "done" ? "Google Chrome is installed" : (chromePhase === "failed" ? "Google Chrome could not be installed" : (chromePhase === "downloading" ? "Downloading Google Chrome" : (chromePhase === "installing" ? "Installing Google Chrome" : "Google Chrome is required")));
  }
  if (text) {
    text.textContent = data.error || data.message || "Webpage, article, and text captures use Google Chrome. Marchive can download the current stable version and install it.";
  }
  if (progress && bar) {
    var known = data.total > 0 && chromePhase === "downloading";
    var busy = chromePhase === "installing" || (working && !known);
    progress.hidden = !(working || chromePhase === "done");
    progress.classList.toggle("is-busy", busy);
    if (busy) bar.style.width = "";
    else if (chromePhase === "done") bar.style.width = "100%";
    else if (data.total > 0) bar.style.width = Math.max(0, Math.min(100, (data.received / data.total) * 100)) + "%";
    else bar.style.width = "0%";
  }
  if (label) {
    if (chromePhase === "downloading" && data.total > 0) label.textContent = "Downloading… " + mb(data.received) + " of " + mb(data.total);
    else if (working) label.textContent = data.message || "";
    else label.textContent = "";
  }
  if (later) later.hidden = working || chromePhase === "done";
  if (install) install.hidden = working || chromePhase === "done" || chromePhase === "failed";
  if (cancel) cancel.hidden = !working;
  if (ok) ok.hidden = chromePhase !== "done" && chromePhase !== "failed" && chromePhase !== "cancelled";
  if (ok && chromePhase === "failed") ok.textContent = "Try Again";
  if (ok && chromePhase === "done") ok.textContent = "OK";
  if (ok && chromePhase === "cancelled") ok.textContent = "OK";
  var status = document.getElementById("status-detail");
  if (status && (working || chromePhase === "failed" || chromePhase === "done" || chromePhase === "cancelled")) {
    status.textContent = data.error || data.message || "";
  }
}

async function pollChrome() {
  if (!chromeOpen()) return;
  var gen = ++chromeGen;
  try {
    var response = await fetch("/api/chrome", { cache: "no-store" });
    if (!response.ok || gen !== chromeGen || !chromeOpen()) return;
    renderChrome(await response.json());
  } catch (err) {
    /* Keep the dialog up and try again. */
  }
  if (gen === chromeGen && chromeOpen() && (chromePhase === "downloading" || chromePhase === "installing")) {
    chromeTimer = window.setTimeout(pollChrome, 400);
  }
}

document.getElementById("chrome-later").addEventListener("click", dismissChrome);
document.getElementById("chrome-ok").addEventListener("click", function () {
  if (chromePhase === "done") {
    window.location.reload();
    return;
  }
  chromePhase = "idle";
  renderChrome({ phase: "idle", message: "Webpage, article, and text captures use Google Chrome. Marchive can download the current stable version and install it." });
  document.getElementById("chrome-alert").hidden = false;
});
document.getElementById("chrome-cancel").addEventListener("click", async function () {
  await fetch("/api/chrome/install/cancel", { method: "POST" });
  pollChrome();
});
document.getElementById("chrome-install").addEventListener("click", async function () {
  chromeGen++;
  renderChrome({ phase: "downloading", received: 0, total: 0, message: "Downloading Google Chrome…" });
  var response = await fetch("/api/chrome/install", { method: "POST" });
  if (!response.ok) {
    var body = {};
    try { body = await response.json(); } catch (err) { body = {}; }
    renderChrome({ phase: "failed", error: body.error || "Could not start the install." });
    return;
  }
  pollChrome();
});
var signinForm = document.getElementById("signin-form");
if (signinForm) {
  signinForm.addEventListener("submit", function (event) {
    if (document.body.dataset.chromeMissing === "1") {
      event.preventDefault();
      showChromeAlert();
    }
  });
}
