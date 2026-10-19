import { useEffect, useState } from "react";
import { captureParts, encodeCapture, parseCapture } from "./format.js";

function captureOptions(stored) {
  return {
    videoQuality: stored.videoQuality || "high",
    audioFormat: stored.audioFormat || "best",
    commentLimit: stored.commentLimit || "all",
  };
}

export function CaptureFields({ kind, capture, onChange, legend = "Capture" }) {
  const stored = parseCapture(capture);
  const parts = captureParts(kind);
  const all = !!stored.all || parts.every((part) => stored[part.key]);
  const options = captureOptions(stored);
  const youtube = kind === "youtube_video";
  const partOn = (key) => all || !!stored[key];
  function emit(nextAll, keys, nextOptions) {
    onChange(encodeCapture(nextAll, keys, nextOptions || options));
  }
  function setAll(checked) {
    emit(checked, parts.map((part) => part.key));
  }
  function toggle(key, checked) {
    const next = {};
    parts.forEach((part) => { next[part.key] = stored.all || !!stored[part.key]; });
    next[key] = checked;
    const keys = parts.map((part) => part.key).filter((part) => next[part]);
    emit(parts.every((part) => next[part.key]), keys);
  }
  function setOption(key, value) {
    const keys = parts.map((part) => part.key).filter((part) => partOn(part));
    emit(all, keys, { ...options, [key]: value });
  }
  const commentAll = options.commentLimit === "all";
  return (
    <fieldset className="capture">
      <legend>{legend}</legend>
      <label className="check">
        <input type="checkbox" checked={all} onChange={(event) => setAll(event.target.checked)} /> All data
      </label>
      <div className="capture-grid">
        {parts.map((part) => (
          <label className="check" key={`${part.key}-${part.label}`}>
            <input type="checkbox" checked={partOn(part.key)} disabled={all} onChange={(event) => toggle(part.key, event.target.checked)} /> {part.label}
          </label>
        ))}
      </div>
      {youtube ? (
        <div className="capture-options">
          <label>Video quality
            <select aria-label="Video quality" value={options.videoQuality} disabled={!partOn("video")} onChange={(event) => setOption("videoQuality", event.target.value)}>
              <option value="low">Low</option>
              <option value="mid">Medium</option>
              <option value="high">High</option>
            </select>
            <span className="hint">Low is about 480p, medium about 720p, and high the largest picture. A separate sound track is combined when ffmpeg is installed.</span>
          </label>
          <label>Audio format
            <select aria-label="Audio format" value={options.audioFormat} disabled={!partOn("audio")} onChange={(event) => setOption("audioFormat", event.target.value)}>
              <option value="best">Best available</option>
              <option value="m4a">M4A</option>
              <option value="opus">Opus</option>
            </select>
          </label>
          <label>Comments
            <select aria-label="How many comments" value={commentAll ? "all" : "count"} disabled={!partOn("comments")} onChange={(event) => setOption("commentLimit", event.target.value === "all" ? "all" : (options.commentLimit === "all" ? "100" : options.commentLimit))}>
              <option value="all">All</option>
              <option value="count">A number</option>
            </select>
          </label>
          {partOn("comments") && !commentAll ? (
            <label>Comment count
              <input type="number" min="1" max="100000" required value={options.commentLimit} onChange={(event) => setOption("commentLimit", event.target.value)} />
            </label>
          ) : null}
        </div>
      ) : null}
    </fieldset>
  );
}

export function EditSheet({ open, row, onClose, onSave, onRemove }) {
  const [form, setForm] = useState(null);
  useEffect(() => {
    if (open && row) setForm({ ...row });
  }, [open, row]);
  if (!open || !form) return null;
  function set(key, value) { setForm((current) => ({ ...current, [key]: value })); }
  return (
    <form
      className="sheet"
      onSubmit={(event) => {
        event.preventDefault();
        onSave(form);
      }}
    >
      <header className="sheet-head"><h2>Edit Schedule</h2></header>
      <div className="sheet-body">
        <p className="sheet-source">{form.title}</p>
        <label hidden={form.kind !== "article_index"}>Article link selector
          <input value={form.selector || ""} placeholder="main a" onChange={(event) => set("selector", event.target.value)} />
          <span className="hint">CSS selector for links on an index page. Leave empty to guess.</span>
        </label>
        <div className="sheet-row">
          <label>Every <input type="number" min="1" required={form.unit !== "once"} disabled={form.unit === "once"} value={form.every} onChange={(event) => set("every", event.target.value)} /></label>
          <label>Unit
            <select value={form.unit} onChange={(event) => set("unit", event.target.value)}>
              <option value="seconds">Seconds</option>
              <option value="minutes">Minutes</option>
              <option value="hours">Hours</option>
              <option value="once">Once</option>
            </select>
          </label>
          <label>New items <input type="number" min="0" max="1000" value={form.maxNew} onChange={(event) => set("maxNew", event.target.value)} /></label>
        </div>
        <label>Archive folder <input required value={form.folder} placeholder="'Marchive' in home directory" onChange={(event) => set("folder", event.target.value)} /></label>
        <CaptureFields kind={form.kind} capture={form.capture} onChange={(capture) => set("capture", capture)} />
        <label className="check"><input type="checkbox" checked={form.enabled} onChange={(event) => set("enabled", event.target.checked)} /> Enabled</label>
      </div>
      <footer className="sheet-foot">
        <button type="button" className="danger-text" onClick={onRemove}>Remove Schedule</button>
        <span className="sheet-spacer" />
        <button type="button" onClick={onClose}>Cancel</button>
        <button type="submit" className="push">Save</button>
      </footer>
    </form>
  );
}

export function AboutSheet({ open, version, onClose }) {
  if (!open) return null;
  return (
    <form className="sheet" onSubmit={(event) => { event.preventDefault(); onClose(); }}>
      <header className="sheet-head"><h2>About Marchive</h2></header>
      <div className="sheet-body prose">
        <p>Version {version}. A local archive of webpages, new articles, and podcast episodes.</p>
        <p>Captures store the page Chrome can already open. Marchive does not try to defeat a paywall.</p>
      </div>
      <footer className="sheet-foot">
        <span className="sheet-spacer" />
        <button type="button" className="push" onClick={onClose}>OK</button>
      </footer>
    </form>
  );
}

export function ConfirmSheet({ open, text, onClose, onYes }) {
  const [files, setFiles] = useState(false);
  useEffect(() => {
    if (open) setFiles(false);
  }, [open, text]);
  if (!open) return null;
  return (
    <form className="sheet confirm" onSubmit={(event) => event.preventDefault()}>
      <header className="sheet-head"><h2>Remove</h2></header>
      <div className="sheet-body">
        <p className="sheet-source">{text}</p>
        <label className="check">
          <input type="checkbox" checked={files} onChange={(event) => setFiles(event.target.checked)} />
          Delete the files
        </label>
      </div>
      <footer className="sheet-foot">
        <span className="sheet-spacer" />
        <button type="button" onClick={onClose}>Cancel</button>
        <button type="button" className="push danger-push" onClick={() => onYes(files)}>Remove</button>
      </footer>
    </form>
  );
}

export function Settings({ library, onSave, onInstall, onYtDlp }) {
  const [form, setForm] = useState(null);
  const [dirty, setDirty] = useState(false);
  useEffect(() => {
    if (!library || dirty) return;
    setForm({
      podcast: library.podcastConcurrency,
      itemTimeout: String(library.itemTimeoutSeconds ?? 0),
      captureTimeout: String(library.captureTimeoutSeconds ?? 0),
      itemTries: String(library.itemTries > 0 ? library.itemTries : 1),
      captureTries: String(library.captureTries > 0 ? library.captureTries : 1),
      mode: library.listenPort > 0 ? "fixed" : "random",
      port: library.listenPort > 0 ? String(library.listenPort) : "",
      open: library.openOnLogin,
      restart: library.restartOnCrash,
    });
  }, [library, dirty]);
  if (!form) return null;
  function set(key, value) {
    setDirty(true);
    setForm((current) => ({ ...current, [key]: value }));
  }
  const fixed = form.mode === "fixed";
  return (
    <div className="prefs">
      <h2>Capture</h2>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          const body = {
            podcastConcurrency: Number(form.podcast),
            itemTimeoutSeconds: Number(form.itemTimeout),
            captureTimeoutSeconds: Number(form.captureTimeout),
            itemTries: Number(form.itemTries),
            captureTries: Number(form.captureTries),
            listenPort: fixed ? Number(form.port) : 0,
            openOnLogin: form.open,
            restartOnCrash: form.restart,
          };
          onSave(body).then(() => setDirty(false)).catch(() => {});
        }}
      >
        <div className="pref-group">
          <label className="pref-row">
            <span>
              <strong>Podcast downloads at once</strong>
              <span className="sub">Episode files downloaded together inside one capture. From 1 to 6.</span>
            </span>
            <input type="number" min="1" max="6" value={form.podcast} onChange={(event) => set("podcast", event.target.value)} />
          </label>
          <label className="pref-row">
            <span>
              <strong>Browser captures at once</strong>
              <span className="sub">Stays at one so captures can share the signed-in Chrome profile.</span>
            </span>
            <input type="number" value="1" readOnly />
          </label>
          <label className="pref-row">
            <span>
              <strong>Item time limit</strong>
              <span className="sub">Seconds for one page, file, or request. 0 means no time limit.</span>
            </span>
            <input type="number" min="0" max="86400" required value={form.itemTimeout} onChange={(event) => set("itemTimeout", event.target.value)} />
          </label>
          <label className="pref-row">
            <span>
              <strong>Capture time limit</strong>
              <span className="sub">Seconds for the whole capture. 0 means no time limit.</span>
            </span>
            <input type="number" min="0" max="86400" required value={form.captureTimeout} onChange={(event) => set("captureTimeout", event.target.value)} />
          </label>
          <label className="pref-row">
            <span>
              <strong>Item attempts</strong>
              <span className="sub">How many times to try one page, file, or request. 1 means a single attempt.</span>
            </span>
            <input type="number" min="1" max="10" required value={form.itemTries} onChange={(event) => set("itemTries", event.target.value)} />
          </label>
          <label className="pref-row">
            <span>
              <strong>Capture attempts</strong>
              <span className="sub">How many times to try the whole capture. 1 means a single attempt.</span>
            </span>
            <input type="number" min="1" max="10" required value={form.captureTries} onChange={(event) => set("captureTries", event.target.value)} />
          </label>
          <label className="pref-row">
            <span>
              <strong>UI port</strong>
              <span className="sub">Random chooses a free localhost port whenever Marchive starts. A fixed port is used the next time it starts, and is the address an installed app opens.{library.activePort > 0 ? ` This session is using ${library.activePort}.` : ""}</span>
            </span>
            <select value={form.mode} onChange={(event) => set("mode", event.target.value)}>
              <option value="random">Random</option>
              <option value="fixed">Fixed</option>
            </select>
          </label>
          <label className="pref-row" hidden={!fixed}>
            <span>
              <strong>Port number</strong>
              <span className="sub">From 1 to 65535, on this computer only.</span>
            </span>
            <input type="number" min="1" max="65535" required={fixed} disabled={!fixed} value={form.port} onChange={(event) => set("port", event.target.value)} />
          </label>
        </div>
        <h2>Startup</h2>
        <div className="pref-group">
          <label className="pref-row">
            <span>
              <strong>Open on login</strong>
              <span className="sub">Starts Marchive when you sign in. macOS uses a LaunchAgent, Linux a systemd user service, and Windows a scheduled task.</span>
            </span>
            <span><input type="checkbox" checked={form.open} onChange={(event) => set("open", event.target.checked)} /></span>
          </label>
          <label className="pref-row">
            <span>
              <strong>Restart on crash</strong>
              <span className="sub">That same service starts Marchive again if it crashes. It applies while the service is running Marchive.</span>
            </span>
            <span><input type="checkbox" checked={form.restart} onChange={(event) => set("restart", event.target.checked)} /></span>
          </label>
          <div className="pref-actions">
            <button type="submit" className="push">Save</button>
          </div>
        </div>
      </form>
      <h2>Chrome</h2>
      <div className="pref-group prose">
        {library.chromePath ? (
          <p>Chrome is available. Sign in opens the Marchive profile stored with this app, then quit Chrome. Captures keep what that window can already open.</p>
        ) : (
          <>
            <p>{library.chromeError}</p>
            <p><button type="button" className="push" onClick={onInstall}>Install Google Chrome</button></p>
          </>
        )}
        {library.signingIn ? <p>Chrome is open on the Marchive profile. Sign in, then quit Chrome.</p> : null}
        {library.browserBusy && !library.signingIn ? <p>Chrome is capturing a page.</p> : null}
      </div>
      <h2>yt-dlp</h2>
      <div className="pref-group prose">
        {library.ytDlpPath ? (
          <p>{library.ytDlpVersion ? `yt-dlp ${library.ytDlpVersion} is ready for YouTube videos.` : "yt-dlp is ready for YouTube videos."} Update replaces it with the current release for this computer.</p>
        ) : (
          <p>YouTube video, audio, subtitles, and comments use yt-dlp. Marchive downloads the current release into its data folder and can replace it later.</p>
        )}
        <p><button type="button" className="push" onClick={onYtDlp}>{library.ytDlpPath ? "Update yt-dlp" : "Install yt-dlp"}</button></p>
      </div>
    </div>
  );
}
