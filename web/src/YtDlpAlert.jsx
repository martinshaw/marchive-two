import { useEffect, useRef, useState } from "react";

function mb(n) {
  if (!n || n < 0) return "0 MB";
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function YtDlpStep({ active, onReady }) {
  const [data, setData] = useState({
    phase: "idle",
    message: "YouTube video, audio, subtitles, and comments need yt-dlp. Marchive downloads the current release into its data folder before this source is saved.",
  });
  const told = useRef(false);

  useEffect(() => {
    if (!active) return undefined;
    let stop = false;
    let timer = 0;
    async function poll() {
      try {
        const response = await fetch("/api/ytdlp", { cache: "no-store" });
        if (!response.ok || stop) return;
        const next = await response.json();
        if (stop) return;
        setData(next);
        if ((next.installed || next.phase === "done") && !told.current) {
          told.current = true;
          onReady();
        }
        const phase = next.phase || "idle";
        if (phase === "downloading" || phase === "installing") timer = window.setTimeout(poll, 400);
      } catch {
        /* Keep the step up and try again. */
      }
    }
    poll();
    return () => {
      stop = true;
      window.clearTimeout(timer);
    };
  }, [active, data.phase, onReady]);

  const phase = data.phase || "idle";
  const working = phase === "downloading" || phase === "installing";
  const ready = !!(data.installed || phase === "done");
  const known = data.total > 0 && phase === "downloading";
  const busy = phase === "installing" || (working && !known);
  let bar = "0%";
  if (busy) bar = "";
  else if (ready) bar = "100%";
  else if (data.total > 0) bar = `${Math.max(0, Math.min(100, (data.received / data.total) * 100))}%`;
  let label = "";
  if (phase === "downloading" && data.total > 0) label = `Downloading… ${mb(data.received)} of ${mb(data.total)}`;
  else if (working) label = data.message || "";
  else if (ready) label = data.version ? `yt-dlp ${data.version} is ready.` : "yt-dlp is ready.";

  async function install() {
    setData({ phase: "downloading", received: 0, total: 0, message: "Downloading yt-dlp…" });
    const response = await fetch("/api/ytdlp/install", { method: "POST" });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      setData({ phase: "failed", error: body.error || "Could not start the download." });
      return;
    }
    setData(await response.json());
  }

  return (
    <div className="sheet-body tool-step">
      <h3>yt-dlp</h3>
      <p>{data.error || (ready ? "YouTube files can be saved with this source. Continue to its address and schedule." : "YouTube video, audio, subtitles, and comments need yt-dlp. Marchive downloads the current release into its data folder before this source is saved.")}</p>
      <div className={`progress${busy ? " is-busy" : ""}`} hidden={!(working || ready)}>
        <div className="progress-bar" style={bar ? { width: bar } : undefined} />
      </div>
      <p className="progress-label">{label}</p>
      <div className="tool-actions">
        <button type="button" className="push" hidden={working || ready} onClick={install}>{phase === "failed" || phase === "cancelled" ? "Try again" : "Install yt-dlp"}</button>
        <button
          type="button"
          hidden={!working}
          onClick={async () => {
            await fetch("/api/ytdlp/install/cancel", { method: "POST" });
            const response = await fetch("/api/ytdlp", { cache: "no-store" });
            if (response.ok) setData(await response.json());
          }}
        >
          Cancel download
        </button>
      </div>
    </div>
  );
}

export function YtDlpAlert({ open, update, onClose, onStatus }) {
  const [data, setData] = useState({
    phase: "idle",
    message: "YouTube captures use yt-dlp. Marchive can download the current release and replace it when you ask.",
  });

  useEffect(() => {
    if (!open) return undefined;
    let stop = false;
    let timer = 0;
    async function poll() {
      try {
        const response = await fetch("/api/ytdlp", { cache: "no-store" });
        if (!response.ok || stop) return;
        const next = await response.json();
        if (stop) return;
        setData(next);
        const phase = next.phase || "idle";
        if (phase === "downloading" || phase === "failed" || phase === "done" || phase === "cancelled" || phase === "installing") {
          onStatus(next.error || next.message || "");
        }
        if (phase === "downloading" || phase === "installing") timer = window.setTimeout(poll, 400);
      } catch {
        /* Keep the dialog up and try again. */
      }
    }
    poll();
    return () => {
      stop = true;
      window.clearTimeout(timer);
    };
  }, [open, data.phase, onStatus]);

  if (!open) return null;
  const phase = data.phase || "idle";
  const working = phase === "downloading" || phase === "installing";
  const known = data.total > 0 && phase === "downloading";
  const busy = phase === "installing" || (working && !known);
  const title = phase === "done" ? "yt-dlp is installed"
    : phase === "failed" ? "yt-dlp could not be installed"
      : phase === "downloading" ? "Downloading yt-dlp"
        : phase === "installing" ? "Checking yt-dlp"
          : update ? "Update yt-dlp" : "yt-dlp is required";
  let bar = "0%";
  if (busy) bar = "";
  else if (phase === "done") bar = "100%";
  else if (data.total > 0) bar = `${Math.max(0, Math.min(100, (data.received / data.total) * 100))}%`;
  let label = "";
  if (phase === "downloading" && data.total > 0) label = `Downloading… ${mb(data.received)} of ${mb(data.total)}`;
  else if (working) label = data.message || "";
  const action = update || data.installed ? "Update" : "Install";

  async function install() {
    setData({ phase: "downloading", received: 0, total: 0, message: "Downloading yt-dlp…" });
    const response = await fetch(update || data.installed ? "/api/ytdlp/update" : "/api/ytdlp/install", { method: "POST" });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      setData({ phase: "failed", error: body.error || "Could not start the download." });
      return;
    }
    setData(await response.json());
  }

  return (
    <div className="alert-layer">
      <div className="alert" role="dialog" aria-modal="true" aria-labelledby="ytdlp-alert-title">
        <div className="alert-main">
          <h2 id="ytdlp-alert-title">{title}</h2>
          <p>{data.error || data.message || "YouTube captures use yt-dlp. Marchive can download the current release and replace it when you ask."}</p>
          <div className={`progress${busy ? " is-busy" : ""}`} hidden={!(working || phase === "done")}>
            <div className="progress-bar" style={bar ? { width: bar } : undefined} />
          </div>
          <p className="progress-label">{label}</p>
          <div className="alert-actions">
            <button type="button" hidden={working || phase === "done"} onClick={onClose}>Not Now</button>
            <button type="button" className="push" hidden={working || phase === "done" || phase === "failed"} onClick={install}>{action}</button>
            <button
              type="button"
              hidden={!working}
              onClick={async () => {
                await fetch("/api/ytdlp/install/cancel", { method: "POST" });
                const response = await fetch("/api/ytdlp", { cache: "no-store" });
                if (response.ok) setData(await response.json());
              }}
            >
              Cancel
            </button>
            <button
              type="button"
              className="push"
              hidden={phase !== "done" && phase !== "failed" && phase !== "cancelled"}
              onClick={() => {
                if (phase === "done") {
                  window.location.reload();
                  return;
                }
                setData({ phase: "idle", message: "YouTube captures use yt-dlp. Marchive can download the current release and replace it when you ask." });
              }}
            >
              {phase === "failed" ? "Try Again" : "OK"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
